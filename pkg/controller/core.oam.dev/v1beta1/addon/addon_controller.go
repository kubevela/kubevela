/*
Copyright 2024 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package addon contains the controller that continuously reconciles Addon
// resources (KEP-2.13, Declarative Addon Lifecycle). The controller is
// registered with the manager behind the AddonCRD feature gate and honors the
// pause label. On each reconcile of a live CR it applies a cleanup finalizer,
// fetches and installs the addon by delegating to pkg/addon, and reports
// status; when the CR is being deleted it runs the deletion policy instead. It
// reschedules itself periodically so drift is corrected without an explicit
// change to the CR.
package addon

import (
	"context"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	ctrlrec "github.com/kubevela/pkg/controller/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/features"
)

const (
	// FieldManagerAddonController is the field manager identity used for the
	// addon controller's status writes, so its ownership of status fields is
	// attributable and does not contend with other writers.
	FieldManagerAddonController = "addon.oam.dev/controller"

	// defaultReconcileInterval is the periodic resync cadence for the addon
	// controller. Each reconcile reschedules itself after this interval so that
	// drift is corrected even without an explicit change to the Addon CR.
	defaultReconcileInterval = 5 * time.Minute

	// failedThreshold is how long SourceResolved may stay False before the phase
	// escalates to failed. It matches the rate-limiter cap so escalation lines up
	// with the backoff window.
	failedThreshold = 10 * time.Minute

	// FinalizerAddonCleanup gates deletion of an Addon CR so the controller can
	// run its deletion policy (Protect/Force/Orphan) before the CR is removed
	// (KEP-2.13). It is added to live CRs in Reconcile and cleared by
	// handleDeletion once cleanup is done.
	FinalizerAddonCleanup = "addon.oam.dev/cleanup"
)

// addonRateLimiter returns the per-controller backoff: 30s base, 10m cap.
func addonRateLimiter() workqueue.TypedRateLimiter[reconcile.Request] {
	return workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](30*time.Second, failedThreshold)
}

// Reconciler reconciles an Addon object. Its field layout mirrors the
// Application reconciler (embedded client, Scheme, Recorder, plus options).
type Reconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Recorder        event.Recorder
	DiscoveryClient *discovery.DiscoveryClient
	Config          *rest.Config
	options

	// Test seams, defaulted to the real implementations by ensureDefaults so the
	// reconcile and deletion logic can be unit-tested without a live installer
	// or cluster. installFn installs, readBackFn reflects the result into
	// status, disableFn tears the owned Application down.
	installFn  func(ctx context.Context, ad *v1beta1.Addon) error
	readBackFn func(ctx context.Context, ad *v1beta1.Addon) error
	disableFn  func(ctx context.Context, ad *v1beta1.Addon, force bool) error
}

type options struct {
	concurrentReconciles int
	reconcileInterval    time.Duration
}

// parseOptions builds the reconciler options from the shared controller args,
// mirroring the parseOptions helper used by the other core controllers.
func parseOptions(args oamctrl.Args) options {
	return options{
		concurrentReconciles: args.ConcurrentReconciles,
		reconcileInterval:    defaultReconcileInterval,
	}
}

// interval returns the configured periodic reconcile interval, falling back to
// the default when unset.
func (r *Reconciler) interval() time.Duration {
	if r.reconcileInterval <= 0 {
		return defaultReconcileInterval
	}
	return r.reconcileInterval
}

// Reconcile drives one Addon through its lifecycle. It fetches the CR and
// honors the pause label, then branches: a CR with a deletionTimestamp runs the
// deletion policy via handleDeletion; otherwise it ensures the cleanup
// finalizer, selects the installing/upgrading phase, delegates the install to
// pkg/addon, reflects the result into status, and reaches running/Ready=True. A
// registry-unreachable failure keeps the phase and backs off (escalating to
// failed past the backoff window); other install errors fail immediately. It
// reschedules itself after the configured interval.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := ctrlrec.NewReconcileContext(ctx)
	defer cancel()

	var addon v1beta1.Addon
	if err := r.Get(ctx, req.NamespacedName, &addon); err != nil {
		// The CR was deleted before this reconcile ran; nothing to do.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if ctrlrec.IsPaused(&addon) {
		klog.InfoS("reconciliation paused", "addon", klog.KObj(&addon))
		return ctrl.Result{RequeueAfter: r.interval()}, nil
	}

	klog.InfoS("Reconcile addon", "addon", klog.KObj(&addon), "observedGeneration", addon.GetGeneration())

	r.ensureDefaults()

	// Being deleted: run the deletion policy and release the finalizer instead
	// of installing.
	if !addon.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &addon)
	}

	// Live CR: ensure the cleanup finalizer is present so a future delete is
	// intercepted by handleDeletion (deletion-policy logic).
	if !controllerutil.ContainsFinalizer(&addon, FinalizerAddonCleanup) {
		controllerutil.AddFinalizer(&addon, FinalizerAddonCleanup)
		if err := r.Update(ctx, &addon); err != nil {
			return ctrl.Result{}, err
		}
	}

	base := addon.DeepCopy()

	if addon.Status.Phase == "" {
		setPhase(&addon, v1beta1.AddonPhaseInstalling)
		setCondition(&addon, v1beta1.AddonConditionReady, metav1.ConditionFalse, "Reconciling", "addon reconcile started")
	} else if addon.Spec.Version != "" && addon.Spec.Version != addon.Status.InstalledVersion && addon.Status.InstalledVersion != "" {
		setPhase(&addon, v1beta1.AddonPhaseUpgrading)
	}

	if err := r.installFn(ctx, &addon); err != nil {
		if isRegistryUnreachable(err) {
			stale := sourceResolvedStaleFor(&addon, failedThreshold)
			setCondition(&addon, v1beta1.AddonConditionSourceResolved, metav1.ConditionFalse, "RegistryUnreachable", err.Error())
			if stale {
				setPhase(&addon, v1beta1.AddonPhaseFailed)
			}
			if perr := r.patchStatus(ctx, base, &addon); perr != nil {
				return ctrl.Result{}, perr
			}
			return ctrl.Result{}, err
		}
		// Not a source/registry failure: the source resolved, so reflect that
		// and fail on the install step rather than leaving a stale
		// SourceResolved=False from an earlier registry outage.
		setCondition(&addon, v1beta1.AddonConditionSourceResolved, metav1.ConditionTrue, "SourceFetched", "addon source resolved and fetched")
		setPhase(&addon, v1beta1.AddonPhaseFailed)
		setCondition(&addon, v1beta1.AddonConditionReady, metav1.ConditionFalse, "InstallFailed", err.Error())
		if perr := r.patchStatus(ctx, base, &addon); perr != nil {
			return ctrl.Result{}, perr
		}
		return ctrl.Result{}, err
	}

	setCondition(&addon, v1beta1.AddonConditionSourceResolved, metav1.ConditionTrue, "SourceFetched", "addon source resolved and fetched")
	if err := r.readBackFn(ctx, &addon); err != nil {
		// Install succeeded but reflecting the result into status failed. Persist
		// the SourceResolved progress so the failure is observable, then retry via
		// the rate limiter — do not mislabel the phase as failed.
		if perr := r.patchStatus(ctx, base, &addon); perr != nil {
			return ctrl.Result{}, perr
		}
		return ctrl.Result{}, err
	}
	setPhase(&addon, v1beta1.AddonPhaseRunning)
	setCondition(&addon, v1beta1.AddonConditionReady, metav1.ConditionTrue, "Installed", "addon installed and running")

	if err := r.patchStatus(ctx, base, &addon); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: r.interval()}, nil
}

// ensureDefaults wires the install / read-back / disable seams to their real
// implementations when unset (tests inject fakes).
func (r *Reconciler) ensureDefaults() {
	if r.installFn == nil {
		r.installFn = r.install
	}
	if r.readBackFn == nil {
		r.readBackFn = r.readBackStatus
	}
	if r.disableFn == nil {
		r.disableFn = r.disable
	}
}

// SetupWithManager wires the reconciler into the controller-runtime manager,
// watching Addon resources and using the exponential backoff rate limiter
// (addonRateLimiter) for requeues on error.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
			RateLimiter:             addonRateLimiter(),
		}).
		For(&v1beta1.Addon{}).
		Complete(r)
}

// Setup adds the addon controller to the manager. It is a no-op unless the
// AddonCRD feature gate is enabled (see KEP-2.13), so it can sit in the
// standard controller setup list without special-casing. When enabled it builds
// the discovery client and rest config the installer needs, then registers the
// reconciler.
func Setup(mgr ctrl.Manager, args oamctrl.Args) error {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.AddonCRD) {
		return nil
	}
	config := mgr.GetConfig()
	dc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return err
	}
	r := Reconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		Recorder:        event.NewAPIRecorder(mgr.GetEventRecorderFor("Addon")).WithAnnotations("controller", "Addon"),
		DiscoveryClient: dc,
		Config:          config,
		options:         parseOptions(args),
	}
	return r.SetupWithManager(mgr)
}
