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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
)

const (
	infoFmtUnknownWorkload    = "APIVersion %v Kind %v workload is unknown for HealthScope "
	infoFmtReady              = "Ready:%d/%d "
	infoFmtNoChildRes         = "cannot get child resource references of workload %v"
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
	kindContainerizedWorkload = corev1alpha2.ContainerizedWorkloadKind
	kindDeployment            = reflect.TypeOf(apps.Deployment{}).Name()
	kindService               = reflect.TypeOf(core.Service{}).Name()
	kindStatefulSet           = reflect.TypeOf(apps.StatefulSet{}).Name()
	kindDaemonSet             = reflect.TypeOf(apps.DaemonSet{}).Name()
)

// WorkloadHealthCondition holds health status of any resource
type WorkloadHealthCondition = v1alpha2.WorkloadHealthCondition

// ScopeHealthCondition holds health condition of a scope
type ScopeHealthCondition = v1alpha2.ScopeHealthCondition

// A WorloadHealthChecker checks health status of specified resource
// and saves status into an HealthCondition object.
type WorloadHealthChecker interface {
	Check(context.Context, client.Client, runtimev1alpha1.TypedReference, string) *WorkloadHealthCondition
}

// WorkloadHealthCheckFn checks health status of specified resource
// and saves status into an HealthCondition object.
type WorkloadHealthCheckFn func(context.Context, client.Client, runtimev1alpha1.TypedReference, string) *WorkloadHealthCondition

// Check the health status of specified resource
func (fn WorkloadHealthCheckFn) Check(ctx context.Context, c client.Client, tr runtimev1alpha1.TypedReference, ns string) *WorkloadHealthCondition {
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

// CheckContainerziedWorkloadHealth check health condition of ContainerizedWorkload
func CheckContainerziedWorkloadHealth(ctx context.Context, c client.Client, ref runtimev1alpha1.TypedReference, namespace string) *WorkloadHealthCondition {
	if ref.GroupVersionKind() != corev1alpha2.SchemeGroupVersion.WithKind(kindContainerizedWorkload) {
		return nil
	}
	r := &WorkloadHealthCondition{
		HealthStatus:   StatusHealthy,
		TargetWorkload: ref,
	}

	cwObj := corev1alpha2.ContainerizedWorkload{}
	cwObj.SetGroupVersionKind(corev1alpha2.SchemeGroupVersion.WithKind(kindContainerizedWorkload))
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &cwObj); err != nil {
		r.HealthStatus = StatusUnhealthy
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}
	r.ComponentName = getComponentNameFromLabel(&cwObj)
	r.TargetWorkload.UID = cwObj.GetUID()

	childRefs := cwObj.Status.Resources
	updateChildResourcesCondition(ctx, c, namespace, r, ref, childRefs)
	return r
}

func updateChildResourcesCondition(ctx context.Context, c client.Client, namespace string, r *WorkloadHealthCondition, ref runtimev1alpha1.TypedReference, childRefs []runtimev1alpha1.TypedReference) {
	subConditions := []*WorkloadHealthCondition{}
	if len(childRefs) != 2 {
		// one deployment and one svc are required by containerizedworkload
		r.Diagnosis = fmt.Sprintf(infoFmtNoChildRes, ref.Name)
		r.HealthStatus = StatusUnhealthy
		return
	}
	for _, childRef := range childRefs {
		switch childRef.Kind {
		case kindDeployment:
			// reuse Deployment health checker
			childCondition := CheckDeploymentHealth(ctx, c, childRef, namespace)
			subConditions = append(subConditions, childCondition)
		case kindService:
			childCondition := &WorkloadHealthCondition{
				TargetWorkload: childRef,
				HealthStatus:   StatusHealthy,
			}
			o := unstructured.Unstructured{}
			o.SetAPIVersion(childRef.APIVersion)
			o.SetKind(childRef.Kind)
			if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: childRef.Name}, &o); err != nil {
				childCondition.HealthStatus = StatusUnhealthy
				childCondition.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
			}
			subConditions = append(subConditions, childCondition)
		}
	}

	for _, sc := range subConditions {
		if sc.HealthStatus != StatusHealthy {
			r.HealthStatus = StatusUnhealthy
		}
		r.Diagnosis = fmt.Sprintf("%s%s", r.Diagnosis, sc.Diagnosis)
	}
}

