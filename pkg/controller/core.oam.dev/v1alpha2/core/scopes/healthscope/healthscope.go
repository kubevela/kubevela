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
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/workflow/pkg/cue/process"
	terraformtypes "github.com/oam-dev/terraform-controller/api/types"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	oamtypes "github.com/oam-dev/kubevela/apis/types"
	af "github.com/oam-dev/kubevela/pkg/appfile"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	infoFmtUnknownWorkload    = "APIVersion %v Kind %v workload is unknown for HealthScope "
	infoFmtReady              = "Ready:%d/%d "
	errHealthCheck            = "error occurs in health check "
	errGetVersioningWorkloads = "error occurs when get versioning peer workloads refs"

	defaultTimeout = 10 * time.Second
)

// HealthStatus represents health status strings.
type HealthStatus = v1alpha2.HealthStatus

const (
	// StatusHealthy represents healthy status.
	StatusHealthy = v1alpha2.StatusHealthy
	// StatusUnhealthy represents unhealthy status.
	StatusUnhealthy = v1alpha2.StatusUnhealthy
	// StatusUnknown represents unknown status.
	StatusUnknown = v1alpha2.StatusUnknown
)

var (
	kindDeployment  = reflect.TypeOf(apps.Deployment{}).Name()
	kindService     = reflect.TypeOf(core.Service{}).Name()
	kindStatefulSet = reflect.TypeOf(apps.StatefulSet{}).Name()
	kindDaemonSet   = reflect.TypeOf(apps.DaemonSet{}).Name()
)

// AppHealthCondition holds health status of an application
type AppHealthCondition = v1alpha2.AppHealthCondition

// WorkloadHealthCondition holds health status of a workload
type WorkloadHealthCondition = v1alpha2.WorkloadHealthCondition

// TraitHealthCondition holds health status of a trait
type TraitHealthCondition = v1alpha2.TraitHealthCondition

// ScopeHealthCondition holds health condition of a scope
type ScopeHealthCondition = v1alpha2.ScopeHealthCondition

// A WorloadHealthChecker checks health status of specified resource
// and saves status into an HealthCondition object.
type WorloadHealthChecker interface {
	Check(context.Context, client.Client, core.ObjectReference, string) *WorkloadHealthCondition
}

// WorkloadHealthCheckFn checks health status of specified resource
// and saves status into an HealthCondition object.
type WorkloadHealthCheckFn func(context.Context, client.Client, core.ObjectReference, string) *WorkloadHealthCondition

// Check the health status of specified resource
func (fn WorkloadHealthCheckFn) Check(ctx context.Context, c client.Client, tr core.ObjectReference, ns string) *WorkloadHealthCondition {
	r := fn(ctx, c, tr, ns)
	if r == nil {
		return r
	}
	// check all workloads of a version-enabled component
	peerRefs, err := getVersioningPeerWorkloadRefs(ctx, c, tr, ns)
	if err != nil {
		r.HealthStatus = StatusUnhealthy
		r.Diagnosis = fmt.Sprintf("%s %s:%s",
			r.Diagnosis,
			errGetVersioningWorkloads,
			err.Error())
		return r
	}

	if len(peerRefs) > 0 {
		var peerHCs PeerHealthConditions
		for _, peerRef := range peerRefs {
			if peerHC := fn(ctx, c, peerRef, ns); peerHC != nil {
				peerHCs = append(peerHCs, *peerHC.DeepCopy())
			}
		}
		peerHCs.MergePeerWorkloadsConditions(r)
	}
	return r
}

// CheckDeploymentHealth checks health condition of Deployment
func CheckDeploymentHealth(ctx context.Context, client client.Client, ref core.ObjectReference, namespace string) *WorkloadHealthCondition {
	if ref.GroupVersionKind() != apps.SchemeGroupVersion.WithKind(kindDeployment) {
		return nil
	}
	r := &WorkloadHealthCondition{
		HealthStatus:   StatusUnhealthy,
		TargetWorkload: ref,
	}

	unstructuredDeployment := &unstructured.Unstructured{}
	unstructuredDeployment.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindDeployment))
	deploymentRef := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := client.Get(ctx, deploymentRef, unstructuredDeployment); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}

	deployment := new(apps.Deployment)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredDeployment.Object, deployment); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}

	r.ComponentName = getComponentNameFromLabel(deployment)
	r.TargetWorkload.UID = deployment.GetUID()

	requiredReplicas := int32(0)
	if deployment.Spec.Replicas != nil {
		requiredReplicas = *deployment.Spec.Replicas
	}

	r.Diagnosis = fmt.Sprintf(infoFmtReady, deployment.Status.ReadyReplicas, requiredReplicas)

	// Health criteria
	if deployment.Status.ReadyReplicas != requiredReplicas {
		return r
	}
	r.HealthStatus = StatusHealthy
	return r
}

