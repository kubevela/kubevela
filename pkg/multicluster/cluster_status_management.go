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
	"encoding/json"
	"time"

	"github.com/kubevela/pkg/multicluster"
	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	metricsV1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
)

const labelKeyMasterNode = "node-role.kubernetes.io/master"

func loadClusterStatus(ctx context.Context, cli client.Client, staticClient kubernetes.Interface, clusterName string) (*v1alpha1.VirtualClusterStatus, error) {
	ctx = multicluster.WithCluster(ctx, clusterName)
	// update healthy
	status := &v1alpha1.VirtualClusterStatus{}
	status.LastProbeTime = metav1.NewTime(time.Now())
	if content, err := staticClient.Discovery().RESTClient().Get().AbsPath("healthz").DoRaw(ctx); err != nil {
		status.Healthy = false
		status.Reason = err.Error()
	} else {
		status.Healthy = true
		status.Reason = string(content)
	}
	// update version
	bs, err := staticClient.Discovery().RESTClient().Get().AbsPath("version").DoRaw(ctx)
	if err != nil {
		return status, err
	}
	if err = json.Unmarshal(bs, &status.Version); err != nil {
		return status, err
	}
	// update resources
	nodes, err := staticClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return status, err
	}
	for _, node := range nodes.Items {
		if _, ok := node.Labels[labelKeyMasterNode]; ok {
			status.Resources.MasterNodeCount++
			continue
		}
		status.Resources.WorkerNodeCount++
		addCapacity(&status.Resources.Capacity, &node.Status.Capacity)
		addCapacity(&status.Resources.Allocatable, &node.Status.Allocatable)
	}
	nodeMetricsList := metricsV1beta1api.NodeMetricsList{}
	if err = cli.List(ctx, &nodeMetricsList); err != nil {
		return status, err
	}
	for _, nodeMetrics := range nodeMetricsList.Items {
		addCapacity(&status.Resources.Capacity, &nodeMetrics.Usage)
	}
	return status, nil
}

func addCapacity(base *corev1.ResourceList, delta *corev1.ResourceList) {
	base.Cpu().Add(*delta.Cpu())
	base.Memory().Add(*delta.Memory())
	base.Pods().Add(*delta.Pods())
	base.Storage().Add(*delta.Storage())
	base.StorageEphemeral().Add(*delta.StorageEphemeral())
}

// ClusterStatusMgr manage metrics of clusters
type ClusterStatusMgr struct {
	ctx           context.Context
	cli           client.Client
	k             kubernetes.Interface
	refreshPeriod time.Duration
}

// NewClusterStatusMgr will create a cluster metrics manager
func NewClusterStatusMgr(ctx context.Context, cli client.Client, k kubernetes.Interface, refreshPeriod time.Duration) (*ClusterStatusMgr, error) {
	mgr := &ClusterStatusMgr{ctx: ctx, cli: cli, k: k, refreshPeriod: refreshPeriod}
	go mgr.Start(ctx)
	return mgr, nil
}

// Refresh will re-collect cluster metrics and refresh cache
func (cmm *ClusterStatusMgr) Refresh() error {
	ctx := context.Background()
	clusterClient := NewClusterClient(cmm.cli)
	clusters, err := clusterClient.List(ctx, client.MatchingLabels{v1alpha1.LabelClusterControlPlane: "false"})
	if err != nil {
		return err
	}

	var errs []error
	// retrieves metrics by cluster-gateway
	for _, cluster := range clusters.Items {
		status, err := loadClusterStatus(ctx, cmm.cli, cmm.k, cluster.Name)
		if err != nil {
			errs = append(errs, err)
		}
		if status != nil {
			if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				vc, err := clusterClient.UpdateStatus(ctx, cluster.Name, *status)
				if err != nil {
					return err
				}
				exportMetrics(vc)
				return nil
			}); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.NewAggregate(errs)
}

// Start will start polling cluster api to collect metrics
func (cmm *ClusterStatusMgr) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			klog.Warning("Stop cluster metrics polling loop.")
			return
		default:
			err := cmm.Refresh()
			if err != nil {
				klog.ErrorS(err, "error encountered during updating cluster status")
			}
			time.Sleep(cmm.refreshPeriod)
		}
	}
}

// exportMetrics will report ClusterMetrics with a clusterName label
func exportMetrics(vc *v1alpha1.VirtualCluster) {
	clusterName, status := vc.Name, vc.Status

	metrics.ClusterIsConnectedGauge.WithLabelValues(clusterName).Set(func() float64 {
		if status.Healthy {
			return 1
		}
		return 0
	}())

	metrics.ClusterWorkerNumberGauge.WithLabelValues(clusterName).Set(float64(status.Resources.WorkerNodeCount))
	metrics.ClusterMasterNumberGauge.WithLabelValues(clusterName).Set(float64(status.Resources.MasterNodeCount))

	// capacity
	memory, cpu, pod := status.Resources.Capacity[corev1.ResourceMemory], status.Resources.Capacity[corev1.ResourceCPU], status.Resources.Capacity[corev1.ResourcePods]
	metrics.ClusterMemoryCapacityGauge.WithLabelValues(clusterName).Set(memory.AsApproximateFloat64())
	metrics.ClusterCPUCapacityGauge.WithLabelValues(clusterName).Set(float64(cpu.MilliValue()))
	metrics.ClusterPodCapacityGauge.WithLabelValues(clusterName).Set(pod.AsApproximateFloat64())

	// allocatable
	memory, cpu, pod = status.Resources.Allocatable[corev1.ResourceMemory], status.Resources.Allocatable[corev1.ResourceCPU], status.Resources.Allocatable[corev1.ResourcePods]
	metrics.ClusterMemoryAllocatableGauge.WithLabelValues(clusterName).Set(memory.AsApproximateFloat64())
	metrics.ClusterCPUAllocatableGauge.WithLabelValues(clusterName).Set(float64(cpu.MilliValue()))
	metrics.ClusterPodAllocatableGauge.WithLabelValues(clusterName).Set(pod.AsApproximateFloat64())

	// usage
	memory, cpu, pod = status.Resources.Usage[corev1.ResourceMemory], status.Resources.Usage[corev1.ResourceCPU], status.Resources.Usage[corev1.ResourcePods]
	metrics.ClusterMemoryUsageGauge.WithLabelValues(clusterName).Set(memory.AsApproximateFloat64())
	metrics.ClusterCPUUsageGauge.WithLabelValues(clusterName).Set(float64(cpu.MilliValue()))
	metrics.ClusterPodUsageGauge.WithLabelValues(clusterName).Set(pod.AsApproximateFloat64())
}
