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
// resources (KEP-2.13, Declarative Addon Lifecycle). This file lands the
// reconciler skeleton: it is registered with the manager behind the AddonCRD
// feature gate, honors the pause label, and reschedules itself periodically.
// It intentionally performs no install, status, or finalizer work yet;
// subsequent stories fill in that behavior.
package addon

import (
	"context"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	ctrlrec "github.com/kubevela/pkg/controller/reconciler"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/features"
)

const (
	// FieldManagerAddonController is the field manager identity used for all
	// server-side-apply writes performed by the addon controller. It is defined
	// here so subsequent stories use a consistent owner; the skeleton does not
	// yet perform any SSA writes.
	FieldManagerAddonController = "addon.oam.dev/controller"

	// defaultReconcileInterval is the periodic resync cadence for the addon
	// controller. Each reconcile reschedules itself after this interval so that
	// drift is corrected even without an explicit change to the Addon CR.
	defaultReconcileInterval = 5 * time.Minute
)

// Reconciler reconciles an Addon object. Its field layout mirrors the
// Application reconciler (embedded client, Scheme, Recorder, plus options).
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder event.Recorder
	options
}

type options struct {
	concurrentReconciles int
	reconcileInterval    time.Duration
}

// Enabled reports whether the AddonCRD feature gate is on. setup.go consults
// this to decide whether to register the controller with the manager.
func Enabled() bool {
	return utilfeature.DefaultMutableFeatureGate.Enabled(features.AddonCRD)
}

// interval returns the configured periodic reconcile interval, falling back to
// the default when unset.
func (r *Reconciler) interval() time.Duration {
	if r.reconcileInterval <= 0 {
		return defaultReconcileInterval
	}
	return r.reconcileInterval
}

// Reconcile is the skeleton reconcile loop. It fetches the Addon, honors the
// pause label, logs the observed generation, and reschedules itself. It does
// not write status, manage finalizers, or perform any install work yet.
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

	// No status writes, finalizer, or install logic yet — subsequent stories
	// (phase machine, source resolve, delegation hook, finalizer) fill this in.
	return ctrl.Result{RequeueAfter: r.interval()}, nil
}

// SetupWithManager wires the reconciler into the controller-runtime manager,
// watching Addon resources.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = event.NewAPIRecorder(mgr.GetEventRecorderFor("Addon")).
			WithAnnotations("controller", "Addon")
	}
	if r.reconcileInterval <= 0 {
		r.reconcileInterval = defaultReconcileInterval
	}
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
		}).
		For(&v1beta1.Addon{}).
		Complete(r)
}

// Setup adds the addon controller to the manager. Callers gate this on
// Enabled() so the controller is only registered when the AddonCRD feature
// gate is on.
func Setup(mgr ctrl.Manager, args oamctrl.Args) error {
	r := Reconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("Addon")).WithAnnotations("controller", "Addon"),
		options: options{
			concurrentReconciles: args.ConcurrentReconciles,
			reconcileInterval:    defaultReconcileInterval,
		},
	}
	return r.SetupWithManager(mgr)
}