// CheckStatefulsetHealth checks health condition of StatefulSet
func CheckStatefulsetHealth(ctx context.Context, client client.Client, ref core.ObjectReference, namespace string) *WorkloadHealthCondition {
	if ref.GroupVersionKind() != apps.SchemeGroupVersion.WithKind(kindStatefulSet) {
		return nil
	}
	r := &WorkloadHealthCondition{
		HealthStatus:   StatusUnhealthy,
		TargetWorkload: ref,
	}

	unstructuredStatefulSet := &unstructured.Unstructured{}
	unstructuredStatefulSet.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindStatefulSet))
	statefulSetRef := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := client.Get(ctx, statefulSetRef, unstructuredStatefulSet); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}

	statefulSet := new(apps.StatefulSet)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredStatefulSet.Object, statefulSet); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}

	r.ComponentName = getComponentNameFromLabel(statefulSet)
	r.TargetWorkload.UID = statefulSet.GetUID()
	requiredReplicas := int32(0)
	if statefulSet.Spec.Replicas != nil {
		requiredReplicas = *statefulSet.Spec.Replicas
	}
	r.Diagnosis = fmt.Sprintf(infoFmtReady, statefulSet.Status.ReadyReplicas, requiredReplicas)

	// Health criteria
	if statefulSet.Status.ReadyReplicas != requiredReplicas {
		return r
	}
	r.HealthStatus = StatusHealthy
	return r
}

// CheckDaemonsetHealth checks health condition of DaemonSet
func CheckDaemonsetHealth(ctx context.Context, client client.Client, ref core.ObjectReference, namespace string) *WorkloadHealthCondition {
	if ref.GroupVersionKind() != apps.SchemeGroupVersion.WithKind(kindDaemonSet) {
		return nil
	}
	r := &WorkloadHealthCondition{
		HealthStatus:   StatusUnhealthy,
		TargetWorkload: ref,
	}

	unstructuredDaemonSet := &unstructured.Unstructured{}
	unstructuredDaemonSet.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindDaemonSet))
	daemonSetRef := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := client.Get(ctx, daemonSetRef, unstructuredDaemonSet); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}

	daemonSet := new(apps.DaemonSet)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredDaemonSet.Object, daemonSet); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}

	r.ComponentName = getComponentNameFromLabel(daemonSet)
	r.TargetWorkload.UID = daemonSet.GetUID()
	r.Diagnosis = fmt.Sprintf(infoFmtReady, daemonSet.Status.NumberReady, daemonSet.Status.DesiredNumberScheduled)

	// Health criteria
	if daemonSet.Status.NumberUnavailable != 0 {
		return r
	}
	r.HealthStatus = StatusHealthy
	return r
}

// CheckByHealthCheckTrait checks health condition through HealthCheckTrait.
func CheckByHealthCheckTrait(ctx context.Context, c client.Client, wlRef core.ObjectReference, ns string) *WorkloadHealthCondition {
	// TODO(roywang) implement HealthCheckTrait feature
	return nil
}

// CheckUnknownWorkload handles unknown type workloads.
func CheckUnknownWorkload(ctx context.Context, c client.Client, wlRef core.ObjectReference, ns string) *WorkloadHealthCondition {
	healthCondition := &WorkloadHealthCondition{
		TargetWorkload: wlRef,
		HealthStatus:   StatusUnknown,
		Diagnosis:      fmt.Sprintf(infoFmtUnknownWorkload, wlRef.APIVersion, wlRef.Kind),
	}

	wl := &unstructured.Unstructured{}
	wl.SetGroupVersionKind(wlRef.GroupVersionKind())
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: wlRef.Name}, wl); err != nil {
		healthCondition.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return healthCondition
	}
	healthCondition.ComponentName = getComponentNameFromLabel(wl)

	// for unknown workloads, just show status instead of precise diagnosis
	wlStatus, _, _ := unstructured.NestedMap(wl.UnstructuredContent(), "status")
	wlStatusR, err := json.Marshal(wlStatus)
	if err != nil {
		healthCondition.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return healthCondition
	}
	healthCondition.WorkloadStatus = string(wlStatusR)
	return healthCondition
}

func getComponentNameFromLabel(o metav1.Object) string {
	if o == nil {
		return ""
	}
	compName, exist := o.GetLabels()[oam.LabelAppComponent]
	if !exist {
		compName = ""
	}
	return compName
}

func getAppConfigNameFromLabel(o metav1.Object) string {
	if o == nil {
		return ""
	}
	appName, exist := o.GetLabels()[oam.LabelAppName]
	if !exist {
		appName = ""
	}
	return appName
}

