/*
Copyright 2021 The KubeVela Authors.

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

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	types2 "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/podutils"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"

	helmreleaseapi "github.com/fluxcd/helm-controller/api/v2beta1"
	helmrepoapi "github.com/fluxcd/source-controller/api/v1beta2"
)

// relationshipKey is the configmap key of relationShip rule
var relationshipKey = "rules"

// set the iterator max depth is 5
var maxDepth = 5

// RuleList the rule list
type RuleList []ChildrenResourcesRule

// GetRule get the rule by the resource type
func (rl *RuleList) GetRule(grt GroupResourceType) (*ChildrenResourcesRule, bool) {
	for i, r := range *rl {
		if r.GroupResourceType == grt {
			return &(*rl)[i], true
		}
	}
	return nil, false
}

// globalRule define the whole relationShip rule
var globalRule RuleList

func init() {
	globalRule = append(globalRule,
		ChildrenResourcesRule{
			GroupResourceType: GroupResourceType{Group: "apps", Kind: "Deployment"},
			SubResources: buildSubResources([]*SubResourceSelector{
				{
					ResourceType: ResourceType{APIVersion: "apps/v1", Kind: "ReplicaSet"},
					listOptions:  defaultWorkloadLabelListOption,
				},
			}),
		},
		ChildrenResourcesRule{
			SubResources: buildSubResources([]*SubResourceSelector{
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "Pod"},
					listOptions:  defaultWorkloadLabelListOption,
				},
			}),
			GroupResourceType: GroupResourceType{Group: "apps", Kind: "ReplicaSet"},
		},
		ChildrenResourcesRule{
			SubResources: buildSubResources([]*SubResourceSelector{
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "Pod"},
					listOptions:  defaultWorkloadLabelListOption,
				},
			}),
			GroupResourceType: GroupResourceType{Group: "apps", Kind: "StatefulSet"},
		},
		ChildrenResourcesRule{
			SubResources: buildSubResources([]*SubResourceSelector{
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "Pod"},
					listOptions:  defaultWorkloadLabelListOption,
				},
			}),
			GroupResourceType: GroupResourceType{Group: "apps", Kind: "DaemonSet"},
		},
		ChildrenResourcesRule{
			GroupResourceType: GroupResourceType{Group: "batch", Kind: "Job"},
			SubResources: buildSubResources([]*SubResourceSelector{
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "Pod"},
					listOptions:  defaultWorkloadLabelListOption,
				},
			}),
		},
		ChildrenResourcesRule{
			GroupResourceType: GroupResourceType{Group: "", Kind: "Service"},
			SubResources: buildSubResources([]*SubResourceSelector{
				{
					ResourceType: ResourceType{APIVersion: "discovery.k8s.io/v1beta1", Kind: "EndpointSlice"},
				},
				{
					ResourceType: ResourceType{APIVersion: "discovery.k8s.io/v1", Kind: "EndpointSlice"},
				},
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "Endpoints"},
					listOptions:  service2EndpointListOption,
				},
			}),
		},
		ChildrenResourcesRule{
			GroupResourceType: GroupResourceType{Group: "helm.toolkit.fluxcd.io", Kind: "HelmRelease"},
			SubResources: buildSubResources([]*SubResourceSelector{
				{
					ResourceType: ResourceType{APIVersion: "apps/v1", Kind: "Deployment"},
				},
				{
					ResourceType: ResourceType{APIVersion: "apps/v1", Kind: "StatefulSet"},
				},
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "ConfigMap"},
				},
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "Secret"},
				},
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "Service"},
				},
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "PersistentVolumeClaim"},
				},
				{
					ResourceType: ResourceType{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"},
				},
				{
					ResourceType: ResourceType{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "HTTPRoute"},
				},
				{
					ResourceType: ResourceType{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "Gateway"},
				},
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "ServiceAccount"},
				},
				{
					ResourceType: ResourceType{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role"},
				},
				{
					ResourceType: ResourceType{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding"},
				},
			}),
			DefaultGenListOptionFunc:      helmRelease2AnyListOption,
			DisableFilterByOwnerReference: true,
		},
		ChildrenResourcesRule{
			GroupResourceType: GroupResourceType{Group: "kustomize.toolkit.fluxcd.io", Kind: "Kustomization"},
			SubResources: buildSubResources([]*SubResourceSelector{
				{
					ResourceType: ResourceType{APIVersion: "apps/v1", Kind: "Deployment"},
				},
				{
					ResourceType: ResourceType{APIVersion: "apps/v1", Kind: "StatefulSet"},
				},
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "ConfigMap"},
				},
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "Secret"},
				},
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "Service"},
				},
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "PersistentVolumeClaim"},
				},
				{
					ResourceType: ResourceType{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"},
				},
				{
					ResourceType: ResourceType{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "HTTPRoute"},
				},
				{
					ResourceType: ResourceType{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "Gateway"},
				},
				{
					ResourceType: ResourceType{APIVersion: "v1", Kind: "ServiceAccount"},
				},
				{
					ResourceType: ResourceType{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role"},
				},
				{
					ResourceType: ResourceType{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding"},
				},
			}),
			DefaultGenListOptionFunc:      kustomization2AnyListOption,
			DisableFilterByOwnerReference: true,
		},
		ChildrenResourcesRule{
			SubResources: buildSubResources([]*SubResourceSelector{
				{
					ResourceType: ResourceType{APIVersion: "batch/v1", Kind: "Job"},
					listOptions:  cronJobLabelListOption,
				},
			}),
			GroupResourceType: GroupResourceType{Group: "batch", Kind: "CronJob"},
		},
	)
}

// GroupResourceType define the parent resource type
type GroupResourceType struct {
	Group string `json:"group"`
	Kind  string `json:"kind"`
}

// ResourceType define the children resource type
type ResourceType struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

// customRule define the customize rule created by user
type customRule struct {
	ParentResourceType   *GroupResourceType `json:"parentResourceType,omitempty"`
	ChildrenResourceType []CustomSelector   `json:"childrenResourceType,omitempty"`
}

// CustomSelector the custom resource selector configuration in configmap. support set the default label selector policy
type CustomSelector struct {
	ResourceType `json:",inline"`
	// defaultLabelSelector means read the label selector condition from the spec.selector.
	DefaultLabelSelector bool `json:"defaultLabelSelector"`
}

// ChildrenResourcesRule define the relationShip between parentObject and children resource
type ChildrenResourcesRule struct {
	// GroupResourceType the root resource type
	GroupResourceType GroupResourceType
	// every subResourceType can have a specified genListOptionFunc.
	SubResources *SubResources
	// if specified genListOptionFunc is nil will use use default genListOptionFunc to generate listOption.
	DefaultGenListOptionFunc genListOptionFunc
	// DisableFilterByOwnerReference means don't use parent resource's UID filter the result.
	DisableFilterByOwnerReference bool
}

func buildSubResources(crs []*SubResourceSelector) *SubResources {
	var cr SubResources = crs
	return &cr
}

func buildSubResourceSelector(cus CustomSelector) *SubResourceSelector {
	cr := SubResourceSelector{
		ResourceType: cus.ResourceType,
	}
	if cus.DefaultLabelSelector {
		cr.listOptions = defaultWorkloadLabelListOption
	}
	return &cr
}

// SubResources the sub resource definitions
type SubResources []*SubResourceSelector

// Get get the sub resource by the resource type
func (c *SubResources) Get(rt ResourceType) *SubResourceSelector {
	for _, r := range *c {
		if r.ResourceType == rt {
			return r
		}
	}
	return nil
}

// Put add a sub resource to the list
func (c *SubResources) Put(cr *SubResourceSelector) {
	*c = append(*c, cr)
}

// SubResourceSelector the sub resource selector configuration
type SubResourceSelector struct {
	ResourceType
	listOptions genListOptionFunc
}

type genListOptionFunc func(unstructured.Unstructured) (client.ListOptions, error)

// WorkloadUnstructured the workload unstructured, such as Deployment、Job、StatefulSet、ReplicaSet and DaemonSet
type WorkloadUnstructured struct {
	unstructured.Unstructured
}

// GetSelector get the selector from the field path
func (w *WorkloadUnstructured) GetSelector(fields ...string) (labels.Selector, error) {
	value, exist, err := unstructured.NestedFieldNoCopy(w.Object, fields...)
	if err != nil {
		return nil, err
	}
	if !exist {
		return labels.Everything(), nil
	}
	if v, ok := value.(map[string]interface{}); ok {
		var selector v1.LabelSelector
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(v, &selector); err != nil {
			return nil, err
		}
		return v1.LabelSelectorAsSelector(&selector)
	}
	return labels.Everything(), nil
}

func (w *WorkloadUnstructured) convertLabel2Selector(fields ...string) (labels.Selector, error) {
	value, exist, err := unstructured.NestedFieldNoCopy(w.Object, fields...)
	if err != nil {
		return nil, err
	}
	if !exist {
		return labels.Everything(), nil
	}
	if v, ok := value.(map[string]interface{}); ok {
		var selector v1.LabelSelector
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(v, &selector.MatchLabels); err != nil {
			return nil, err
		}
		return v1.LabelSelectorAsSelector(&selector)
	}
	return labels.Everything(), nil
}

var defaultWorkloadLabelListOption genListOptionFunc = func(obj unstructured.Unstructured) (client.ListOptions, error) {
	workload := WorkloadUnstructured{obj}
	deploySelector, err := workload.GetSelector("spec", "selector")
	if err != nil {
		return client.ListOptions{}, err
	}
	return client.ListOptions{Namespace: obj.GetNamespace(), LabelSelector: deploySelector}, nil
}

var service2EndpointListOption = func(obj unstructured.Unstructured) (client.ListOptions, error) {
	svc := v12.Service{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &svc)
	if err != nil {
		return client.ListOptions{}, err
	}
	stsSelector, err := v1.LabelSelectorAsSelector(&v1.LabelSelector{MatchLabels: svc.Labels})
	if err != nil {
		return client.ListOptions{}, err
	}
	return client.ListOptions{Namespace: svc.Namespace, LabelSelector: stsSelector}, nil
}

var cronJobLabelListOption = func(obj unstructured.Unstructured) (client.ListOptions, error) {
	workload := WorkloadUnstructured{obj}
	cronJobSelector, err := workload.convertLabel2Selector("spec", "jobTemplate", "metadata", "labels")
	if err != nil {
		return client.ListOptions{}, err
	}
	return client.ListOptions{Namespace: obj.GetNamespace(), LabelSelector: cronJobSelector}, nil
}

var helmRelease2AnyListOption = func(obj unstructured.Unstructured) (client.ListOptions, error) {
	hrSelector, err := v1.LabelSelectorAsSelector(&v1.LabelSelector{MatchLabels: map[string]string{
		"helm.toolkit.fluxcd.io/name":      obj.GetName(),
		"helm.toolkit.fluxcd.io/namespace": obj.GetNamespace(),
	}})
	if err != nil {
		return client.ListOptions{}, err
	}
	return client.ListOptions{LabelSelector: hrSelector}, nil
}

var kustomization2AnyListOption = func(obj unstructured.Unstructured) (client.ListOptions, error) {
	kusSelector, err := v1.LabelSelectorAsSelector(&v1.LabelSelector{MatchLabels: map[string]string{
		"kustomize.toolkit.fluxcd.io/name":      obj.GetName(),
		"kustomize.toolkit.fluxcd.io/namespace": obj.GetNamespace(),
	}})
	if err != nil {
		return client.ListOptions{}, err
	}
	return client.ListOptions{LabelSelector: kusSelector}, nil
}

type healthyCheckFunc func(obj unstructured.Unstructured) (*types.HealthStatus, error)

var checkPodStatus = func(obj unstructured.Unstructured) (*types.HealthStatus, error) {
	var pod v12.Pod
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pod)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured Pod to typed: %w", err)
	}

	getFailMessage := func(ctr *v12.ContainerStatus) string {
		if ctr.State.Terminated != nil {
			if ctr.State.Terminated.Message != "" {
				return ctr.State.Terminated.Message
			}
			if ctr.State.Terminated.Reason == "OOMKilled" {
				return ctr.State.Terminated.Reason
			}
			if ctr.State.Terminated.ExitCode != 0 {
				return fmt.Sprintf("container %q failed with exit code %d", ctr.Name, ctr.State.Terminated.ExitCode)
			}
		}
		return ""
	}

	switch pod.Status.Phase {
	case v12.PodSucceeded:
		return &types.HealthStatus{
			Status:  types.HealthStatusHealthy,
			Reason:  pod.Status.Reason,
			Message: pod.Status.Message,
		}, nil
	case v12.PodRunning:
		switch pod.Spec.RestartPolicy {
		case v12.RestartPolicyAlways:
			// if pod is ready, it is automatically healthy
			if podutils.IsPodReady(&pod) {
				return &types.HealthStatus{
					Status: types.HealthStatusHealthy,
					Reason: "all containers are ready",
				}, nil
			}
			// if it's not ready, check to see if any container terminated, if so, it's unhealthy
			for _, ctrStatus := range pod.Status.ContainerStatuses {
				if ctrStatus.LastTerminationState.Terminated != nil {
					return &types.HealthStatus{
						Status:  types.HealthStatusUnHealthy,
						Reason:  pod.Status.Reason,
						Message: pod.Status.Message,
					}, nil
				}
			}
			// otherwise we are progressing towards a ready state
			return &types.HealthStatus{
				Status:  types.HealthStatusProgressing,
				Reason:  pod.Status.Reason,
				Message: pod.Status.Message,
			}, nil
		case v12.RestartPolicyOnFailure, v12.RestartPolicyNever:
			// pods set with a restart policy of OnFailure or Never, have a finite life.
			// These pods are typically resource hooks. Thus, we consider these as Progressing
			// instead of healthy.
			return &types.HealthStatus{
				Status:  types.HealthStatusProgressing,
				Reason:  pod.Status.Reason,
				Message: pod.Status.Message,
			}, nil
		}
	case v12.PodPending:
		return &types.HealthStatus{
			Status:  types.HealthStatusProgressing,
			Message: pod.Status.Message,
		}, nil
	case v12.PodFailed:
		if pod.Status.Message != "" {
			// Pod has a nice error message. Use that.
			return &types.HealthStatus{Status: types.HealthStatusUnHealthy, Message: pod.Status.Message}, nil
		}
		for _, ctr := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
			if msg := getFailMessage(ctr.DeepCopy()); msg != "" {
				return &types.HealthStatus{Status: types.HealthStatusUnHealthy, Message: msg}, nil
			}
		}
		return &types.HealthStatus{Status: types.HealthStatusUnHealthy, Message: ""}, nil
	default:
	}
	return &types.HealthStatus{
		Status:  types.HealthStatusUnKnown,
		Reason:  string(pod.Status.Phase),
		Message: pod.Status.Message,
	}, nil
}

var checkHelmReleaseStatus = func(obj unstructured.Unstructured) (*types.HealthStatus, error) {
	helmRelease := &helmreleaseapi.HelmRelease{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &helmRelease)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured helmRelease to typed: %w", err)
	}
	if len(helmRelease.Status.Conditions) != 0 {
		for _, condition := range helmRelease.Status.Conditions {
			if condition.Type == "Ready" {
				if condition.Status == v1.ConditionTrue {
					return &types.HealthStatus{
						Status: types.HealthStatusHealthy,
					}, nil
				}
				return &types.HealthStatus{
					Status:  types.HealthStatusUnHealthy,
					Message: condition.Message,
				}, nil
			}
		}
	}
	return &types.HealthStatus{
		Status: types.HealthStatusUnKnown,
	}, nil
}

var checkHelmRepoStatus = func(obj unstructured.Unstructured) (*types.HealthStatus, error) {
	helmRepo := helmrepoapi.HelmRepository{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &helmRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured helmRelease to typed: %w", err)
	}
	if len(helmRepo.Status.Conditions) != 0 {
		for _, condition := range helmRepo.Status.Conditions {
			if condition.Type == "Ready" {
				if condition.Status == v1.ConditionTrue {
					return &types.HealthStatus{
						Status:  types.HealthStatusHealthy,
						Message: condition.Message,
					}, nil
				}
				return &types.HealthStatus{
					Status:  types.HealthStatusUnHealthy,
					Message: condition.Message,
				}, nil
			}
		}
	}
	return &types.HealthStatus{
		Status: types.HealthStatusUnKnown,
	}, nil
}

var checkReplicaSetStatus = func(obj unstructured.Unstructured) (*types.HealthStatus, error) {
	replicaSet := appsv1.ReplicaSet{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &replicaSet)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured ReplicaSet to typed: %w", err)
	}
	if replicaSet.Generation <= replicaSet.Status.ObservedGeneration {
		cond := getAppsv1ReplicaSetCondition(replicaSet.Status, appsv1.ReplicaSetReplicaFailure)
		if cond != nil && cond.Status == v12.ConditionTrue {
			return &types.HealthStatus{
				Status:  types.HealthStatusUnHealthy,
				Reason:  cond.Reason,
				Message: cond.Message,
			}, nil
		} else if replicaSet.Spec.Replicas != nil && replicaSet.Status.AvailableReplicas < *replicaSet.Spec.Replicas {
			return &types.HealthStatus{
				Status:  types.HealthStatusProgressing,
				Message: fmt.Sprintf("Waiting for rollout to finish: %d out of %d new replicas are available...", replicaSet.Status.AvailableReplicas, *replicaSet.Spec.Replicas),
			}, nil
		}
	} else {
		return &types.HealthStatus{
			Status:  types.HealthStatusProgressing,
			Message: "Waiting for rollout to finish: observed replica set generation less then desired generation",
		}, nil
	}

	return &types.HealthStatus{
		Status: types.HealthStatusHealthy,
	}, nil
}

func getAppsv1ReplicaSetCondition(status appsv1.ReplicaSetStatus, condType appsv1.ReplicaSetConditionType) *appsv1.ReplicaSetCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

var checkPVCHealthStatus = func(obj unstructured.Unstructured) (*types.HealthStatus, error) {
	pvc := v12.PersistentVolumeClaim{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pvc)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured PVC to typed: %w", err)
	}
	var status types.HealthStatusCode
	switch pvc.Status.Phase {
	case v12.ClaimLost:
		status = types.HealthStatusUnHealthy
	case v12.ClaimPending:
		status = types.HealthStatusProgressing
	case v12.ClaimBound:
		status = types.HealthStatusHealthy
	default:
		status = types.HealthStatusUnKnown
	}
	return &types.HealthStatus{Status: status}, nil
}

var checkServiceStatus = func(obj unstructured.Unstructured) (*types.HealthStatus, error) {
	svc := v12.Service{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &svc)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured service to typed: %w", err)
	}
	health := types.HealthStatus{Status: types.HealthStatusHealthy}
	if svc.Spec.Type == v12.ServiceTypeLoadBalancer {
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			health.Status = types.HealthStatusHealthy
		} else {
			health.Status = types.HealthStatusProgressing
		}
	}
	return &health, nil
}

// CheckResourceStatus return object status data
func CheckResourceStatus(obj unstructured.Unstructured) (*types.HealthStatus, error) {
	group := obj.GroupVersionKind().Group
	kind := obj.GroupVersionKind().Kind
	var checkFunc healthyCheckFunc
	switch group {
	case "":
		switch kind {
		case "Pod":
			checkFunc = checkPodStatus
		case "Service":
			checkFunc = checkServiceStatus
		case "PersistentVolumeClaim":
			checkFunc = checkPVCHealthStatus
		}
	case "apps":
		switch kind {
		case "ReplicaSet":
			checkFunc = checkReplicaSetStatus
		default:
		}
	case "helm.toolkit.fluxcd.io":
		switch kind {
		case "HelmRelease":
			checkFunc = checkHelmReleaseStatus
		default:
		}
	case "source.toolkit.fluxcd.io":
		switch kind {
		case "HelmRepository":
			checkFunc = checkHelmRepoStatus
		default:
		}
	default:
	}
	if checkFunc != nil {
		return checkFunc(obj)
	}
	return &types.HealthStatus{Status: types.HealthStatusHealthy}, nil
}

type additionalInfoFunc func(obj unstructured.Unstructured) (map[string]interface{}, error)

func additionalInfo(obj unstructured.Unstructured) (map[string]interface{}, error) {
	group := obj.GroupVersionKind().Group
	kind := obj.GroupVersionKind().Kind
	var infoFunc additionalInfoFunc
	switch group {
	case "":
		switch kind {
		case "Pod":
			infoFunc = podAdditionalInfo
		case "Service":
			infoFunc = svcAdditionalInfo
		}
	case "apps":
		switch kind {
		case "Deployment":
			infoFunc = deploymentAdditionalInfo
		case "StatefulSet":
			infoFunc = statefulSetAdditionalInfo
		default:
		}
	default:
	}
	if infoFunc != nil {
		return infoFunc(obj)
	}
	return nil, nil
}

func svcAdditionalInfo(obj unstructured.Unstructured) (map[string]interface{}, error) {
	svc := v12.Service{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &svc)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured svc to typed: %w", err)
	}
	if svc.Spec.Type == v12.ServiceTypeLoadBalancer {
		var eip string
		for _, ingress := range svc.Status.LoadBalancer.Ingress {
			if len(ingress.IP) != 0 {
				eip = ingress.IP
			}
		}
		if len(eip) == 0 {
			eip = "pending"
		}
		return map[string]interface{}{
			"EIP": eip,
		}, nil
	}
	return nil, nil
}

// the logic of this func totaly copy from the source-code of kubernetes tableConvertor
// https://github.com/kubernetes/kubernetes/blob/ea0764452222146c47ec826977f49d7001b0ea8c/pkg/printers/internalversion/printers.go#L740
// The result is same with the output of kubectl.
// nolint
func podAdditionalInfo(obj unstructured.Unstructured) (map[string]interface{}, error) {
	pod := v12.Pod{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pod)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured Pod to typed: %w", err)
	}

	hasPodReadyCondition := func(conditions []v12.PodCondition) bool {
		for _, condition := range conditions {
			if condition.Type == v12.PodReady && condition.Status == v12.ConditionTrue {
				return true
			}
		}
		return false
	}

	restarts := 0
	totalContainers := len(pod.Spec.Containers)
	readyContainers := 0

	reason := string(pod.Status.Phase)
	if pod.Status.Reason != "" {
		reason = pod.Status.Reason
	}

	initializing := false
	for i := range pod.Status.InitContainerStatuses {
		container := pod.Status.InitContainerStatuses[i]
		restarts += int(container.RestartCount)
		switch {
		case container.State.Terminated != nil && container.State.Terminated.ExitCode == 0:
			continue
		case container.State.Terminated != nil:
			// initialization is failed
			if len(container.State.Terminated.Reason) == 0 {
				if container.State.Terminated.Signal != 0 {
					reason = fmt.Sprintf("Init:Signal:%d", container.State.Terminated.Signal)
				} else {
					reason = fmt.Sprintf("Init:ExitCode:%d", container.State.Terminated.ExitCode)
				}
			} else {
				reason = "Init:" + container.State.Terminated.Reason
			}
			initializing = true
		case container.State.Waiting != nil && len(container.State.Waiting.Reason) > 0 && container.State.Waiting.Reason != "PodInitializing":
			reason = "Init:" + container.State.Waiting.Reason
			initializing = true
		default:
			reason = fmt.Sprintf("Init:%d/%d", i, len(pod.Spec.InitContainers))
			initializing = true
		}
		break
	}
	if !initializing {
		restarts = 0
		hasRunning := false
		for i := len(pod.Status.ContainerStatuses) - 1; i >= 0; i-- {
			container := pod.Status.ContainerStatuses[i]

			restarts += int(container.RestartCount)
			if container.State.Waiting != nil && container.State.Waiting.Reason != "" {
				reason = container.State.Waiting.Reason
			} else if container.State.Terminated != nil && container.State.Terminated.Reason != "" {
				reason = container.State.Terminated.Reason
			} else if container.State.Terminated != nil && container.State.Terminated.Reason == "" {
				if container.State.Terminated.Signal != 0 {
					reason = fmt.Sprintf("Signal:%d", container.State.Terminated.Signal)
				} else {
					reason = fmt.Sprintf("ExitCode:%d", container.State.Terminated.ExitCode)
				}
			} else if container.Ready && container.State.Running != nil {
				hasRunning = true
				readyContainers++
			}
		}

		// change pod status back to "Running" if there is at least one container still reporting as "Running" status
		if reason == "Completed" && hasRunning {
			if hasPodReadyCondition(pod.Status.Conditions) {
				reason = "Running"
			} else {
				reason = "NotReady"
			}
		}
	}

	if pod.DeletionTimestamp != nil && pod.Status.Reason == "NodeLost" {
		reason = "Unknown"
	} else if pod.DeletionTimestamp != nil {
		reason = "Terminating"
	}
	return map[string]interface{}{
		"Ready":    fmt.Sprintf("%d/%d", readyContainers, totalContainers),
		"Status":   reason,
		"Restarts": restarts,
		"Age":      translateTimestampSince(pod.CreationTimestamp),
	}, nil
}

func deploymentAdditionalInfo(obj unstructured.Unstructured) (map[string]interface{}, error) {
	deployment := appsv1.Deployment{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &deployment)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured Deployment to typed: %w", err)
	}

	readyReplicas := deployment.Status.ReadyReplicas
	desiredReplicas := deployment.Spec.Replicas
	updatedReplicas := deployment.Status.UpdatedReplicas
	availableReplicas := deployment.Status.AvailableReplicas

	return map[string]interface{}{
		"Ready":     fmt.Sprintf("%d/%d", readyReplicas, *desiredReplicas),
		"Update":    updatedReplicas,
		"Available": availableReplicas,
		"Age":       translateTimestampSince(deployment.CreationTimestamp),
	}, nil
}

func statefulSetAdditionalInfo(obj unstructured.Unstructured) (map[string]interface{}, error) {
	statefulSet := appsv1.StatefulSet{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &statefulSet)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured StatefulSet to typed: %w", err)
	}

	readyReplicas := statefulSet.Status.ReadyReplicas
	desiredReplicas := statefulSet.Spec.Replicas

	return map[string]interface{}{
		"Ready": fmt.Sprintf("%d/%d", readyReplicas, *desiredReplicas),
		"Age":   translateTimestampSince(statefulSet.CreationTimestamp),
	}, nil
}

func fetchObjectWithResourceTreeNode(ctx context.Context, cluster string, k8sClient client.Client, resource types.ResourceTreeNode) (*unstructured.Unstructured, error) {
	o := unstructured.Unstructured{}
	o.SetAPIVersion(resource.APIVersion)
	o.SetKind(resource.Kind)
	o.SetNamespace(resource.Namespace)
	o.SetName(resource.Name)
	err := k8sClient.Get(multicluster.ContextWithClusterName(ctx, cluster), types2.NamespacedName{Namespace: resource.Namespace, Name: resource.Name}, &o)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func listItemByRule(clusterCTX context.Context, k8sClient client.Client, resource ResourceType,
	parentObject unstructured.Unstructured, specifiedFunc genListOptionFunc, defaultFunc genListOptionFunc, disableFilterByOwner bool) ([]unstructured.Unstructured, error) {

	itemList := unstructured.UnstructuredList{}
	itemList.SetAPIVersion(resource.APIVersion)
	itemList.SetKind(fmt.Sprintf("%sList", resource.Kind))
	var err error
	if specifiedFunc == nil && defaultFunc == nil {
		// if the relationShip between parent and child hasn't defined by any genListOption, list all subResource and filter by ownerReference UID
		err = k8sClient.List(clusterCTX, &itemList)
		if err != nil {
			return nil, err
		}
		var res []unstructured.Unstructured
		for _, item := range itemList.Items {
			for _, reference := range item.GetOwnerReferences() {
				if reference.UID == parentObject.GetUID() {
					res = append(res, item)
				}
			}
		}
		sort.Slice(res, func(i, j int) bool {
			return res[i].GetName() < res[j].GetName()
		})
		return res, nil
	}
	var listOptions client.ListOptions
	if specifiedFunc != nil {
		//  specified func will override the default func
		listOptions, err = specifiedFunc(parentObject)
		if err != nil {
			return nil, err
		}
	} else {
		listOptions, err = defaultFunc(parentObject)
		if err != nil {
			return nil, err
		}
	}
	err = k8sClient.List(clusterCTX, &itemList, &listOptions)
	if err != nil {
		return nil, err
	}
	if !disableFilterByOwner {
		var res []unstructured.Unstructured
		for _, item := range itemList.Items {
			if len(item.GetOwnerReferences()) == 0 {
				res = append(res, item)
			}
			for _, reference := range item.GetOwnerReferences() {
				if reference.UID == parentObject.GetUID() {
					res = append(res, item)
				}
			}
		}
		return res, nil
	}
	sort.Slice(itemList.Items, func(i, j int) bool {
		return itemList.Items[i].GetName() < itemList.Items[j].GetName()
	})
	return itemList.Items, nil
}

func iterateListSubResources(ctx context.Context, cluster string, k8sClient client.Client, parentResource types.ResourceTreeNode, depth int, filter func(node types.ResourceTreeNode) bool) ([]*types.ResourceTreeNode, error) {
	if depth > maxDepth {
		klog.Warningf("listing application resource tree has reached the max-depth %d parentObject is %v", depth, parentResource)
		return nil, nil
	}
	parentObject, err := fetchObjectWithResourceTreeNode(ctx, cluster, k8sClient, parentResource)
	if err != nil {
		return nil, err
	}
	group := parentObject.GetObjectKind().GroupVersionKind().Group
	kind := parentObject.GetObjectKind().GroupVersionKind().Kind

	if rule, ok := globalRule.GetRule(GroupResourceType{Group: group, Kind: kind}); ok {
		var resList []*types.ResourceTreeNode
		for i := range *rule.SubResources {
			resource := (*rule.SubResources)[i].ResourceType
			specifiedFunc := (*rule.SubResources)[i].listOptions

			clusterCTX := multicluster.ContextWithClusterName(ctx, cluster)
			items, err := listItemByRule(clusterCTX, k8sClient, resource, *parentObject, specifiedFunc, rule.DefaultGenListOptionFunc, rule.DisableFilterByOwnerReference)
			if err != nil {
				if meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err) || kerrors.IsNotFound(err) {
					klog.Warningf("ignore list resources: %s as %v", resource.Kind, err)
					continue
				}
				return nil, err
			}
			for i, item := range items {
				rtn := types.ResourceTreeNode{
					APIVersion: item.GetAPIVersion(),
					Kind:       item.GroupVersionKind().Kind,
					Namespace:  item.GetNamespace(),
					Name:       item.GetName(),
					UID:        item.GetUID(),
					Cluster:    cluster,
					Object:     items[i],
				}
				if _, ok := globalRule.GetRule(GroupResourceType{Group: item.GetObjectKind().GroupVersionKind().Group, Kind: item.GetObjectKind().GroupVersionKind().Kind}); ok {
					childrenRes, err := iterateListSubResources(ctx, cluster, k8sClient, rtn, depth+1, filter)
					if err != nil {
						return nil, err
					}
					rtn.LeafNodes = childrenRes
				}
				if !filter(rtn) && len(rtn.LeafNodes) == 0 {
					continue
				}
				healthStatus, err := CheckResourceStatus(item)
				if err != nil {
					return nil, err
				}
				rtn.HealthStatus = *healthStatus
				addInfo, err := additionalInfo(item)
				if err != nil {
					return nil, err
				}
				rtn.CreationTimestamp = item.GetCreationTimestamp().Time
				if !item.GetDeletionTimestamp().IsZero() {
					rtn.DeletionTimestamp = item.GetDeletionTimestamp().Time
				}
				rtn.AdditionalInfo = addInfo
				resList = append(resList, &rtn)
			}
		}
		return resList, nil
	}
	return nil, nil
}

// mergeCustomRules merge user defined resource topology rules with the system ones
func mergeCustomRules(ctx context.Context, k8sClient client.Client) error {
	rulesList := v12.ConfigMapList{}
	if err := k8sClient.List(ctx, &rulesList, client.InNamespace(velatypes.DefaultKubeVelaNS), client.HasLabels{oam.LabelResourceRules}); err != nil {
		return client.IgnoreNotFound(err)
	}
	for _, item := range rulesList.Items {
		ruleStr := item.Data[relationshipKey]
		var (
			customRules []*customRule
			format      string
			err         error
		)
		if item.Labels != nil {
			format = item.Labels[oam.LabelResourceRuleFormat]
		}
		switch format {
		case oam.ResourceTopologyFormatJSON:
			err = json.Unmarshal([]byte(ruleStr), &customRules)
		case oam.ResourceTopologyFormatYAML, "":
			err = yaml.Unmarshal([]byte(ruleStr), &customRules)
		}
		if err != nil {
			// don't let one miss-config configmap brake whole process
			klog.Errorf("relationship rule configmap %s miss config %v", item.Name, err)
			continue
		}
		for _, rule := range customRules {

			if cResource, ok := globalRule.GetRule(*rule.ParentResourceType); ok {
				for i, resourceType := range rule.ChildrenResourceType {
					if cResource.SubResources.Get(resourceType.ResourceType) == nil {
						cResource.SubResources.Put(buildSubResourceSelector(rule.ChildrenResourceType[i]))
					}
				}
			} else {
				var subResources []*SubResourceSelector
				for i := range rule.ChildrenResourceType {
					subResources = append(subResources, buildSubResourceSelector(rule.ChildrenResourceType[i]))
				}
				globalRule = append(globalRule, ChildrenResourcesRule{
					GroupResourceType:        *rule.ParentResourceType,
					DefaultGenListOptionFunc: nil,
					SubResources:             buildSubResources(subResources)})
			}
		}
	}
	return nil
}

func translateTimestampSince(timestamp v1.Time) string {
	if timestamp.IsZero() {
		return "<unknown>"
	}

	return duration.HumanDuration(time.Since(timestamp.Time))
}