// CheckDeploymentHealth checks health condition of Deployment
func CheckDeploymentHealth(ctx context.Context, client client.Client, ref runtimev1alpha1.TypedReference, namespace string) *WorkloadHealthCondition {
	if ref.GroupVersionKind() != apps.SchemeGroupVersion.WithKind(kindDeployment) {
		return nil
	}
	r := &WorkloadHealthCondition{
		HealthStatus:   StatusUnhealthy,
		TargetWorkload: ref,
	}
	deployment := apps.Deployment{}
	deployment.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindDeployment))
	deploymentRef := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := client.Get(ctx, deploymentRef, &deployment); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}
	r.ComponentName = getComponentNameFromLabel(&deployment)
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
func CheckStatefulsetHealth(ctx context.Context, client client.Client, ref runtimev1alpha1.TypedReference, namespace string) *WorkloadHealthCondition {
	if ref.GroupVersionKind() != apps.SchemeGroupVersion.WithKind(kindStatefulSet) {
		return nil
	}
	r := &WorkloadHealthCondition{
		HealthStatus:   StatusUnhealthy,
		TargetWorkload: ref,
	}
	statefulset := apps.StatefulSet{}
	statefulset.APIVersion = ref.APIVersion
	statefulset.Kind = ref.Kind
	nk := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := client.Get(ctx, nk, &statefulset); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}
	r.ComponentName = getComponentNameFromLabel(&statefulset)
	r.TargetWorkload.UID = statefulset.GetUID()
	requiredReplicas := int32(0)
	if statefulset.Spec.Replicas != nil {
		requiredReplicas = *statefulset.Spec.Replicas
	}
	r.Diagnosis = fmt.Sprintf(infoFmtReady, statefulset.Status.ReadyReplicas, requiredReplicas)

	// Health criteria
	if statefulset.Status.ReadyReplicas != requiredReplicas {
		return r
	}
	r.HealthStatus = StatusHealthy
	return r
}

// CheckDaemonsetHealth checks health condition of DaemonSet
func CheckDaemonsetHealth(ctx context.Context, client client.Client, ref runtimev1alpha1.TypedReference, namespace string) *WorkloadHealthCondition {
	if ref.GroupVersionKind() != apps.SchemeGroupVersion.WithKind(kindDaemonSet) {
		return nil
	}
	r := &WorkloadHealthCondition{
		HealthStatus:   StatusUnhealthy,
		TargetWorkload: ref,
	}
	daemonset := apps.DaemonSet{}
	daemonset.APIVersion = ref.APIVersion
	daemonset.Kind = ref.Kind
	nk := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := client.Get(ctx, nk, &daemonset); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}
	r.ComponentName = getComponentNameFromLabel(&daemonset)
	r.TargetWorkload.UID = daemonset.GetUID()
	r.Diagnosis = fmt.Sprintf(infoFmtReady, daemonset.Status.NumberReady, daemonset.Status.DesiredNumberScheduled)

	// Health criteria
	if daemonset.Status.NumberUnavailable != 0 {
		return r
	}
	r.HealthStatus = StatusHealthy
	return r
}

// CheckByHealthCheckTrait checks health condition through HealthCheckTrait.
func CheckByHealthCheckTrait(ctx context.Context, c client.Client, wlRef runtimev1alpha1.TypedReference, ns string) *WorkloadHealthCondition {
	// TODO(roywang) implement HealthCheckTrait feature
	return nil
}

// CheckUnknownWorkload handles unknown type workloads.
func CheckUnknownWorkload(ctx context.Context, c client.Client, wlRef runtimev1alpha1.TypedReference, ns string) *WorkloadHealthCondition {
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

func getVersioningPeerWorkloadRefs(ctx context.Context, c client.Reader, wlRef runtimev1alpha1.TypedReference, ns string) ([]runtimev1alpha1.TypedReference, error) {
	o := &unstructured.Unstructured{}
	o.SetGroupVersionKind(wlRef.GroupVersionKind())
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: wlRef.Name}, o); err != nil {
		return nil, err
	}

	compName := getComponentNameFromLabel(o)
	appName := getAppConfigNameFromLabel(o)
	if compName == "" || appName == "" {
		// if missing these lables, cannot get peer workloads
		return nil, nil
	}

	peerRefs := []runtimev1alpha1.TypedReference{}
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
		tmpRef := runtimev1alpha1.TypedReference{}
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
