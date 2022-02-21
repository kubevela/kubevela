/*
Copyright 2020-2022 The KubeVela Authors.

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

package multicluster

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterInfo describes the basic information of a cluster
type ClusterInfo struct {
	Nodes             *corev1.NodeList
	WorkerNumber      int
	MasterNumber      int
	MemoryCapacity    resource.Quantity
	CPUCapacity       resource.Quantity
	PodCapacity       resource.Quantity
	MemoryAllocatable resource.Quantity
	CPUAllocatable    resource.Quantity
	PodAllocatable    resource.Quantity
	StorageClasses    *storagev1.StorageClassList
}

// GetClusterInfo retrieves current cluster info from cluster
func GetClusterInfo(_ctx context.Context, k8sClient client.Client, clusterName string) (*ClusterInfo, error) {
	ctx := ContextWithClusterName(_ctx, clusterName)
	nodes := &corev1.NodeList{}
	if err := k8sClient.List(ctx, nodes); err != nil {
		return nil, errors.Wrapf(err, "failed to list cluster nodes")
	}
	var workerNumber, masterNumber int
	var memoryCapacity, cpuCapacity, podCapacity, memoryAllocatable, cpuAllocatable, podAllocatable resource.Quantity
	for _, node := range nodes.Items {
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			masterNumber++
		} else {
			workerNumber++
		}
		capacity := node.Status.Capacity
		memoryCapacity.Add(*capacity.Memory())
		cpuCapacity.Add(*capacity.Cpu())
		podCapacity.Add(*capacity.Pods())
		allocatable := node.Status.Allocatable
		memoryAllocatable.Add(*allocatable.Memory())
		cpuAllocatable.Add(*allocatable.Cpu())
		podAllocatable.Add(*allocatable.Pods())
	}
	storageClasses := &storagev1.StorageClassList{}
	if err := k8sClient.List(ctx, storageClasses); err != nil {
		return nil, errors.Wrapf(err, "failed to list storage classes")
	}
	return &ClusterInfo{
		Nodes:             nodes,
		WorkerNumber:      workerNumber,
		MasterNumber:      masterNumber,
		MemoryCapacity:    memoryCapacity,
		CPUCapacity:       cpuCapacity,
		PodCapacity:       podCapacity,
		MemoryAllocatable: memoryAllocatable,
		CPUAllocatable:    cpuAllocatable,
		PodAllocatable:    podAllocatable,
		StorageClasses:    storageClasses,
	}, nil
}
