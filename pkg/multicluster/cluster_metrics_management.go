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

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// metricsMap records the metrics of clusters
var metricsMap map[string]*ClusterMetrics

// ClusterMetricsMgr manage metrics of clusters
type ClusterMetricsMgr struct {
	kubeClient client.Client
}

// ClusterMetricsHelper is the interface that provides operations for cluster metrics
type ClusterMetricsHelper interface {
	Refresh() error
}

// NewClusterMetricsMgr will create a cluster metrics manager
func NewClusterMetricsMgr(kubeClient client.Client) (*ClusterMetricsMgr, error) {
	mgr := &ClusterMetricsMgr{
		kubeClient: kubeClient,
	}
	err := mgr.Refresh()
	return mgr, err
}

// Refresh will re-collect cluster metrics and refresh cache
func (cmm *ClusterMetricsMgr) Refresh() error {
	clusters, _ := ListVirtualClusters(context.Background(), cmm.kubeClient)
	m := make(map[string]*ClusterMetrics)

	// request metrics api by cluster-gateway
	for _, cluster := range clusters {
		isConnected := true
		clusterInfo, err := GetClusterInfo(context.Background(), cmm.kubeClient, cluster.Name)
		if err != nil {
			klog.Warningf("failed to get cluster info of cluster-(%s)", cluster.Name)
			isConnected = false
		}
		clusterUsageMetrics, err := GetClusterMetricsFromMetricsAPI(context.Background(), cmm.kubeClient, cluster.Name)
		if err != nil {
			klog.Warningf("failed to request metrics api of cluster-(%s)", cluster.Name)
		}
		metrics := &ClusterMetrics{
			IsConnected:         isConnected,
			ClusterInfo:         clusterInfo,
			ClusterUsageMetrics: clusterUsageMetrics,
		}
		m[cluster.Name] = metrics
	}
	metricsMap = m
	return nil
}
