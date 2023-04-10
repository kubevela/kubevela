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
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ctrlrec "github.com/kubevela/pkg/controller/reconciler"
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

	"github.com/kubevela/workflow/pkg/cue/packages"

	commonapis "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	af "github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/common"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/policy/envbinding"
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

// WorkloadReference refer to a multi-env workload
type WorkloadReference struct {
	corev1.ObjectReference
	clusterName string
	envName     string
}

// AppInfo contains app's name and app's env
type AppInfo struct {
	appName string
	envName string
}

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
	ctx, cancel := ctrlrec.NewReconcileContext(ctx)
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

	requeueAfter := interval - elapsed
	if requeueAfter <= time.Second { // prevent underflow
		requeueAfter = time.Second
	}
	return reconcile.Result{RequeueAfter: requeueAfter}, errors.Wrap(r.UpdateStatus(ctx, hs), errUpdateHealthScopeStatus)
}

// GetScopeHealthStatus get the status of the healthscope based on workload resources.
func (r *Reconciler) GetScopeHealthStatus(ctx context.Context, healthScope *v1alpha2.HealthScope) (ScopeHealthCondition, []*AppHealthCondition) {
	klog.InfoS("Get scope health status", "name", healthScope.GetName())
	scopeCondition := ScopeHealthCondition{
		HealthStatus: StatusHealthy, // if no workload referenced, scope is healthy by default
	}

	wlRefs := make([]WorkloadReference, 0)
	if len(healthScope.Spec.WorkloadReferences) > 0 {
		for _, ref := range healthScope.Spec.WorkloadReferences {
			wlRefs = append(wlRefs, WorkloadReference{
				ObjectReference: ref,
			})
		}
	} else {
		for _, app := range healthScope.Spec.AppRefs {
			wlRefs = append(wlRefs, r.createWorkloadRefs(ctx, app, healthScope.GetNamespace())...)
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

	appfiles, appInfos := r.CollectAppfilesAndAppNames(ctx, wlRefs, healthScope.GetNamespace())

	type wlHealthResult struct {
		name    string
		envName string
		w       *WorkloadHealthCondition
	}
	// process workloads concurrently
	wlHealthResultsC := make(chan wlHealthResult, len(wlRefs))
	var wg sync.WaitGroup
	wg.Add(len(wlRefs))

	for _, workloadRef := range wlRefs {
		go func(resRef WorkloadReference) {
			defer wg.Done()
			var (
				wlHealthCondition *WorkloadHealthCondition
				traitConditions   []*TraitHealthCondition
			)

			subCtx := multicluster.ContextWithClusterName(ctxWithTimeout, resRef.clusterName)
			ns := resRef.Namespace
			if ns == "" {
				ns = healthScope.GetNamespace()
			}
			if appfile, ok := appfiles[resRef]; ok {
				wlHealthCondition, traitConditions = CUEBasedHealthCheck(subCtx, r.client, resRef, ns, appfile)
				if wlHealthCondition != nil {
					klog.V(common.LogDebug).InfoS("Get health condition from CUE-based health check", "workload", resRef, "healthCondition", wlHealthCondition)
					wlHealthCondition.Traits = traitConditions
					wlHealthResultsC <- wlHealthResult{
						name:    appInfos[resRef].appName,
						envName: appInfos[resRef].envName,
						w:       wlHealthCondition,
					}
					return
				}
			}

			wlHealthCondition = r.traitChecker.Check(subCtx, r.client, resRef.ObjectReference, ns)
			if wlHealthCondition != nil {
				klog.V(common.LogDebug).InfoS("Get health condition from health check trait ", "workload", resRef, "healthCondition", wlHealthCondition)
				wlHealthCondition.Traits = traitConditions
				wlHealthResultsC <- wlHealthResult{
					name:    appInfos[resRef].appName,
					envName: appInfos[resRef].envName,
					w:       wlHealthCondition,
				}
				return
			}

			for _, checker := range r.checkers {
				wlHealthCondition = checker.Check(subCtx, r.client, resRef.ObjectReference, ns)
				if wlHealthCondition != nil {
					klog.V(common.LogDebug).InfoS("Get health condition from built-in checker", "workload", resRef, "healthCondition", wlHealthCondition)
					// found matched checker and get health condition
					wlHealthCondition.Traits = traitConditions
					wlHealthResultsC <- wlHealthResult{
						name:    appInfos[resRef].appName,
						envName: appInfos[resRef].envName,
						w:       wlHealthCondition,
					}
					return
				}
			}
			// handle unknown workload
			klog.V(common.LogDebug).InfoS("Get unknown workload", "workload", resRef)
			wlHealthCondition = r.unknownChecker.Check(subCtx, r.client, resRef.ObjectReference, ns)
			wlHealthCondition.Traits = traitConditions
			wlHealthResultsC <- wlHealthResult{
				name:    appInfos[resRef].appName,
				envName: appInfos[resRef].envName,
				w:       wlHealthCondition,
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
			if a.AppName == wlC.name && a.EnvName == wlC.envName {
				a.Components = append(a.Components, wlC.w)
				appended = true
				break
			}
		}
		if !appended {
			appHealth := &AppHealthCondition{
				AppName:    wlC.name,
				EnvName:    wlC.envName,
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
func (r *Reconciler) CollectAppfilesAndAppNames(ctx context.Context, refs []WorkloadReference, ns string) (map[WorkloadReference]*af.Appfile, map[WorkloadReference]AppInfo) {
	appfiles := map[WorkloadReference]*af.Appfile{}
	appNames := map[WorkloadReference]AppInfo{}

	tmps := map[AppInfo]*af.Appfile{}
	for _, ref := range refs {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(ref.GroupVersionKind())

		refNs := ref.Namespace
		if refNs == "" {
			refNs = ns
		}
		subCtx := multicluster.ContextWithClusterName(ctx, ref.clusterName)
		if err := r.client.Get(subCtx, client.ObjectKey{Name: ref.Name, Namespace: refNs}, u); err != nil {
			// no need to check error in this function
			// HealthCheckFn  will handle all errors latter
			continue
		}

		appInfo := AppInfo{
			appName: u.GetLabels()[oam.LabelAppName],
			envName: ref.envName,
		}
		if appfile, ok := tmps[appInfo]; ok {
			appfiles[ref] = appfile
			appNames[ref] = appInfo
			continue
		}

		// create new appfile
		appfile, err := r.createAppfile(ctx, appInfo.appName, ns, appInfo.envName)
		if err != nil {
			continue
		}
		tmps[appInfo] = appfile

		appfiles[ref] = appfile
		appNames[ref] = appInfo
	}
	return appfiles, appNames
}

// UpdateStatus updates v1alpha2.HealthScope's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, hs *v1alpha2.HealthScope, opts ...client.SubResourceUpdateOption) error {
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
	multiClusterAppCondition := make(map[string][]*AppHealthCondition)
	for _, appHealth := range appHealthConditions {
		multiClusterAppCondition[appHealth.AppName] = append(multiClusterAppCondition[appHealth.AppName], appHealth)
	}

	for appName, healthConditions := range multiClusterAppCondition {
		if appName == "" {
			// for backward compatibility, skip patching status for HealthScope from v1alpha2.AppConfig
			continue
		}
		app := &v1beta1.Application{}
		if err := r.client.Get(ctx, client.ObjectKey{Name: appName, Namespace: hs.Namespace}, app); err != nil {
			return err
		}
		if app.Status.Workflow == nil {
			continue
		}
		if !app.Status.Workflow.Finished && !app.Status.Workflow.Suspend {
			continue
		}
		copyApp := app.DeepCopy()
		componentPosition := make(map[string]int)
		for i, comp := range app.Spec.Components {
			componentPosition[comp.Name] = i
		}

		hsRef := corev1.ObjectReference{
			APIVersion: hs.APIVersion,
			Kind:       hs.Kind,
			Namespace:  hs.Namespace,
			Name:       hs.Name,
			UID:        hs.UID,
		}
		compStatus := make([]commonapis.ApplicationComponentStatus, 0)
		for i := range healthConditions {
			healthCondition := healthConditions[i]
			sort.Sort(sortAppCondition{
				componentPosition,
				healthCondition,
			})
			compStatus = append(compStatus, constructAppCompStatus(healthCondition, hsRef)...)

		}
		app.Status.Services = compStatus
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

func (r *Reconciler) createAppfile(ctx context.Context, appName, ns, envName string) (*af.Appfile, error) {
	appParser := af.NewApplicationParser(r.client, r.dm, r.pd)
	if len(envName) != 0 {
		app := &v1beta1.Application{}
		if err := r.client.Get(ctx, types.NamespacedName{Namespace: ns, Name: appName}, app); err != nil {
			return nil, err
		}
		patchedApp, err := envbinding.PatchApplicationByEnvBindingEnv(app, "", envName)
		if err != nil {
			return nil, err
		}
		return appParser.GenerateAppFile(ctx, patchedApp)
	}

	app := &v1beta1.Application{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: appName, Namespace: ns}, app); err != nil {
		return nil, err
	}
	return appParser.GenerateAppFile(ctx, app)
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
			Env:  appC.EnvName,
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

func (r *Reconciler) createWorkloadRefs(ctx context.Context, appRef v1alpha2.AppReference, ns string) []WorkloadReference {
	wlRefs := make([]WorkloadReference, 0)

	application := &v1beta1.Application{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: ns, Name: appRef.AppName}, application); err != nil {
		klog.ErrorS(err, "Failed to get application")
		return wlRefs
	}

	// ugly implementation, should be reworked in future
	decisionsMap := map[string]string{}
	var decisions []struct {
		Cluster string
		Env     string
	}
	policyStatus, err := envbinding.GetEnvBindingPolicyStatus(application, "")
	if err == nil && policyStatus != nil {
		for _, env := range policyStatus.Envs {
			for _, placement := range env.Placements {
				if placement.Namespace != "" {
					decisionsMap[placement.Cluster+"."+placement.Namespace] = env.Env
				} else {
					decisionsMap[placement.Cluster] = env.Env
				}
				decisions = append(decisions, struct {
					Cluster string
					Env     string
				}{
					Cluster: placement.Cluster,
					Env:     env.Env,
				})
			}
		}
	}

	if len(appRef.CompReferences) != 0 {
		for _, decision := range decisions {
			for _, comp := range appRef.CompReferences {
				wlRefs = append(wlRefs, WorkloadReference{
					ObjectReference: comp.Workload,
					clusterName:     decision.Cluster,
					envName:         decision.Env,
				})
			}
		}
		return wlRefs
	}

	if application.Status.AppliedResources != nil {
		resources := application.Status.AppliedResources
		for _, rs := range resources {
			if rs.Creator == commonapis.WorkflowResourceCreator {
				o := new(unstructured.Unstructured)
				o.SetKind(rs.Kind)
				o.SetAPIVersion(rs.APIVersion)
				if err := r.client.Get(multicluster.ContextWithClusterName(ctx, rs.Cluster), client.ObjectKey{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}, o); err != nil {
					continue
				}

				if labels := o.GetLabels(); labels != nil {
					var envName string
					if _envName, ok := decisionsMap[rs.Cluster+"."+rs.Namespace]; ok {
						envName = _envName
					} else {
						envName = decisionsMap[rs.Cluster]
					}
					if labels[oam.WorkloadTypeLabel] != "" {
						wlRefs = append(wlRefs, WorkloadReference{
							ObjectReference: rs.ObjectReference,
							clusterName:     rs.Cluster,
							envName:         envName,
						})
					} else if labels[oam.TraitTypeLabel] != "" && labels[oam.LabelManageWorkloadTrait] == "true" {
						// this means this trait is a manage-Workload trait, get workload GVK and name for trait's annotation
						objectRef := corev1.ObjectReference{}
						err := json.Unmarshal([]byte(o.GetAnnotations()[oam.AnnotationWorkloadGVK]), &objectRef)
						if err != nil {
							// don't break whole check process due to this error
							continue
						}
						if o.GetAnnotations() != nil && len(o.GetAnnotations()[oam.AnnotationWorkloadName]) != 0 {
							objectRef.Name = o.GetAnnotations()[oam.AnnotationWorkloadName]
						} else {
							// use component name as default
							objectRef.Name = labels[oam.LabelAppComponent]
						}
						wlRefs = append(wlRefs, WorkloadReference{
							ObjectReference: objectRef,
							clusterName:     rs.Cluster,
							envName:         envName,
						})
					}
				}
			}
		}
	}
	return wlRefs
}

type sortAppCondition struct {
	componentPosition map[string]int
	appCondition      *AppHealthCondition
}

func (s sortAppCondition) Len() int { return len(s.appCondition.Components) }
func (s sortAppCondition) Swap(i, j int) {
	s.appCondition.Components[i], s.appCondition.Components[j] = s.appCondition.Components[j], s.appCondition.Components[i]
}
func (s sortAppCondition) Less(i, j int) bool {
	idx1 := s.componentPosition[s.appCondition.Components[i].ComponentName]
	idx2 := s.componentPosition[s.appCondition.Components[j].ComponentName]
	return idx1 < idx2
}
