/*
Copyright 2020 The Crossplane Authors.

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

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	reconcileTimeout = 1 * time.Minute
	dependCheckWait  = 10 * time.Second
	shortWait        = 30 * time.Second
	longWait         = 1 * time.Minute
)

var errResult = reconcile.Result{RequeueAfter: shortWait}

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
func Setup(mgr ctrl.Manager, args core.Args, l logging.Logger) error {
	dm, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("create discovery dm fail %w", err)
	}
	name := "oam/" + strings.ToLower(v1alpha2.ApplicationConfigurationGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&v1alpha2.ApplicationConfiguration{}).
		Watches(&source.Kind{Type: &v1alpha2.Component{}}, &ComponentHandler{
			Client:                mgr.GetClient(),
			Logger:                l,
			RevisionLimit:         args.RevisionLimit,
			CustomRevisionHookURL: args.CustomRevisionHookURL,
		}).
		Complete(NewReconciler(mgr, dm,
			l.WithValues("controller", name),
			WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
			WithApplyOnceOnlyMode(args.ApplyMode)))
}

// An OAMApplicationReconciler reconciles OAM ApplicationConfigurations by rendering and
// instantiating their Components and Traits.
type OAMApplicationReconciler struct {
	client            client.Client
	components        ComponentRenderer
	workloads         WorkloadApplicator
	gc                GarbageCollector
	scheme            *runtime.Scheme
	log               logging.Logger
	record            event.Recorder
	preHooks          map[string]ControllerHooks
	postHooks         map[string]ControllerHooks
	applyOnceOnlyMode core.ApplyOnceOnlyMode
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

// NewReconciler returns an OAMApplicationReconciler that reconciles ApplicationConfigurations
// by rendering and instantiating their Components and Traits.
func NewReconciler(m ctrl.Manager, dm discoverymapper.DiscoveryMapper, log logging.Logger, o ...ReconcilerOption) *OAMApplicationReconciler {
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
			applicator: apply.NewAPIApplicator(m.GetClient(), log),
			rawClient:  m.GetClient(),
			dm:         dm,
		},
		gc:                GarbageCollectorFn(eligible),
		log:               log,
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
func (r *OAMApplicationReconciler) Reconcile(req reconcile.Request) (result reconcile.Result, returnErr error) {
	log := r.log.WithValues("request", req)
	log.Debug("Reconciling")

	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()

	ac := &v1alpha2.ApplicationConfiguration{}
	if err := r.client.Get(ctx, req.NamespacedName, ac); err != nil {
		if apierrors.IsNotFound(err) {
			// stop processing this resource
			return ctrl.Result{}, nil
		}
		return errResult, errors.Wrap(err, errGetAppConfig)
	}
	acPatch := ac.DeepCopy()
	firstReconcile := false
	var newComponents []string
	if ac.ObjectMeta.DeletionTimestamp.IsZero() {
		if registerFinalizers(ac) {
			log.Debug("Register new finalizers", "finalizers", ac.ObjectMeta.Finalizers)
			return reconcile.Result{}, errors.Wrap(r.client.Update(ctx, ac), errUpdateAppConfigStatus)
		}
	} else {
		if err := r.workloads.Finalize(ctx, ac); err != nil {
			log.Debug("Failed to finalize workloads", "workloads status", ac.Status.Workloads,
				"error", err, "requeue-after", result.RequeueAfter)
			r.record.Event(ac, event.Warning(reasonCannotFinalizeWorkloads, err))
			ac.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, errFinalizeWorkloads)))
			return reconcile.Result{}, errors.Wrap(r.UpdateStatus(ctx, ac), errUpdateAppConfigStatus)
		}
		return reconcile.Result{}, errors.Wrap(r.client.Update(ctx, ac), errUpdateAppConfigStatus)
	}

	// make sure this is the last functional defer function to be called
	defer func() {
		// Make sure if error occurs, reconcile will not happen too frequency
		if returnErr != nil && result.RequeueAfter < shortWait {
			result.RequeueAfter = shortWait
		}
	}()

	// execute the posthooks at the end no matter what
	defer func() {
		updateObservedGeneration(ac)
		for name, hook := range r.postHooks {
			exeResult, err := hook.Exec(ctx, ac, log)
			if err != nil {
				log.Debug("Failed to execute post-hooks", "hook name", name, "error", err, "requeue-after", result.RequeueAfter)
				r.record.Event(ac, event.Warning(reasonCannotExecutePosthooks, err))
				ac.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, errExecutePosthooks)))
				result = exeResult
				returnErr = errors.Wrap(r.UpdateStatus(ctx, ac), errUpdateAppConfigStatus)
				return
			}
			r.record.Event(ac, event.Normal(reasonExecutePosthook, "Successfully executed a posthook", "posthook name", name))
		}
		returnErr = errors.Wrap(r.UpdateStatus(ctx, ac), errUpdateAppConfigStatus)
	}()

	// execute the prehooks
	for name, hook := range r.preHooks {
		result, err := hook.Exec(ctx, ac, log)
		if err != nil {
			log.Debug("Failed to execute pre-hooks", "hook name", name, "error", err, "requeue-after", result.RequeueAfter)
			r.record.Event(ac, event.Warning(reasonCannotExecutePrehooks, err))
			ac.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, errExecutePrehooks)))
			return result, errors.Wrap(r.UpdateStatus(ctx, ac), errUpdateAppConfigStatus)
		}
		r.record.Event(ac, event.Normal(reasonExecutePrehook, "Successfully executed a prehook", "prehook name ", name))
	}

	log = log.WithValues("uid", ac.GetUID(), "version", ac.GetResourceVersion())

	// we have special logics for application generated applicationConfiguration
	if newAppConifgAnno, exist := ac.GetAnnotations()[oam.AnnotationNewAppConfig]; exist {
		if newAppConifgAnno == "true" {
			firstReconcile = true
			defer func() {
				// we only flip the annotation after a successful reconcile
				if returnErr == nil {
					ac.GetAnnotations()[oam.AnnotationNewAppConfig] = "false"
					returnErr = r.client.Update(ctx, ac)
				}
			}()
		}
	}
	// TODO: Handle multiple components
	if newComponent, exist := ac.GetAnnotations()[oam.AnnotationNewComponent]; exist {
		newComponents = []string{newComponent}
	}

	workloads, depStatus, err := r.components.Render(ctx, ac)
	if err != nil {
		log.Info("Cannot render components", "error", err, "requeue-after", time.Now().Add(shortWait))
		r.record.Event(ac, event.Warning(reasonCannotRenderComponents, err))
		ac.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, errRenderComponents)))
		return errResult, errors.Wrap(r.UpdateStatus(ctx, ac), errUpdateAppConfigStatus)
	}
	log.Debug("Successfully rendered components", "workloads", len(workloads))
	r.record.Event(ac, event.Normal(reasonRenderComponents, "Successfully rendered components", "workloads", strconv.Itoa(len(workloads))))

	applyOpts := []apply.ApplyOption{apply.MustBeControllableBy(ac.GetUID()), applyOnceOnly(ac, r.applyOnceOnlyMode, log)}
	if err := r.workloads.Apply(ctx, ac.Status.Workloads, workloads, firstReconcile, newComponents, applyOpts...); err != nil {
		log.Debug("Cannot apply components", "error", err, "requeue-after", time.Now().Add(shortWait))
		r.record.Event(ac, event.Warning(reasonCannotApplyComponents, err))
		ac.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, errApplyComponents)))
		return errResult, errors.Wrap(r.UpdateStatus(ctx, ac), errUpdateAppConfigStatus)
	}
	log.Debug("Successfully applied components", "workloads", len(workloads))
	r.record.Event(ac, event.Normal(reasonApplyComponents, "Successfully applied components", "workloads", strconv.Itoa(len(workloads))))

	// Kubernetes garbage collection will (by default) reap workloads and traits
	// when the appconfig that controls them (in the controller reference sense)
	// is deleted. Here we cover the case in which a component or one of its
	// traits is removed from an extant appconfig.
	for _, e := range r.gc.Eligible(ac.GetNamespace(), ac.Status.Workloads, workloads) {
		// https://github.com/golang/go/wiki/CommonMistakes#using-reference-to-loop-iterator-variable
		e := e

		log := log.WithValues("kind", e.GetKind(), "name", e.GetName())
		record := r.record.WithAnnotations("kind", e.GetKind(), "name", e.GetName())

		if err := r.client.Delete(ctx, &e); resource.IgnoreNotFound(err) != nil {
			log.Debug("Cannot garbage collect component", "error", err, "requeue-after", time.Now().Add(shortWait))
			record.Event(ac, event.Warning(reasonCannotGGComponents, err))
			ac.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, errGCComponent)))
			return errResult, errors.Wrap(r.UpdateStatus(ctx, ac), errUpdateAppConfigStatus)
		}
		log.Debug("Garbage collected resource")
		record.Event(ac, event.Normal(reasonGGComponent, "Successfully garbage collected component"))
	}

	// patch the final status on the client side, k8s sever can't merge them
	r.updateStatus(ctx, ac, acPatch, workloads)

	ac.Status.Dependency = v1alpha2.DependencyStatus{}
	waitTime := longWait
	if len(depStatus.Unsatisfied) != 0 {
		waitTime = dependCheckWait
		ac.Status.Dependency = *depStatus
	}

	// the posthook function will do the final status update
	return reconcile.Result{RequeueAfter: waitTime}, nil
}

// UpdateStatus updates v1alpha2.ApplicationConfiguration's Status with retry.RetryOnConflict
func (r *OAMApplicationReconciler) UpdateStatus(ctx context.Context, ac *v1alpha2.ApplicationConfiguration, opts ...client.UpdateOption) error {
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
		if err := r.client.List(ctx, &ul, client.MatchingLabels{oam.LabelAppName: ac.Name, oam.LabelAppComponent: w.ComponentName, oam.LabelOAMResourceType: oam.ResourceTypeWorkload}); err != nil {
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
				Reference: v1alpha1.TypedReference{
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
	ac.SetConditions(v1alpha1.ReconcileSuccess())
}

func updateObservedGeneration(ac *v1alpha2.ApplicationConfiguration) {
	if ac.Status.ObservedGeneration != ac.Generation {
		ac.Status.ObservedGeneration = ac.Generation
	}
	for i, w := range ac.Status.Workloads {
		// only all workload meet requirements can say the generation is observed successfully
		if w.ObservedGeneration != ac.Generation && len(ac.Status.Dependency.Unsatisfied) == 0 {
			ac.Status.Workloads[i].ObservedGeneration = ac.GetGeneration()
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
		Reference: v1alpha1.TypedReference{
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
		acw.Traits[i].Reference = v1alpha1.TypedReference{
			APIVersion: w.Traits[i].Object.GetAPIVersion(),
			Kind:       w.Traits[i].Object.GetKind(),
			Name:       w.Traits[i].Object.GetName(),
		}
	}
	for i, s := range w.Scopes {
		acw.Scopes[i].Reference = v1alpha1.TypedReference{
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
// TODO(wonderflow): Do we have a better way to recognise it's a revisionWorkload which can't be garbage collected by AppConfig?
func IsRevisionWorkload(status v1alpha2.WorkloadStatus) bool {
	return strings.HasPrefix(status.Reference.Name, status.ComponentName+"-")
}

func eligible(namespace string, ws []v1alpha2.WorkloadStatus, w []Workload) []unstructured.Unstructured {
	applied := make(map[v1alpha1.TypedReference]bool)
	for _, wl := range w {
		r := v1alpha1.TypedReference{
			APIVersion: wl.Workload.GetAPIVersion(),
			Kind:       wl.Workload.GetKind(),
			Name:       wl.Workload.GetName(),
		}
		applied[r] = true
		for _, t := range wl.Traits {
			r := v1alpha1.TypedReference{
				APIVersion: t.Object.GetAPIVersion(),
				Kind:       t.Object.GetKind(),
				Name:       t.Object.GetName(),
			}
			applied[r] = true
		}
	}
	eligible := make([]unstructured.Unstructured, 0)
	for _, s := range ws {

		if !applied[s.Reference] && !IsRevisionWorkload(s) {
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
func applyOnceOnly(ac *v1alpha2.ApplicationConfiguration, mode core.ApplyOnceOnlyMode, log logging.Logger) apply.ApplyOption {
	return func(_ context.Context, existing, desired runtime.Object) error {
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
			log.Info("ignore apply only once check, because resourceType is not workload or trait", oam.LabelOAMResourceType, dLabels[oam.LabelOAMResourceType])
			return nil
		}

		// the resource doesn't exist (maybe not created before, or created but deleted by others)
		if existing == nil {
			if mode != core.ApplyOnceOnlyForce {
				// non-force mode will always create the resource if not exist.
				log.Info("apply only once with mode:" + string(mode) + ", but old resource not exist, will create a new one")
				return nil
			}

			createdBefore := false
			for _, w := range ac.Status.Workloads {
				// traverse recorded workloads to find the one matching applied resource
				if w.Reference.GetObjectKind().GroupVersionKind() == desired.GetObjectKind().GroupVersionKind() &&
					w.Reference.Name == d.GetName() {
					// the workload matches applied resource
					createdBefore = true
				}
				if !createdBefore {
					// the workload is not matched, then traverse its traits to find matching one
					for _, t := range w.Traits {
						if t.Reference.GetObjectKind().GroupVersionKind() == desired.GetObjectKind().GroupVersionKind() &&
							t.Reference.Name == d.GetName() {
							// the trait matches applied resource
							createdBefore = true
						}
					}
				}
				// don't use if-else here because it will miss the case that the resource is a trait
				if createdBefore {
					// the resource was created before and appConfig status recorded the resource version applied
					// if recorded ObservedGeneration and ComponentRevisionName both equal to the applied resource's,
					// that means its spec is not changed
					if (strconv.Itoa(int(w.ObservedGeneration)) != dAnnots[oam.AnnotationAppGeneration]) ||
						(w.ComponentRevisionName != dLabels[oam.LabelAppComponentRevision]) {
						log.Info("apply only once with mode: "+string(mode)+", but condition not meet, will create new",
							oam.AnnotationAppGeneration, strconv.Itoa(int(w.ObservedGeneration))+"/"+dAnnots[oam.AnnotationAppGeneration],
							oam.LabelAppComponentRevision, w.ComponentRevisionName+"/"+dLabels[oam.LabelAppComponentRevision])
						// its spec is changed, so re-create the resource
						return nil
					}
					// its spec is not changed, so return an error to abort creating it
					return &GenerationUnchanged{}
				}
			}
			log.Info("apply only once with mode: force, but resource not created before, will create new", "appConfig", ac.Name)
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
			log.Info("apply only once with mode: "+string(mode)+", but new generation or revision created, will create new",
				oam.AnnotationAppGeneration, e.GetAnnotations()[oam.AnnotationAppGeneration]+"/"+dAnnots[oam.AnnotationAppGeneration],
				oam.LabelAppComponentRevision, eLabels[oam.LabelAppComponentRevision]+"/"+dLabels[oam.LabelAppComponentRevision])
			// its spec is changed, so apply new configuration to it
			return nil
		}
		// its spec is not changed, return an error to abort applying it
		return &GenerationUnchanged{}
	}
}
