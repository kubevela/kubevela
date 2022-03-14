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

package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// ClusterIsConnectedGauge report if the cluster is connected
	ClusterIsConnectedGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cluster_isconnected",
		Help: "if cluster is connected.",
	}, []string{"cluster"})

	// ClusterWorkerNumberGauge report the number of WorkerNumber in cluster
	ClusterWorkerNumberGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cluster_worker_node_number",
		Help:        "cluster worker node number.",
		ConstLabels: prometheus.Labels{},
	}, []string{"cluster"})

	// ClusterMasterNumberGauge report the number of MasterNumber in cluster
	ClusterMasterNumberGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cluster_master_node_number",
		Help:        "cluster master node number.",
		ConstLabels: prometheus.Labels{},
	}, []string{"cluster"})

	// ClusterMemoryCapacityGauge report the number of MemoryCapacity in cluster
	ClusterMemoryCapacityGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cluster_memory_capacity",
		Help:        "cluster memory capacity number.",
		ConstLabels: prometheus.Labels{},
	}, []string{"cluster"})

	// ClusterCPUCapacityGauge report the number of CPUCapacity in cluster
	ClusterCPUCapacityGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cluster_cpu_capacity",
		Help:        "cluster cpu capacity number.",
		ConstLabels: prometheus.Labels{},
	}, []string{"cluster"})

	// ClusterPodCapacityGauge report the number of PodCapacity in cluster
	ClusterPodCapacityGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cluster_pod_capacity",
		Help:        "cluster pod capacity number.",
		ConstLabels: prometheus.Labels{},
	}, []string{"cluster"})

	// ClusterMemoryAllocatableGauge report the number of MemoryAllocatable in cluster
	ClusterMemoryAllocatableGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cluster_memory_allocatable",
		Help:        "cluster memory allocatable number.",
		ConstLabels: prometheus.Labels{},
	}, []string{"cluster"})

	// ClusterCPUAllocatableGauge report the number of CPUAllocatable in cluster
	ClusterCPUAllocatableGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cluster_cpu_allocatable",
		Help:        "cluster cpu allocatable number.",
		ConstLabels: prometheus.Labels{},
	}, []string{"cluster"})

	// ClusterPodAllocatableGauge report the number of PodAllocatable in cluster
	ClusterPodAllocatableGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cluster_pod_allocatable",
		Help:        "cluster pod allocatable number.",
		ConstLabels: prometheus.Labels{},
	}, []string{"cluster"})

	// ClusterMemoryUsageGauge report the number of MemoryUsage in cluster
	ClusterMemoryUsageGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cluster_memory_usage",
		Help:        "cluster memory usage number.",
		ConstLabels: prometheus.Labels{},
	}, []string{"cluster"})

	// ClusterCPUUsageGauge report the number of CPUUsage in cluster
	ClusterCPUUsageGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cluster_cpu_usage",
		Help:        "cluster cpu usage number.",
		ConstLabels: prometheus.Labels{},
	}, []string{"cluster"})
)
