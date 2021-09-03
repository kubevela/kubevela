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

package healthscope

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonapis "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	af "github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/common"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

const (
	longWait = 10 * time.Second
)

// Reconcile error strings.
const (
	errGetHealthScope          = "cannot get health scope"
	errUpdateHealthScopeStatus = "cannot update health scope status"
)

// Reconcile event reasons.
const (
	reasonHealthCheck = "HealthCheck"
)

// Setup adds a controller that reconciles HealthScope.
func Setup(mgr ctrl.Manager, args controller.Args) error {
	name := "oam/" + strings.ToLower(v1alpha2.HealthScopeGroupKind)
	r := NewReconciler(mgr, WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))))
	r.dm = args.DiscoveryMapper
	r.pd = args.PackageDiscover
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&v1alpha2.HealthScope{}).
		Complete(r)
}

// A Reconciler reconciles OAM Scopes by keeping track of the health status of components.
type Reconciler struct {
	client client.Client
	dm     discoverymapper.DiscoveryMapper
	pd     *packages.PackageDiscover
	record event.Recorder
	// traitChecker represents checker fetching health condition from HealthCheckTrait
	traitChecker WorloadHealthChecker
	// checkers represents a set of built-in checkers
	checkers []WorloadHealthChecker
	// unknownChecker represents checker handling workloads that
	// cannot be hanlded by traitChecker nor built-in checkers
	unknownChecker WorloadHealthChecker
}

// A ReconcilerOption configures a Reconciler.
type ReconcilerOption func(*Reconciler)

// WithRecorder specifies how the Reconciler should record events.
func WithRecorder(er event.Recorder) ReconcilerOption {
	return func(r *Reconciler) {
		r.record = er
	}
}

// WithTraitChecker adds health checker based on HealthCheckTrait
func WithTraitChecker(c WorloadHealthChecker) ReconcilerOption {
	return func(r *Reconciler) {
		r.traitChecker = c
	}
}

// WithChecker adds workload health checker
func WithChecker(c WorloadHealthChecker) ReconcilerOption {
	return func(r *Reconciler) {
		if r.checkers == nil {
			r.checkers = make([]WorloadHealthChecker, 0)
		}
		r.checkers = append(r.checkers, c)
	}
}

// NewReconciler returns a Reconciler that reconciles HealthScope by keeping track of its healthstatus.
func NewReconciler(m ctrl.Manager, o ...ReconcilerOption) *Reconciler {
	r := &Reconciler{
		client:       m.GetClient(),
		record:       event.NewNopRecorder(),
		traitChecker: WorkloadHealthCheckFn(CheckByHealthCheckTrait),
		checkers: []WorloadHealthChecker{
			WorkloadHealthCheckFn(CheckPodSpecWorkloadHealth),
			WorkloadHealthCheckFn(CheckContainerziedWorkloadHealth),
			WorkloadHealthCheckFn(CheckDeploymentHealth),
			WorkloadHealthCheckFn(CheckStatefulsetHealth),
			WorkloadHealthCheckFn(CheckDaemonsetHealth),
		},
		unknownChecker: WorkloadHealthCheckFn(CheckUnknownWorkload),
	}
	for _, ro := range o {
		ro(r)
	}

	return r
}

// Reconcile an OAM HealthScope by keeping track of its health status.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	ctx, cancel := common.NewReconcileContext(ctx)
	defer cancel()
	klog.InfoS("Reconcile healthScope", "healthScope", klog.KRef(req.Namespace, req.Name))

	hs := &v1alpha2.HealthScope{}
	if err := r.client.Get(ctx, req.NamespacedName, hs); err != nil {
		return reconcile.Result{}, errors.Wrap(resource.IgnoreNotFound(err), errGetHealthScope)
	}

	interval := longWait
	if hs.Spec.ProbeInterval != nil {
		interval = time.Duration(*hs.Spec.ProbeInterval) * time.Second
	}

	if interval <= 0 {
		interval = longWait
	}

	start := time.Now()

	klog.InfoS("healthScope", "uid", hs.GetUID(), "version", hs.GetResourceVersion())

	scopeCondition, appConditions := r.GetScopeHealthStatus(ctx, hs)
	klog.V(common.LogDebug).InfoS("Successfully ran health check", "scope", hs.Name)
	r.record.Event(hs, event.Normal(reasonHealthCheck, "Successfully ran health check"))

	elapsed := time.Since(start)
	hs.Status.ScopeHealthCondition = scopeCondition
	hs.Status.AppHealthConditions = appConditions

	if err := r.patchHealthStatusToApplications(ctx, appConditions, hs); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "cannot patch health status to application")
	}
	return reconcile.Result{RequeueAfter: interval - elapsed}, errors.Wrap(r.UpdateStatus(ctx, hs), errUpdateHealthScopeStatus)
}