func getVersioningPeerWorkloadRefs(ctx context.Context, c client.Reader, wlRef core.ObjectReference, ns string) ([]core.ObjectReference, error) {
	o := &unstructured.Unstructured{}
	o.SetGroupVersionKind(wlRef.GroupVersionKind())
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: wlRef.Name}, o); err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	compName := getComponentNameFromLabel(o)
	appName := getAppConfigNameFromLabel(o)
	if compName == "" || appName == "" {
		// if missing these labels, cannot get peer workloads
		return nil, nil
	}

	peerRefs := []core.ObjectReference{}
	l := &unstructured.UnstructuredList{}
	l.SetGroupVersionKind(wlRef.GroupVersionKind())

	opts := []client.ListOption{
		client.InNamespace(ns),
		client.MatchingLabels{
			oam.LabelAppComponent: compName,
			oam.LabelAppName:      appName},
	}
	if err := c.List(ctx, l, opts...); err != nil {
		return nil, err
	}

	for _, obj := range l.Items {
		if obj.GetName() == o.GetName() {
			continue
		}
		tmpRef := core.ObjectReference{}
		tmpRef.SetGroupVersionKind(obj.GroupVersionKind())
		tmpRef.Name = obj.GetName()
		peerRefs = append(peerRefs, tmpRef)
	}
	return peerRefs, nil
}

// PeerHealthConditions refers to a slice of health condition of worloads
// belonging to one version-enabled component
type PeerHealthConditions []WorkloadHealthCondition

func (p PeerHealthConditions) Len() int { return len(p) }
func (p PeerHealthConditions) Less(i, j int) bool {
	// sort by revision number in descending order
	return extractRevision(p[i].TargetWorkload.Name) > extractRevision(p[j].TargetWorkload.Name)
}
func (p PeerHealthConditions) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

// exract revision number from revision name in format: <comp-name>-v<revision number>
// any non-qualified format should return 0
func extractRevision(c string) int {
	i, _ := strconv.ParseInt(c[strings.LastIndex(c, "v")+1:], 10, 0)
	return int(i)
}

// MergePeerWorkloadsConditions merge health conditions of all peer workloads into basic
func (p PeerHealthConditions) MergePeerWorkloadsConditions(basic *WorkloadHealthCondition) {
	if basic == nil || len(p) == 0 {
		return
	}
	// copy to keep idempotent
	peerHCs := make(PeerHealthConditions, len(p))
	copy(peerHCs, p)
	//nolint:makezero
	peerHCs = append(peerHCs, *basic.DeepCopy())

	// sort by revision number in descending order
	sort.Sort(peerHCs)

	for _, peerHC := range peerHCs {
		if peerHC.HealthStatus == StatusUnhealthy {
			// if ANY peer workload is unhealthy
			// then the overall condition is unhealthy
			basic.HealthStatus = StatusUnhealthy
		}
	}
	// re-format diagnosis/workloadStatus to show multiple workloads'
	if basic.HealthStatus == StatusUnknown {
		basic.WorkloadStatus = fmt.Sprintf("%s:%s", peerHCs[0].TargetWorkload.Name, peerHCs[0].WorkloadStatus)
		for _, peerHC := range peerHCs[1:] {
			basic.WorkloadStatus = fmt.Sprintf("%s %s:%s",
				basic.WorkloadStatus,
				peerHC.TargetWorkload.Name,
				peerHC.WorkloadStatus)
		}
	} else {
		basic.Diagnosis = fmt.Sprintf("%s:%s", peerHCs[0].TargetWorkload.Name, peerHCs[0].Diagnosis)
		for i, peerHC := range peerHCs[1:] {
			if i > 0 && peerHC.Diagnosis == fmt.Sprintf(infoFmtReady, 0, 0) {
				// skip timeworn ones
				continue
			}
			basic.Diagnosis = fmt.Sprintf("%s %s:%s",
				basic.Diagnosis,
				peerHC.TargetWorkload.Name,
				peerHC.Diagnosis)
		}
	}
}

