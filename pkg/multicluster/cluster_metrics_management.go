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
	"time"

	"github.com/oam-dev/kubevela/pkg/monitor/metrics"

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
func NewClusterMetricsMgr(ctx context.Context, kubeClient client.Client, refreshPeriod time.Duration) (*ClusterMetricsMgr, error) {
	mgr := &ClusterMetricsMgr{
		kubeClient: kubeClient,
	}
	go mgr.start(ctx, refreshPeriod)
	return mgr, nil
}

// Refresh will re-collect cluster metrics and refresh cache
func (cmm *ClusterMetricsMgr) Refresh() ([]VirtualCluster, error) {
	clusters, _ := ListVirtualClusters(context.Background(), cmm.kubeClient)
	m := make(map[string]*ClusterMetrics)

	// retrieves metrics by cluster-gateway
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
		cm := &ClusterMetrics{
			IsConnected:         isConnected,
			ClusterInfo:         clusterInfo,
			ClusterUsageMetrics: clusterUsageMetrics,
		}
		m[cluster.Name] = cm
		cluster.Metrics = cm
	}
	metricsMap = m
	return clusters, nil
}

// start will start polling cluster api to collect metrics
func (cmm *ClusterMetricsMgr) start(ctx context.Context, refreshPeriod time.Duration) {
	for {
		select {
		case <-ctx.Done():
			klog.Warning("Stop cluster metrics polling loop.")
			return
		default:
			clusters, _ := cmm.Refresh()
			for _, cluster := range clusters {
				exportMetrics(cluster.Metrics, cluster.Name)
			}
			time.Sleep(refreshPeriod)
		}
	}
}

// exportMetrics will report ClusterMetrics with a clusterName label
func exportMetrics(m *ClusterMetrics, clusterName string) {
	if m == nil {
		return
	}
	metrics.ClusterIsConnectedGauge.WithLabelValues(clusterName).Set(func() float64 {
		if m.IsConnected {
			return 1
		}
		return 0
	}())
	if m.ClusterInfo != nil {
		metrics.ClusterWorkerNumberGauge.WithLabelValues(clusterName).Set(float64(m.ClusterInfo.WorkerNumber))
		metrics.ClusterMasterNumberGauge.WithLabelValues(clusterName).Set(float64(m.ClusterInfo.MasterNumber))
		metrics.ClusterMemoryCapacityGauge.WithLabelValues(clusterName).Set(m.ClusterInfo.MemoryCapacity.AsApproximateFloat64())
		metrics.ClusterCPUCapacityGauge.WithLabelValues(clusterName).Set(float64(m.ClusterInfo.CPUCapacity.MilliValue()))
		metrics.ClusterPodCapacityGauge.WithLabelValues(clusterName).Set(m.ClusterInfo.PodCapacity.AsApproximateFloat64())
		metrics.ClusterMemoryAllocatableGauge.WithLabelValues(clusterName).Set(m.ClusterInfo.MemoryAllocatable.AsApproximateFloat64())
		metrics.ClusterCPUAllocatableGauge.WithLabelValues(clusterName).Set(float64(m.ClusterInfo.CPUAllocatable.MilliValue()))
		metrics.ClusterPodAllocatableGauge.WithLabelValues(clusterName).Set(m.ClusterInfo.PodAllocatable.AsApproximateFloat64())
	}
	if m.ClusterUsageMetrics != nil {
		metrics.ClusterMemoryUsageGauge.WithLabelValues(clusterName).Set(m.ClusterUsageMetrics.MemoryUsage.AsApproximateFloat64())
		metrics.ClusterCPUUsageGauge.WithLabelValues(clusterName).Set(float64(m.ClusterUsageMetrics.CPUUsage.MilliValue()))
	}
}