// GetScopeHealthStatus get the status of the healthscope based on workload resources.
func (r *Reconciler) GetScopeHealthStatus(ctx context.Context, healthScope *v1alpha2.HealthScope) (ScopeHealthCondition, []*AppHealthCondition) {
	klog.InfoS("Get scope health status", "name", healthScope.GetName())
	scopeCondition := ScopeHealthCondition{
		HealthStatus: StatusHealthy, // if no workload referenced, scope is healthy by default
	}

	var wlRefs []corev1.ObjectReference
	if len(healthScope.Spec.WorkloadReferences) > 0 {
		wlRefs = healthScope.Spec.WorkloadReferences
	} else {
		wlRefs = make([]corev1.ObjectReference, 0)
		for _, app := range healthScope.Spec.AppRefs {
			for _, comp := range app.CompReferences {
				wlRefs = append(wlRefs, comp.Workload)
			}
		}
	}

	if len(wlRefs) == 0 {
		return scopeCondition, []*AppHealthCondition{}
	}

	timeout := defaultTimeout
	if healthScope.Spec.ProbeTimeout != nil {
		timeout = time.Duration(*healthScope.Spec.ProbeTimeout) * time.Second
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	appfiles, appNames := r.CollectAppfilesAndAppNames(ctx, wlRefs, healthScope.GetNamespace())

	type wlHealthResult struct {
		name string
		w    *WorkloadHealthCondition
	}
	// process workloads concurrently
	wlHealthResultsC := make(chan wlHealthResult, len(wlRefs))
	var wg sync.WaitGroup
	wg.Add(len(wlRefs))

	for _, workloadRef := range wlRefs {
		go func(resRef corev1.ObjectReference) {
			defer wg.Done()
			var (
				wlHealthCondition *WorkloadHealthCondition
				traitConditions   []*TraitHealthCondition
			)

			if appfile, ok := appfiles[resRef]; ok {
				wlHealthCondition, traitConditions = CUEBasedHealthCheck(ctx, r.client, resRef, healthScope.GetNamespace(), appfile)
				if wlHealthCondition != nil {
					klog.V(common.LogDebug).InfoS("Get health condition from CUE-based health check", "workload", resRef, "healthCondition", wlHealthCondition)
					wlHealthCondition.Traits = traitConditions
					wlHealthResultsC <- wlHealthResult{
						name: appNames[resRef],
						w:    wlHealthCondition,
					}
					return
				}
			}

			wlHealthCondition = r.traitChecker.Check(ctx, r.client, resRef, healthScope.GetNamespace())
			if wlHealthCondition != nil {
				klog.V(common.LogDebug).InfoS("Get health condition from health check trait ", "workload", resRef, "healthCondition", wlHealthCondition)
				wlHealthCondition.Traits = traitConditions
				wlHealthResultsC <- wlHealthResult{
					name: appNames[resRef],
					w:    wlHealthCondition,
				}
				return
			}

			for _, checker := range r.checkers {
				wlHealthCondition = checker.Check(ctxWithTimeout, r.client, resRef, healthScope.GetNamespace())
				if wlHealthCondition != nil {
					klog.V(common.LogDebug).InfoS("Get health condition from built-in checker", "workload", resRef, "healthCondition", wlHealthCondition)
					// found matched checker and get health condition
					wlHealthCondition.Traits = traitConditions
					wlHealthResultsC <- wlHealthResult{
						name: appNames[resRef],
						w:    wlHealthCondition,
					}
					return
				}
			}
			// handle unknown workload
			klog.V(common.LogDebug).InfoS("Gpkg/controller/core.oam.dev/v1alpha2/setup.go:42:69et unknown workload", "workload", resRef)
			wlHealthCondition = r.unknownChecker.Check(ctx, r.client, resRef, healthScope.GetNamespace())
			wlHealthCondition.Traits = traitConditions
			wlHealthResultsC <- wlHealthResult{
				name: appNames[resRef],
				w:    wlHealthCondition,
			}
		}(workloadRef)
	}

	go func() {
		wg.Wait()
		close(wlHealthResultsC)
	}()

	appHealthConditions := make([]*AppHealthCondition, 0)
	var healthyCount, unhealthyCount, unknownCount int64
	for wlC := range wlHealthResultsC {
		switch wlC.w.HealthStatus { //nolint:exhaustive
		case StatusHealthy:
			healthyCount++
		case StatusUnhealthy:
			unhealthyCount++
		case StatusUnknown:
			unknownCount++
		default:
			unknownCount++
		}
		appended := false
		for _, a := range appHealthConditions {
			if a.AppName == wlC.name {
				a.Components = append(a.Components, wlC.w)
				appended = true
				break
			}
		}
		if !appended {
			appHealth := &AppHealthCondition{
				AppName:    wlC.name,
				Components: []*v1alpha2.WorkloadHealthCondition{wlC.w},
			}
			appHealthConditions = append(appHealthConditions, appHealth)
		}
	}
	if unhealthyCount > 0 || unknownCount > 0 {
		// ANY unhealthy or unknown worloads make the whole scope unhealthy
		scopeCondition.HealthStatus = StatusUnhealthy
	}
	scopeCondition.Total = int64(len(wlRefs))
	scopeCondition.HealthyWorkloads = healthyCount
	scopeCondition.UnhealthyWorkloads = unhealthyCount
	scopeCondition.UnknownWorkloads = unknownCount

	return scopeCondition, appHealthConditions
}

// CollectAppfilesAndAppNames retrieve appfiles and app names for CUEBasedHealthCheck
func (r *Reconciler) CollectAppfilesAndAppNames(ctx context.Context, refs []corev1.ObjectReference, ns string) (map[corev1.ObjectReference]*af.Appfile, map[corev1.ObjectReference]string) {
	appfiles := map[corev1.ObjectReference]*af.Appfile{}
	appNames := map[corev1.ObjectReference]string{}

	tmps := map[string]*af.Appfile{}
	for _, ref := range refs {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(ref.GroupVersionKind())
		if err := r.client.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ns}, u); err != nil {
			// no need to check error in this function
			// HealthCheckFn  will handle all errors latter
			continue
		}
		appName := u.GetLabels()[oam.LabelAppName]
		if appfile, ok := tmps[appName]; ok {
			appfiles[ref] = appfile
			appNames[ref] = appName
			continue
		}
		app := &v1beta1.Application{}
		if err := r.client.Get(ctx, client.ObjectKey{Name: appName, Namespace: ns}, app); err != nil {
			continue
		}
		appParser := af.NewApplicationParser(r.client, r.dm, r.pd)
		appfile, err := appParser.GenerateAppFile(ctx, app)
		if err != nil {
			continue
		}
		tmps[appName] = appfile

		appfiles[ref] = appfile
		appNames[ref] = appName
	}
	return appfiles, appNames
}

