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
	"fmt"

	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"

	appsv1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	types2 "k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/util/podutils"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
)

// set the iterator max depth is 5
var maxDepth = 5

// globalRule define the whole relationShip rule
var globalRule map[GroupResourceType]ChildrenResourcesRule

func init() {
	globalRule = make(map[GroupResourceType]ChildrenResourcesRule)
	globalRule[GroupResourceType{Group: "apps", Kind: "Deployment"}] = ChildrenResourcesRule{
		CareResource: map[ResourceType]genListOptionFunc{
			{APIVersion: "apps/v1", Kind: "ReplicaSet"}: deploy2RsLabelListOption,
		},
	}
	globalRule[GroupResourceType{Group: "apps", Kind: "ReplicaSet"}] = ChildrenResourcesRule{
		CareResource: map[ResourceType]genListOptionFunc{
			{APIVersion: "v1", Kind: "Pod"}: rs2PodLabelListOption,
		},
	}
	globalRule[GroupResourceType{Group: "apps", Kind: "StatefulSet"}] = ChildrenResourcesRule{
		CareResource: map[ResourceType]genListOptionFunc{
			{APIVersion: "v1", Kind: "Pod"}: statefulSet2PodListOption,
		},
	}
	globalRule[GroupResourceType{Group: "helm.toolkit.fluxcd.io", Kind: "HelmRelease"}] = ChildrenResourcesRule{
		CareResource: map[ResourceType]genListOptionFunc{
			{APIVersion: "apps/v1", Kind: "Deployment"}:           nil,
			{APIVersion: "apps/v1", Kind: "StatefulSet"}:          nil,
			{APIVersion: "v1", Kind: "ConfigMap"}:                 nil,
			{APIVersion: "v1", Kind: "Secret"}:                    nil,
			{APIVersion: "v1", Kind: "Service"}:                   nil,
			{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"}: nil,
		},
		DefaultGenListOptionFunc: helmRelease2AnyListOption,
	}
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

// ChildrenResourcesRule define the relationShip between parentObject and children resource
type ChildrenResourcesRule struct {
	// every subResourceType can have a specified genListOptionFunc.
	CareResource map[ResourceType]genListOptionFunc
	// if specified genListOptionFunc is nil will use use default genListOptionFunc to generate listOption.
	DefaultGenListOptionFunc genListOptionFunc
}

type genListOptionFunc func(unstructured.Unstructured) (client.ListOptions, error)

var deploy2RsLabelListOption = func(obj unstructured.Unstructured) (client.ListOptions, error) {
	deploy := appsv1.Deployment{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &deploy)
	if err != nil {
		return client.ListOptions{}, err
	}
	deploySelector, err := v1.LabelSelectorAsSelector(deploy.Spec.Selector)
	if err != nil {
		return client.ListOptions{}, err
	}
	return client.ListOptions{Namespace: deploy.Namespace, LabelSelector: deploySelector}, nil
}

var rs2PodLabelListOption = func(obj unstructured.Unstructured) (client.ListOptions, error) {
	rs := appsv1.ReplicaSet{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &rs)
	if err != nil {
		return client.ListOptions{}, err
	}
	rsSelector, err := v1.LabelSelectorAsSelector(rs.Spec.Selector)
	if err != nil {
		return client.ListOptions{}, err
	}
	return client.ListOptions{Namespace: rs.Namespace, LabelSelector: rsSelector}, nil
}

var statefulSet2PodListOption = func(obj unstructured.Unstructured) (client.ListOptions, error) {
	sts := appsv1.StatefulSet{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &sts)
	if err != nil {
		return client.ListOptions{}, err
	}
	stsSelector, err := v1.LabelSelectorAsSelector(sts.Spec.Selector)
	if err != nil {
		return client.ListOptions{}, err
	}
	return client.ListOptions{Namespace: sts.Namespace, LabelSelector: stsSelector}, nil
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

func checkResourceStatus(obj unstructured.Unstructured) (*types.HealthStatus, error) {
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
	}
	if checkFunc != nil {
		return checkFunc(obj)
	}
	return &types.HealthStatus{Status: types.HealthStatusHealthy}, nil
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
	parentObject unstructured.Unstructured, specifiedFunc genListOptionFunc, defaultFunc genListOptionFunc) ([]unstructured.Unstructured, error) {

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
	return itemList.Items, nil
}

func iteratorChildResources(ctx context.Context, cluster string, k8sClient client.Client, parentResource types.ResourceTreeNode, depth int) ([]types.ResourceTreeNode, error) {
	if depth > maxDepth {
		log.Logger.Warnf("listing application resource tree has reached the max-depth %d parentObject is %v", depth, parentResource)
		return nil, nil
	}
	parentObject, err := fetchObjectWithResourceTreeNode(ctx, cluster, k8sClient, parentResource)
	if err != nil {
		return nil, err
	}
	group := parentObject.GetObjectKind().GroupVersionKind().Group
	kind := parentObject.GetObjectKind().GroupVersionKind().Kind

	if rules, ok := globalRule[GroupResourceType{Group: group, Kind: kind}]; ok {
		var resList []types.ResourceTreeNode
		for resource, specifiedFunc := range rules.CareResource {
			clusterCTX := multicluster.ContextWithClusterName(ctx, cluster)
			items, err := listItemByRule(clusterCTX, k8sClient, resource, *parentObject, specifiedFunc, rules.DefaultGenListOptionFunc)
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				rtn := types.ResourceTreeNode{
					APIVersion: item.GetAPIVersion(),
					Kind:       item.GroupVersionKind().Kind,
					Namespace:  item.GetNamespace(),
					Name:       item.GetName(),
					UID:        item.GetUID(),
					Cluster:    cluster,
				}
				if _, ok := globalRule[GroupResourceType{Group: item.GetObjectKind().GroupVersionKind().Group, Kind: item.GetObjectKind().GroupVersionKind().Kind}]; ok {
					childrenRes, err := iteratorChildResources(ctx, cluster, k8sClient, rtn, depth+1)
					if err != nil {
						return nil, err
					}
					rtn.LeafNodes = childrenRes
				}
				healthStatus, err := checkResourceStatus(item)
				if err != nil {
					return nil, err
				}
				rtn.HealthStatus = *healthStatus
				resList = append(resList, rtn)
			}
		}
		return resList, nil
	}
	return nil, nil
}