// CUEBasedHealthCheck check workload and traits health through CUE-based health checking approach.
func CUEBasedHealthCheck(ctx context.Context, c client.Client, wlRef WorkloadReference, ns string, appfile *af.Appfile) (*WorkloadHealthCondition, []*TraitHealthCondition) {
	wlHealth := &WorkloadHealthCondition{
		TargetWorkload: wlRef.ObjectReference,
	}

	o := &unstructured.Unstructured{}
	o.SetGroupVersionKind(wlRef.GroupVersionKind())
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: wlRef.Name}, o); err != nil {
		wlHealth.HealthStatus = StatusUnhealthy
		wlHealth.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return wlHealth, nil
	}
	compName := getComponentNameFromLabel(o)
	wlHealth.ComponentName = compName

	var wl *af.Workload
	for _, v := range appfile.Workloads {
		if v.Name == compName {
			wl = v
			break
		}
	}
	if wl == nil {
		// almost impossible
		return nil, nil
	}

	var pCtx process.Context

	// if error occurs when check workload health, it's not allowed to check traits
	// because CUE-based health checking replies on valid process context
	okToCheckTrait := false

	func() {

		switch wl.CapabilityCategory {
		case oamtypes.TerraformCategory:
			ctx := context.Background()
			pCtx = af.NewBasicContext(af.GenerateContextDataFromAppFile(appfile, wl.Name), wl.Params)
			var configuration terraformapi.Configuration
			if err := c.Get(ctx, client.ObjectKey{Name: wl.Name, Namespace: ns}, &configuration); err != nil {
				wlHealth.HealthStatus = StatusUnhealthy
				wlHealth.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
			}
			if configuration.Status.Apply.State != terraformtypes.Available {
				wlHealth.HealthStatus = StatusUnhealthy
			} else {
				wlHealth.HealthStatus = StatusHealthy
			}
			wlHealth.Diagnosis = configuration.Status.Apply.Message
			okToCheckTrait = true
		default:
			pCtx = velaprocess.NewContext(af.GenerateContextDataFromAppFile(appfile, wl.Name))
			pCtx.SetCtx(ctx)
			if wl.CapabilityCategory != oamtypes.CUECategory {
				templateStr, err := af.GenerateCUETemplate(wl)
				if err != nil {
					wlHealth.HealthStatus = StatusUnhealthy
					wlHealth.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
					return
				}
				wl.FullTemplate.TemplateStr = templateStr
			}

			if err := wl.EvalContext(pCtx); err != nil {
				wlHealth.HealthStatus = StatusUnhealthy
				wlHealth.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
				return
			}
			// if workload has no CUE-based health template, skip check workload,
			// but still okay to check traits because process context is ready
			if len(wl.FullTemplate.Health) == 0 {
				wlHealth = nil
				okToCheckTrait = true
				return
			}
			accessor := util.NewApplicationResourceNamespaceAccessor(ns, "")
			templateContext, err := wl.GetTemplateContext(pCtx, c, accessor)
			if err != nil {
				wlHealth.HealthStatus = StatusUnhealthy
				wlHealth.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
				return
			}
			isHealthy, err := wl.EvalHealth(templateContext)
			if err != nil {
				wlHealth.HealthStatus = StatusUnhealthy
				wlHealth.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
				return
			}
			if isHealthy {
				wlHealth.HealthStatus = StatusHealthy
			} else {
				// TODO(wonderflow): we should add a custom way to let the template say why it's unhealthy, only a bool flag is not enough
				wlHealth.HealthStatus = StatusUnhealthy
			}
			wlHealth.CustomStatusMsg, err = wl.EvalStatus(templateContext)
			if err != nil {
				wlHealth.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
			}
			okToCheckTrait = true
		}
	}()

	traits := make([]*v1alpha2.TraitHealthCondition, len(wl.Traits))
	for i, tr := range wl.Traits {
		tHealth := &v1alpha2.TraitHealthCondition{
			Type: tr.Name,
		}
		if !okToCheckTrait {
			tHealth.HealthStatus = StatusUnknown
			tHealth.Diagnosis = "error occurs in checking workload health"
			traits[i] = tHealth
			continue
		}

		if len(tr.FullTemplate.Health) == 0 {
			tHealth.HealthStatus = StatusHealthy
			tHealth.Diagnosis = "no CUE-based health check template"
			traits[i] = tHealth
			continue
		}
		if err := tr.EvalContext(pCtx); err != nil {
			tHealth.HealthStatus = StatusUnhealthy
			tHealth.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
			traits[i] = tHealth
			continue
		}
		accessor := util.NewApplicationResourceNamespaceAccessor("", ns)
		templateContext, err := tr.GetTemplateContext(pCtx, c, accessor)
		if err != nil {
			tHealth.HealthStatus = StatusUnhealthy
			tHealth.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
			traits[i] = tHealth
			continue
		}
		isHealthy, err := tr.EvalHealth(templateContext)
		if err != nil {
			tHealth.HealthStatus = StatusUnhealthy
			tHealth.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
			traits[i] = tHealth
			continue
		}
		if isHealthy {
			tHealth.HealthStatus = StatusHealthy
		} else {
			// TODO(wonderflow): we should add a custom way to let the template say why it's unhealthy, only a bool flag is not enough
			tHealth.HealthStatus = StatusUnhealthy
		}
		tHealth.CustomStatusMsg, err = tr.EvalStatus(templateContext)
		if err != nil {
			tHealth.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		}
		traits[i] = tHealth
	}
	return wlHealth, traits
}
