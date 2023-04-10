/*
Copyright 2021 The Crossplane Authors.

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

package applicationconfiguration

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ctrlrec "github.com/kubevela/pkg/controller/reconciler"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	oamtype "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/common"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// Reconcile error strings.
const (
	errGetAppConfig          = "cannot get application configuration"
	errUpdateAppConfigStatus = "cannot update application configuration status"
	errExecutePrehooks       = "failed to execute pre-hooks"
	errExecutePosthooks      = "failed to execute post-hooks"
	errRenderComponents      = "cannot render components"
	errApplyComponents       = "cannot apply components"
	errGCComponent           = "cannot garbage collect components"
	errFinalizeWorkloads     = "failed to finalize workloads"
)

// Reconcile event reasons.
const (
	reasonRevision                = "ACRevision"
	reasonRenderComponents        = "RenderedComponents"
	reasonExecutePrehook          = "ExecutePrehook"
	reasonExecutePosthook         = "ExecutePosthook"
	reasonApplyComponents         = "AppliedComponents"
	reasonGGComponent             = "GarbageCollectedComponent"
	reasonCannotExecutePrehooks   = "CannotExecutePrehooks"
	reasonCannotExecutePosthooks  = "CannotExecutePosthooks"
	reasonCannotRenderComponents  = "CannotRenderComponents"
	reasonCannotApplyComponents   = "CannotApplyComponents"
	reasonCannotGGComponents      = "CannotGarbageCollectComponents"
	reasonCannotFinalizeWorkloads = "CannotFinalizeWorkloads"
)

// Setup adds a controller that reconciles ApplicationConfigurations.
func Setup(mgr ctrl.Manager, args core.Args) error {
	name := "oam/" + strings.ToLower(v1alpha2.ApplicationConfigurationGroupKind)

	builder := ctrl.NewControllerManagedBy(mgr)
	builder.WithOptions(controller.Options{
		MaxConcurrentReconciles: args.ConcurrentReconciles,
	})

	return builder.
		Named(name).
		For(&v1alpha2.ApplicationConfiguration{}).
		Watches(&source.Kind{Type: &v1alpha2.Component{}}, &ComponentHandler{
			Client:                mgr.GetClient(),
			RevisionLimit:         args.RevisionLimit,
			CustomRevisionHookURL: args.CustomRevisionHookURL,
		}).
		Complete(NewReconciler(mgr, args.DiscoveryMapper,
			WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
			WithApplyOnceOnlyMode(args.ApplyMode),
			WithDependCheckWait(args.DependCheckWait)))
}

// An OAMApplicationReconciler reconciles OAM ApplicationConfigurations by rendering and
// instantiating their Components and Traits.
type OAMApplicationReconciler struct {
	client            client.Client
	components        ComponentRenderer
	workloads         WorkloadApplicator
	gc                GarbageCollector
	scheme            *runtime.Scheme
	record            event.Recorder
	preHooks          map[string]ControllerHooks
	postHooks         map[string]ControllerHooks
	applyOnceOnlyMode core.ApplyOnceOnlyMode
	dependCheckWait   time.Duration
}

// A ReconcilerOption configures a Reconciler.
type ReconcilerOption func(*OAMApplicationReconciler)

// WithRenderer specifies how the Reconciler should render workloads and traits.
func WithRenderer(r ComponentRenderer) ReconcilerOption {
	return func(rc *OAMApplicationReconciler) {
		rc.components = r
	}
}

// WithApplicator specifies how the Reconciler should apply workloads and traits.
func WithApplicator(a WorkloadApplicator) ReconcilerOption {
	return func(rc *OAMApplicationReconciler) {
		rc.workloads = a
	}
}

// WithGarbageCollector specifies how the Reconciler should garbage collect
// workloads and traits when an ApplicationConfiguration is edited to remove
// them.
func WithGarbageCollector(gc GarbageCollector) ReconcilerOption {
	return func(rc *OAMApplicationReconciler) {
		rc.gc = gc
	}
}

// WithRecorder specifies how the Reconciler should record events.
func WithRecorder(er event.Recorder) ReconcilerOption {
	return func(r *OAMApplicationReconciler) {
		r.record = er
	}
}

// WithPrehook register a pre-hook to the Reconciler
func WithPrehook(name string, hook ControllerHooks) ReconcilerOption {
	return func(r *OAMApplicationReconciler) {
		r.preHooks[name] = hook
	}
}

// WithPosthook register a post-hook to the Reconciler
func WithPosthook(name string, hook ControllerHooks) ReconcilerOption {
	return func(r *OAMApplicationReconciler) {
		r.postHooks[name] = hook
	}
}

// WithApplyOnceOnlyMode indicates whether workloads and traits should be
// affected if no spec change is made in the ApplicationConfiguration.
func WithApplyOnceOnlyMode(mode core.ApplyOnceOnlyMode) ReconcilerOption {
	return func(r *OAMApplicationReconciler) {
		r.applyOnceOnlyMode = mode
	}
}

// WithDependCheckWait set depend check wait
func WithDependCheckWait(dependCheckWait time.Duration) ReconcilerOption {
	return func(r *OAMApplicationReconciler) {
		r.dependCheckWait = dependCheckWait
	}
}

// NewReconciler returns an OAMApplicationReconciler that reconciles ApplicationConfigurations
// by rendering and instantiating their Components and Traits.
func NewReconciler(m ctrl.Manager, dm discoverymapper.DiscoveryMapper, o ...ReconcilerOption) *OAMApplicationReconciler {
	r := &OAMApplicationReconciler{
		client: m.GetClient(),
		scheme: m.GetScheme(),
		components: &components{
			client:   m.GetClient(),
			dm:       dm,
			params:   ParameterResolveFn(resolve),
			workload: ResourceRenderFn(renderWorkload),
			trait:    ResourceRenderFn(renderTrait),
		},
		workloads: &workloads{
			applicator: apply.NewAPIApplicator(m.GetClient()),
			rawClient:  m.GetClient(),
			dm:         dm,
		},
		gc:                GarbageCollectorFn(eligible),
		record:            event.NewNopRecorder(),
		preHooks:          make(map[string]ControllerHooks),
		postHooks:         make(map[string]ControllerHooks),
		applyOnceOnlyMode: core.ApplyOnceOnlyOff,
	}

	for _, ro := range o {
		ro(r)
	}

	return r
}

// NOTE(negz): We don't validate anything against their definitions at the
// controller level. We assume this will be done by validating admission
// webhooks.

// Reconcile an OAM ApplicationConfigurations by rendering and instantiating its
// Components and Traits.
func (r *OAMApplicationReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	ctx, cancel := ctrlrec.NewReconcileContext(ctx)
	defer cancel()

	klog.InfoS("Reconcile applicationConfiguration", "applicationConfiguration", klog.KRef(req.Namespace, req.Name))

	ac := &v1alpha2.ApplicationConfiguration{}
	if err := r.client.Get(ctx, req.NamespacedName, ac); err != nil {
		return reconcile.Result{}, errors.Wrap(client.IgnoreNotFound(err), errGetAppConfig)
	}

	ctx = util.SetNamespaceInCtx(ctx, ac.Namespace)
	if ac.ObjectMeta.DeletionTimestamp.IsZero() {
		if registerFinalizers(ac) {
			klog.V(common.LogDebug).InfoS("Register new finalizers", "finalizers", ac.ObjectMeta.Finalizers)
			return reconcile.Result{}, errors.Wrap(r.client.Update(ctx, ac), errUpdateAppConfigStatus)
		}
	} else {
		if err := r.workloads.Finalize(ctx, ac); err != nil {
			klog.InfoS("Failed to finalize workloads", "workloads status", ac.Status.Workloads,
				"err", err)
			r.record.Event(ac, event.Warning(reasonCannotFinalizeWorkloads, err))
			ac.SetConditions(condition.ReconcileError(errors.Wrap(err, errFinalizeWorkloads)))
			return reconcile.Result{}, errors.Wrap(r.UpdateStatus(ctx, ac), errUpdateAppConfigStatus)
		}
		return reconcile.Result{}, errors.Wrap(r.client.Update(ctx, ac), errUpdateAppConfigStatus)
	}

	reconResult, err := r.ACReconcile(ctx, ac)
	if err != nil {
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r.client, ac, condition.ReconcileError(err))
	}
	// always update ac status and set the error
	if err := r.UpdateStatus(ctx, ac); err != nil {
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r.client, ac, condition.ReconcileError(err))
	}
	return reconResult, nil
}

// ACReconcile contains all the reconcile logic of an AC, it can be used by other controller
func (r *OAMApplicationReconciler) ACReconcile(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) (result reconcile.Result, resultErr error) {

	acPatch := ac.DeepCopy()
	// execute the posthooks at the end no matter what
	defer func() {
		updateObservedGeneration(ac)
		for name, hook := range r.postHooks {
			exeResult, err := hook.Exec(ctx, ac)
			if err != nil {
				klog.InfoS("Failed to execute post-hooks", "hook name", name, "err", err,
					"requeue-after", exeResult.RequeueAfter)
				r.record.Event(ac, event.Warning(reasonCannotExecutePosthooks, err))
				result = exeResult
				resultErr = errors.Wrap(err, errExecutePosthooks)
				return
			}
			r.record.Event(ac, event.Normal(reasonExecutePosthook, "Successfully executed a posthook",
				"posthook name", name))
		}
	}()

	// execute the prehooks
	for name, hook := range r.preHooks {
		result, err := hook.Exec(ctx, ac)
		if err != nil {
			klog.InfoS("Failed to execute pre-hooks", "hook name", name, "requeue-after", result.RequeueAfter, "err", err)
			r.record.Event(ac, event.Warning(reasonCannotExecutePrehooks, err))
			return result, errors.Wrap(err, errExecutePrehooks)
		}
		r.record.Event(ac, event.Normal(reasonExecutePrehook, "Successfully executed a prehook", "prehook name ", name))
	}

	klog.InfoS("ApplicationConfiguration", "uid", ac.GetUID(), "version", ac.GetResourceVersion())

	// we have special logics for application generated applicationConfiguration
	if isControlledByApp(ac) {
		if ac.GetAnnotations()[oam.AnnotationAppRevision] == strconv.FormatBool(true) {
			msg := "Encounter an application revision, no need to reconcile"
			klog.Info(msg)
			r.record.Event(ac, event.Normal(reasonRevision, msg))
			ac.SetConditions(condition.Unavailable())
			ac.Status.RollingStatus = oamtype.InactiveAfterRollingCompleted
			// TODO: GC the traits/workloads
			return reconcile.Result{}, nil
		}
	}

	workloads, depStatus, err := r.components.Render(ctx, ac)
	if err != nil {
		klog.InfoS("Cannot render components", "err", err)
		r.record.Event(ac, event.Warning(reasonCannotRenderComponents, err))
		return reconcile.Result{}, errors.Wrap(err, errRenderComponents)
	}
	klog.V(common.LogDebug).InfoS("Successfully rendered components", "workloads", len(workloads))
	r.record.Event(ac, event.Normal(reasonRenderComponents, "Successfully rendered components",
		"workloads", strconv.Itoa(len(workloads))))

	applyOpts := []apply.ApplyOption{apply.MustBeControllableBy(ac.GetUID()), applyOnceOnly(ac, r.applyOnceOnlyMode)}
	if err := r.workloads.Apply(ctx, ac.Status.Workloads, workloads, applyOpts...); err != nil {
		klog.InfoS("Cannot apply workload", "err", err)
		r.record.Event(ac, event.Warning(reasonCannotApplyComponents, err))
		return reconcile.Result{}, errors.Wrap(err, errApplyComponents)
	}
	// only change the status after the apply succeeds
	// TODO: take into account the templating object may not be applied if there are dependencies
	if ac.Status.RollingStatus == oamtype.RollingTemplating {
		klog.InfoS("mark the ac rolling status as templated", "appConfig", klog.KRef(ac.Namespace, ac.Name))
		ac.Status.RollingStatus = oamtype.RollingTemplated
	}
	klog.V(common.LogDebug).InfoS("Successfully applied components", "workloads", len(workloads))
	r.record.Event(ac, event.Normal(reasonApplyComponents, "Successfully applied components",
		"workloads", strconv.Itoa(len(workloads))))

	// Kubernetes garbage collection will (by default) reap workloads and traits
	// when the appconfig that controls them (in the controller reference sense)
	// is deleted. Here we cover the case in which a component or one of its
	// traits is removed from an extant appconfig.
	for _, e := range r.gc.Eligible(ac.GetNamespace(), ac.Status.Workloads, workloads) {
		// https://github.com/golang/go/wiki/CommonMistakes#using-reference-to-loop-iterator-variable
		e := e
		klog.InfoS("Collect garbage ", "resource", klog.KRef(e.GetNamespace(), e.GetName()),
			"apiVersion", e.GetAPIVersion(), "kind", e.GetKind())
		record := r.record.WithAnnotations("kind", e.GetKind(), "name", e.GetName())

		err := r.confirmDeleteOnApplyOnceMode(ctx, ac.GetNamespace(), &e)
		if err != nil {
			klog.InfoS("Confirm component can't be garbage collected", "err", err)
			record.Event(ac, event.Warning(reasonCannotGGComponents, err))
			return reconcile.Result{}, errors.Wrap(err, errGCComponent)
		}
		if err := r.client.Delete(ctx, &e); resource.IgnoreNotFound(err) != nil {
			klog.InfoS("Cannot garbage collect component", "err", err)
			record.Event(ac, event.Warning(reasonCannotGGComponents, err))
			return reconcile.Result{}, errors.Wrap(err, errGCComponent)
		}
		klog.V(common.LogDebug).Info("Garbage collected resource")
		record.Event(ac, event.Normal(reasonGGComponent, "Successfully garbage collected component"))
	}

	// patch the final status on the client side, k8s sever can't merge them
	r.updateStatus(ctx, ac, acPatch, workloads)

	ac.Status.Dependency = v1alpha2.DependencyStatus{}
	var waitTime time.Duration
	if len(depStatus.Unsatisfied) != 0 {
		waitTime = r.dependCheckWait
		ac.Status.Dependency = *depStatus
	}

	// the defer function will do the final status update
	return reconcile.Result{RequeueAfter: waitTime}, nil
}

// confirmDeleteOnApplyOnceMode will confirm whether the workload can be delete or not in apply once only enabled mode
// currently only workload replicas with 0 can be delete
func (r *OAMApplicationReconciler) confirmDeleteOnApplyOnceMode(ctx context.Context, namespace string, u *unstructured.Unstructured) error {
	if r.applyOnceOnlyMode == core.ApplyOnceOnlyOff {
		return nil
	}
	getU := u.DeepCopy()
	err := r.client.Get(ctx, client.ObjectKey{Name: u.GetName(), Namespace: namespace}, getU)
	if err != nil {
		// no need to check if workload not found
		return resource.IgnoreNotFound(err)
	}
	// only check for workload
	if labels := getU.GetLabels(); labels == nil || labels[oam.LabelOAMResourceType] != oam.ResourceTypeWorkload {
		return nil
	}
	paved := fieldpath.Pave(getU.Object)

	// TODO: add more kinds of workload replica check here if needed
	// "spec.replicas" maybe not accurate for all kinds of workload, but it work for most of them(including Deployment/StatefulSet/CloneSet).
	// For workload which don't align with the `spec.replicas` schema, the check won't work
	replicas, err := paved.GetInteger("spec.replicas")
	if err != nil {
		// it's possible for workload without the `spec.replicas`, it's omitempty
		if strings.Contains(err.Error(), "no such field") {
			return nil
		}
		return errors.WithMessage(err, "fail to get 'spec.replicas' from workload")
	}
	if replicas > 0 {
		return errors.Errorf("can't delete workload with replicas %d in apply once only mode", replicas)
	}
	return nil
}

// UpdateStatus updates v1alpha2.ApplicationConfiguration's Status with retry.RetryOnConflict
func (r *OAMApplicationReconciler) UpdateStatus(ctx context.Context, ac *v1alpha2.ApplicationConfiguration, opts ...client.SubResourceUpdateOption) error {
	status := ac.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.client.Get(ctx, types.NamespacedName{Namespace: ac.Namespace, Name: ac.Name}, ac); err != nil {
			return
		}
		ac.Status = status
		return r.client.Status().Update(ctx, ac, opts...)
	})
}

func (r *OAMApplicationReconciler) updateStatus(ctx context.Context, ac, acPatch *v1alpha2.ApplicationConfiguration, workloads []Workload) {
	ac.Status.Workloads = make([]v1alpha2.WorkloadStatus, len(workloads))
	historyWorkloads := make([]v1alpha2.HistoryWorkload, 0)
	for i, w := range workloads {
		ac.Status.Workloads[i] = workloads[i].Status()
		if !w.RevisionEnabled {
			continue
		}
		var ul unstructured.UnstructuredList
		ul.SetKind(w.Workload.GetKind())
		ul.SetAPIVersion(w.Workload.GetAPIVersion())
		if err := r.client.List(ctx, &ul, client.MatchingLabels{oam.LabelAppName: ac.Name,
			oam.LabelAppComponent: w.ComponentName, oam.LabelOAMResourceType: oam.ResourceTypeWorkload}); err != nil {
			continue
		}
		for _, v := range ul.Items {
			if v.GetName() == w.ComponentRevisionName {
				continue
			}
			// These workload exists means the component is under progress of rollout
			// Trait will not work for these remaining workload
			historyWorkloads = append(historyWorkloads, v1alpha2.HistoryWorkload{
				Revision: v.GetName(),
				Reference: corev1.ObjectReference{
					APIVersion: v.GetAPIVersion(),
					Kind:       v.GetKind(),
					Name:       v.GetName(),
					UID:        v.GetUID(),
				},
			})
		}
	}
	ac.Status.HistoryWorkloads = historyWorkloads
	// patch the extra fields in the status that is wiped by the Status() function
	patchExtraStatusField(&ac.Status, acPatch.Status)
	ac.SetConditions(condition.ReconcileSuccess())
}

func updateObservedGeneration(ac *v1alpha2.ApplicationConfiguration) {
	if ac.Status.ObservedGeneration != ac.Generation {
		ac.Status.ObservedGeneration = ac.Generation
	}
	for i, w := range ac.Status.Workloads {
		// only the workload meet all dependency can mean the generation applied successfully
		if w.AppliedComponentRevision != w.ComponentRevisionName && !w.DependencyUnsatisfied {
			ac.Status.Workloads[i].AppliedComponentRevision = w.ComponentRevisionName
		}
		for j, t := range w.Traits {
			if t.AppliedGeneration != ac.Generation && !t.DependencyUnsatisfied {
				ac.Status.Workloads[i].Traits[j].AppliedGeneration = ac.Generation
			}
		}
	}
}

func patchExtraStatusField(acStatus *v1alpha2.ApplicationConfigurationStatus, acPatchStatus v1alpha2.ApplicationConfigurationStatus) {
	// patch the extra status back
	for i := range acStatus.Workloads {
		for _, w := range acPatchStatus.Workloads {
			// find the workload in the old status
			if acStatus.Workloads[i].ComponentRevisionName == w.ComponentRevisionName {
				if len(w.Status) > 0 {
					acStatus.Workloads[i].Status = w.Status
				}
				// find the trait
				for j := range acStatus.Workloads[i].Traits {
					for _, t := range w.Traits {
						tr := acStatus.Workloads[i].Traits[j].Reference
						if t.Reference.APIVersion == tr.APIVersion && t.Reference.Kind == tr.Kind && t.Reference.Name == tr.Name {
							if len(t.Status) > 0 {
								acStatus.Workloads[i].Traits[j].Status = t.Status
							}
						}
					}
				}
			}
		}
	}
}

// if any finalizers newly registered, return true
func registerFinalizers(ac *v1alpha2.ApplicationConfiguration) bool {
	newFinalizer := false
	if !meta.FinalizerExists(&ac.ObjectMeta, workloadScopeFinalizer) && hasScope(ac) {
		meta.AddFinalizer(&ac.ObjectMeta, workloadScopeFinalizer)
		newFinalizer = true
	}
	return newFinalizer
}

func hasScope(ac *v1alpha2.ApplicationConfiguration) bool {
	for _, c := range ac.Spec.Components {
		if len(c.Scopes) > 0 {
			return true
		}
	}
	return false
}

// A Workload produced by an OAM ApplicationConfiguration.
type Workload struct {
	// ComponentName that produced this workload.
	ComponentName string

	// ComponentRevisionName of current component
	ComponentRevisionName string

	// A Workload object.
	Workload *unstructured.Unstructured

	// SkipApply indicates that the workload should not be applied
	SkipApply bool

	// HasDep indicates whether this resource has dependencies and unready to be applied.
	HasDep bool

	// Traits associated with this workload.
	Traits []*Trait

	// RevisionEnabled means multiple workloads of same component will possibly be alive.
	RevisionEnabled bool

	// Scopes associated with this workload.
	Scopes []unstructured.Unstructured

	// Record the DataOutputs of this workload, key is name of DataOutput.
	DataOutputs map[string]v1alpha2.DataOutput

	// Record the DataInputs of this workload.
	DataInputs []v1alpha2.DataInput
}

// A Trait produced by an OAM ApplicationConfiguration.
type Trait struct {
	Object unstructured.Unstructured

	// HasDep indicates whether this resource has dependencies and unready to be applied.
	HasDep bool

	// Definition indicates the trait's definition
	Definition v1alpha2.TraitDefinition

	// Record the DataOutputs of this trait, key is name of DataOutput.
	DataOutputs map[string]v1alpha2.DataOutput

	// Record the DataInputs of this trait.
	DataInputs []v1alpha2.DataInput
}

// Status produces the status of this workload and its traits, suitable for use
// in the status of an ApplicationConfiguration.
func (w Workload) Status() v1alpha2.WorkloadStatus {
	acw := v1alpha2.WorkloadStatus{
		ComponentName:         w.ComponentName,
		ComponentRevisionName: w.ComponentRevisionName,
		DependencyUnsatisfied: w.HasDep,
		Reference: corev1.ObjectReference{
			APIVersion: w.Workload.GetAPIVersion(),
			Kind:       w.Workload.GetKind(),
			Name:       w.Workload.GetName(),
		},
		Traits: make([]v1alpha2.WorkloadTrait, len(w.Traits)),
		Scopes: make([]v1alpha2.WorkloadScope, len(w.Scopes)),
	}
	for i, tr := range w.Traits {
		if tr.Definition.Name == util.Dummy && tr.Definition.Spec.Reference.Name == util.Dummy {
			acw.Traits[i].Message = util.DummyTraitMessage
		}
		acw.Traits[i].Reference = corev1.ObjectReference{
			APIVersion: w.Traits[i].Object.GetAPIVersion(),
			Kind:       w.Traits[i].Object.GetKind(),
			Name:       w.Traits[i].Object.GetName(),
		}
		acw.Traits[i].DependencyUnsatisfied = tr.HasDep
	}
	for i, s := range w.Scopes {
		acw.Scopes[i].Reference = corev1.ObjectReference{
			APIVersion: s.GetAPIVersion(),
			Kind:       s.GetKind(),
			Name:       s.GetName(),
		}
	}
	return acw
}

// A GarbageCollector returns resource eligible for garbage collection. A
// resource is considered eligible if a reference exists in the supplied slice
// of workload statuses, but not in the supplied slice of workloads.
type GarbageCollector interface {
	Eligible(namespace string, ws []v1alpha2.WorkloadStatus, w []Workload) []unstructured.Unstructured
}

// A GarbageCollectorFn returns resource eligible for garbage collection.
type GarbageCollectorFn func(namespace string, ws []v1alpha2.WorkloadStatus, w []Workload) []unstructured.Unstructured

// Eligible resources.
func (fn GarbageCollectorFn) Eligible(namespace string, ws []v1alpha2.WorkloadStatus, w []Workload) []unstructured.Unstructured {
	return fn(namespace, ws, w)
}

// IsRevisionWorkload check is a workload is an old revision Workload which shouldn't be garbage collected.
func IsRevisionWorkload(status v1alpha2.WorkloadStatus, w []Workload) bool {
	if strings.HasPrefix(status.Reference.Name, status.ComponentName+"-") {
		// for compatibility, keep the old way
		return true
	}

	// check all workload, with same componentName
	for _, wr := range w {
		if wr.ComponentName == status.ComponentName {
			return wr.RevisionEnabled
		}
	}
	// component not found, should be deleted
	return false
}

func eligible(namespace string, ws []v1alpha2.WorkloadStatus, w []Workload) []unstructured.Unstructured {
	applied := make(map[corev1.ObjectReference]bool)
	for _, wl := range w {
		r := corev1.ObjectReference{
			APIVersion: wl.Workload.GetAPIVersion(),
			Kind:       wl.Workload.GetKind(),
			Name:       wl.Workload.GetName(),
		}
		applied[r] = true
		for _, t := range wl.Traits {
			r := corev1.ObjectReference{
				APIVersion: t.Object.GetAPIVersion(),
				Kind:       t.Object.GetKind(),
				Name:       t.Object.GetName(),
			}
			applied[r] = true
		}
	}
	eligible := make([]unstructured.Unstructured, 0)
	for _, s := range ws {

		if !applied[s.Reference] && !IsRevisionWorkload(s, w) {
			w := &unstructured.Unstructured{}
			w.SetAPIVersion(s.Reference.APIVersion)
			w.SetKind(s.Reference.Kind)
			w.SetNamespace(namespace)
			w.SetName(s.Reference.Name)
			eligible = append(eligible, *w)
		}

		for _, ts := range s.Traits {
			if !applied[ts.Reference] {
				t := &unstructured.Unstructured{}
				t.SetAPIVersion(ts.Reference.APIVersion)
				t.SetKind(ts.Reference.Kind)
				t.SetNamespace(namespace)
				t.SetName(ts.Reference.Name)
				eligible = append(eligible, *t)
			}
		}
	}

	return eligible
}

// GenerationUnchanged indicates the resource being applied has no generation changed
// comparing to the existing one.
type GenerationUnchanged struct{}

func (e *GenerationUnchanged) Error() string {
	return fmt.Sprint("apply-only-once enabled,",
		"and detect generation in the annotation unchanged, will not apply.",
		"Please ignore this error in other logic.")
}

// applyOnceOnly is an ApplyOption that controls the applying mechanism for workload and trait.
// More detail refers to the ApplyOnceOnlyMode type annotation
func applyOnceOnly(ac *v1alpha2.ApplicationConfiguration, mode core.ApplyOnceOnlyMode) apply.ApplyOption {
	return apply.MakeCustomApplyOption(func(existing, desired client.Object) error {
		if mode == core.ApplyOnceOnlyOff {
			return nil
		}
		d, _ := desired.(metav1.Object)
		if d == nil {
			return errors.Errorf("cannot access metadata of object being applied: %q",
				desired.GetObjectKind().GroupVersionKind())
		}
		dLabels := d.GetLabels()
		dAnnots := d.GetAnnotations()
		if dLabels[oam.LabelOAMResourceType] != oam.ResourceTypeWorkload &&
			dLabels[oam.LabelOAMResourceType] != oam.ResourceTypeTrait {
			// this ApplyOption only works for workload and trait
			// skip if the resource is not workload nor trait, e.g., scope
			klog.InfoS("Ignore apply only once check, because resourceType is not workload or trait",
				oam.LabelOAMResourceType, dLabels[oam.LabelOAMResourceType])
			return nil
		}

		// the resource doesn't exist (maybe not created before, or created but deleted by others)
		if existing == nil {
			if mode != core.ApplyOnceOnlyForce {
				// non-force mode will always create the resource if not exist.
				klog.InfoS("Apply only once with mode:" + string(mode) + ", but old resource not exist, will create a new one")
				return nil
			}

			createdBefore := false
			var appliedRevision, appliedGeneration string
			for _, w := range ac.Status.Workloads {
				// traverse recorded workloads to find the one matching applied resource
				if w.Reference.GetObjectKind().GroupVersionKind() == desired.GetObjectKind().GroupVersionKind() &&
					w.Reference.Name == d.GetName() {
					// the workload matches applied resource
					createdBefore = true
					// for workload, when revision enabled, only when revision changed that can trigger to create a new one
					if dLabels[oam.LabelOAMResourceType] == oam.ResourceTypeWorkload &&
						w.AppliedComponentRevision == dLabels[oam.LabelAppComponentRevision] {
						// the revision is not changed, so return an error to abort creating it
						return &GenerationUnchanged{}
					}
					appliedRevision = w.AppliedComponentRevision
					break
				}
				// the workload is not matched, then traverse its traits to find matching one
				for _, t := range w.Traits {
					if t.Reference.GetObjectKind().GroupVersionKind() == desired.GetObjectKind().GroupVersionKind() &&
						t.Reference.Name == d.GetName() {
						// the trait matches applied resource
						createdBefore = true
						// the resource was created before and appConfig status recorded the resource version applied
						// if recorded AppliedGeneration and ComponentRevisionName both equal to the applied resource's,
						// that means its spec is not changed
						if dLabels[oam.LabelOAMResourceType] == oam.ResourceTypeTrait &&
							w.ComponentRevisionName == dLabels[oam.LabelAppComponentRevision] &&
							strconv.FormatInt(t.AppliedGeneration, 10) == dAnnots[oam.AnnotationAppGeneration] {
							// the revision is not changed, so return an error to abort creating it
							return &GenerationUnchanged{}
						}
						appliedGeneration = strconv.FormatInt(t.AppliedGeneration, 10)
						break
					}
				}
			}
			var message = "apply only once with mode: force, but resource not created before, will create new"
			if createdBefore {
				message = "apply only once with mode: force, but resource updated, will create new"
			}
			klog.InfoS(message, "appConfig", ac.Name, "gvk", desired.GetObjectKind().GroupVersionKind(), "name", d.GetName(),
				"resourceType", dLabels[oam.LabelOAMResourceType], "appliedCompRevision", appliedRevision,
				"labeledCompRevision", dLabels[oam.LabelAppComponentRevision],
				"appliedGeneration", appliedGeneration, "labeledGeneration", dAnnots[oam.AnnotationAppGeneration])

			// no recorded workloads nor traits matches the applied resource
			// that means the resource is not created before, so create it
			return nil
		}

		// the resource already exists
		e, _ := existing.(metav1.Object)
		if e == nil {
			return errors.Errorf("cannot access metadata of existing object: %q",
				existing.GetObjectKind().GroupVersionKind())
		}
		eLabels := e.GetLabels()
		// if existing resource's (observed)AppConfigGeneration and ComponentRevisionName both equal to the applied one's,
		// that means its spec is not changed
		if (e.GetAnnotations()[oam.AnnotationAppGeneration] != dAnnots[oam.AnnotationAppGeneration]) ||
			(eLabels[oam.LabelAppComponentRevision] != dLabels[oam.LabelAppComponentRevision]) {
			klog.InfoS("Apply only once with mode: "+string(mode)+", but new generation or revision created, will create new",
				oam.AnnotationAppGeneration, e.GetAnnotations()[oam.AnnotationAppGeneration]+"/"+dAnnots[oam.AnnotationAppGeneration],
				oam.LabelAppComponentRevision, eLabels[oam.LabelAppComponentRevision]+"/"+dLabels[oam.LabelAppComponentRevision])
			// its spec is changed, so apply new configuration to it
			return nil
		}
		// its spec is not changed, return an error to abort applying it
		return &GenerationUnchanged{}
	})
}