// UpdateStatus updates v1alpha2.HealthScope's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, hs *v1alpha2.HealthScope, opts ...client.UpdateOption) error {
	status := hs.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.client.Get(ctx, types.NamespacedName{Namespace: hs.Namespace, Name: hs.Name}, hs); err != nil {
			return
		}
		hs.Status = status
		return r.client.Status().Update(ctx, hs, opts...)
	})
}

func (r *Reconciler) patchHealthStatusToApplications(ctx context.Context, appHealthConditions []*AppHealthCondition, hs *v1alpha2.HealthScope) error {
	for _, appC := range appHealthConditions {
		if appC.AppName == "" {
			// for backward compatibility, skip patching status for HealthScope from v1alpha2.AppConfig
			continue
		}
		app := &v1beta1.Application{}
		if err := r.client.Get(ctx, client.ObjectKey{Name: appC.AppName, Namespace: hs.Namespace}, app); err != nil {
			return err
		}
		copyApp := app.DeepCopy()
		app.Status.Services = constructAppCompStatus(appC, corev1.ObjectReference{
			APIVersion: hs.APIVersion,
			Kind:       hs.Kind,
			Namespace:  hs.Namespace,
			Name:       hs.Name,
			UID:        hs.UID,
		})
		app.Status.SetConditions(condition.Condition{
			Type:               v1beta1.TypeHealthy,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             v1beta1.ReasonHealthy,
		})
		for _, compS := range app.Status.Services {
			if !compS.Healthy {
				app.Status.SetConditions(condition.Condition{
					Type:               v1beta1.TypeHealthy,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: metav1.Now(),
					Reason:             v1beta1.ReasonUnhealthy,
				})
				break
			}
		}
		if err := r.client.Status().Patch(ctx, app, client.MergeFrom(copyApp)); err != nil {
			return err
		}
	}
	return nil
}

// convert v1alpha2.AppHealthCondition to v1beta1.ApplicationComponentStatus used in Application status
func constructAppCompStatus(appC *AppHealthCondition, hsRef corev1.ObjectReference) []commonapis.ApplicationComponentStatus {
	r := make([]commonapis.ApplicationComponentStatus, len(appC.Components))
	for i, comp := range appC.Components {
		isCompHealthy := true
		isWorkloadHealthy := comp.HealthStatus == v1alpha2.StatusHealthy
		msg := comp.CustomStatusMsg
		if len(msg) == 0 {
			msg = comp.Diagnosis
		}
		r[i] = commonapis.ApplicationComponentStatus{
			Name: comp.ComponentName,
			WorkloadDefinition: commonapis.WorkloadGVK{
				APIVersion: comp.TargetWorkload.APIVersion,
				Kind:       comp.TargetWorkload.Kind,
			},
			Healthy: isWorkloadHealthy,
			Message: msg,
		}
		if !isWorkloadHealthy {
			isCompHealthy = false
		}
		if len(comp.Traits) > 0 {
			r[i].Traits = make([]commonapis.ApplicationTraitStatus, len(comp.Traits))
			for j, tC := range comp.Traits {
				isTraitHealthy := func() bool { return tC.HealthStatus == v1alpha2.StatusHealthy }()
				r[i].Traits[j] = commonapis.ApplicationTraitStatus{
					Type:    tC.Type,
					Healthy: isTraitHealthy,
					Message: tC.CustomStatusMsg,
				}
				if !isTraitHealthy {
					isCompHealthy = false
				}
			}
		}
		r[i].Scopes = []corev1.ObjectReference{hsRef}
		r[i].Healthy = isCompHealthy
	}
	return r
}
