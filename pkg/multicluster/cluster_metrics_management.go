/*
Copyright 2022 The KubeVela Authors.

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
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// CPUResourceName is a key in metricsMap to represent cpu resources
	CPUResourceName = "cpu"
	// MemoryResourceName is a key in metricsMap to represent memory resources
	MemoryResourceName = "memory"
	// IsConnectedName is a key in metricsMap to represent connectivity
	IsConnectedName = "is_connected"

	// CPUResourceDescription is the description of the CPU metrics
	CPUResourceDescription = "CPU resources available in the cluster. (unit is m)"
	// MemoryResourceDescription is the description of the Memory metrics
	MemoryResourceDescription = "Memory resources available in the cluster. (unit is Mi)"
	// IsConnectedDescription is the description of the IsConnected metrics
	IsConnectedDescription = "Whether the managed cluster is connected to the hub cluster."
)

// ClusterMetricsMgr manage metrics of cluster
type ClusterMetricsMgr struct {
	kubeClient   client.Client
	clusterNum   int
	clusterNames []string

	// metricsMap records the metrics of clusters in clusterNames
	metricsMap map[string]*MetricsDetail
}

// MetricsDetail info the detail of metrics
type MetricsDetail struct {
	value       []string
	description string
}

// ClusterMetricsHelper is the interface that provides operations for cluster metrics
type ClusterMetricsHelper interface {
	Refresh() error
	CPUResources() ([]string, MetricsDetail, error)
	MemoryResources() ([]string, MetricsDetail, error)
	IsConnected() ([]string, MetricsDetail, error)
}

// NewClusterMetricsMgr will create a cluster metrics manager
func NewClusterMetricsMgr(kubeClient client.Client) *ClusterMetricsMgr {
	metricsMap := make(map[string]*MetricsDetail)
	metricsMap[CPUResourceName] = &MetricsDetail{description: CPUResourceDescription}
	metricsMap[MemoryResourceName] = &MetricsDetail{description: MemoryResourceDescription}
	metricsMap[IsConnectedName] = &MetricsDetail{description: IsConnectedDescription}
	mgr := &ClusterMetricsMgr{
		kubeClient: kubeClient,
		metricsMap: metricsMap,
	}
	return mgr
}

// Refresh will re-collect cluster metrics and refresh cache
// TODO obtain the occupied resource metrics by querying the metrics-server of the cluster.
func (cmm *ClusterMetricsMgr) Refresh() error {
	// refresh clusterNames firstly
	err := cmm.RefreshClusterName()
	if err != nil {
		return err
	}

	clusterNum := cmm.clusterNum
	cpuResources := make([]string, clusterNum)
	memoryResources := make([]string, clusterNum)
	connectivity := make([]string, clusterNum)

	// request metrics by cluster-gateway
	for i, clusterName := range cmm.clusterNames {
		ctx := ContextWithClusterName(context.Background(), clusterName)
		var nodes corev1.NodeList
		var isConnected = true
		if err = cmm.kubeClient.List(ctx, &nodes, &client.ListOptions{}); err != nil {
			// If the cluster is disconnected, just set the resource to 0
			klog.Errorf("Failed to get nodes information from cluster-(%s).", clusterName)
			isConnected = false
		}

		var cpuResource int64
		var memoryResource int64

		for _, item := range nodes.Items {
			cpuResource += item.Status.Allocatable.Cpu().MilliValue()
			memoryResource += item.Status.Allocatable.Memory().Value()
		}

		cpuResources[i] = strconv.FormatInt(cpuResource, 10)
		memoryResources[i] = strconv.FormatInt(memoryResource/(1024*1024), 10)
		connectivity[i] = strconv.FormatBool(isConnected)
	}

	// transform to ClusterMetricsMgr
	cmm.metricsMap[CPUResourceName].value = cpuResources
	cmm.metricsMap[MemoryResourceName].value = memoryResources
	cmm.metricsMap[IsConnectedName].value = connectivity

	return nil
}

// CPUResources will return the current list of cluster name and cpu resource detail asynchronously
func (cmm *ClusterMetricsMgr) CPUResources() ([]string, MetricsDetail, error) {
	return cmm.clusterNames, *cmm.metricsMap[CPUResourceName], nil
}

// MemoryResources will return the current list of cluster name and memory resource detail asynchronously
func (cmm *ClusterMetricsMgr) MemoryResources() ([]string, MetricsDetail, error) {
	return cmm.clusterNames, *cmm.metricsMap[MemoryResourceName], nil
}

// IsConnected will return the current list of cluster name and connectivity detail asynchronously
func (cmm *ClusterMetricsMgr) IsConnected() ([]string, MetricsDetail, error) {
	return cmm.clusterNames, *cmm.metricsMap[IsConnectedName], nil
}

// RefreshClusterName will refresh the clusterNames of ClusterMetricsMgr
func (cmm *ClusterMetricsMgr) RefreshClusterName() error {
	clusters, err := ListVirtualClusters(context.Background(), cmm.kubeClient)
	if err != nil {
		klog.Errorf("Refresh cluster name failed.")
		return err
	}
	cmm.clusterNum = len(clusters)
	cmm.clusterNames = make([]string, cmm.clusterNum)
	for i, cluster := range clusters {
		cmm.clusterNames[i] = cluster.Name
	}
	return nil
}
