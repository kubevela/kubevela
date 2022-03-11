/*
 Copyright 2021. The KubeVela Authors.

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

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// CreateAppHandlerDurationHistogram report the create appHandler execution duration.
	CreateAppHandlerDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "create_app_handler_time_seconds",
		Help:        "create appHandler duration distributions, this operate will list ResourceTrackers.",
		Buckets:     histogramBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"controller"})

	// HandleFinalizersDurationHistogram report the handle finalizers execution duration.
	HandleFinalizersDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "handle_finalizers_time_seconds",
		Help:        "handle finalizers duration distributions.",
		Buckets:     histogramBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"controller", "type"})

	// ParseAppFileDurationHistogram report the parse appFile execution duration.
	ParseAppFileDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "parse_appFile_time_seconds",
		Help:        "parse appFile duration distributions.",
		Buckets:     histogramBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"controller"})

	// PrepareCurrentAppRevisionDurationHistogram report the parse current appRevision execution duration.
	PrepareCurrentAppRevisionDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "prepare_current_appRevision_time_seconds",
		Help:        "parse current appRevision duration distributions.",
		Buckets:     histogramBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"controller"})

	// ApplyAppRevisionDurationHistogram report the apply appRevision execution duration.
	ApplyAppRevisionDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "apply_appRevision_time_seconds",
		Help:        "apply appRevision duration distributions.",
		Buckets:     histogramBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"controller"})

	// PrepareWorkflowAndPolicyDurationHistogram report the prepare workflow and policy execution duration.
	PrepareWorkflowAndPolicyDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "prepare_workflow_and_policy_time_seconds",
		Help:        "prepare workflow and policy duration distributions.",
		Buckets:     histogramBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"controller"})

	// GCResourceTrackersDurationHistogram report the gc resourceTrackers execution duration.
	GCResourceTrackersDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "gc_resourceTrackers_time_seconds",
		Help:        "gc resourceTrackers duration distributions.",
		Buckets:     histogramBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"stage"})

	// ClientRequestHistogram report the client request execution duration.
	ClientRequestHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "client_request_time_seconds",
		Help:        "client request duration distributions.",
		Buckets:     histogramBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"verb", "Kind", "apiVersion", "unstructured", "cluster"})

	// ApplicationReconcileTimeHistogram report the reconciling time cost of application controller with state transition recorded
	ApplicationReconcileTimeHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "application_reconcile_time_seconds",
		Help:        "application reconcile duration distributions.",
		Buckets:     histogramBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"begin_phase", "end_phase"})

	// ApplyComponentTimeHistogram report the time cost of applyComponentFunc
	ApplyComponentTimeHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "apply_component_time_seconds",
		Help:        "apply component duration distributions.",
		Buckets:     histogramBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"stage"})
)

var (
	// ListResourceTrackerCounter report the list resource tracker number.
	ListResourceTrackerCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "list_resourcetracker_num",
		Help: "list resourceTrackers times.",
	}, []string{"controller"})
)

var (
	// ResourceTrackerNumberGauge report the number of resourceTracker
	ResourceTrackerNumberGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "resourcetracker_number",
		Help: "resourceTracker number.",
	}, []string{"controller"})

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

	// ClusterPodUsageGauge report the number of PodUsage in cluster
	ClusterPodUsageGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cluster_pod_usage",
		Help:        "cluster pod usage number.",
		ConstLabels: prometheus.Labels{},
	}, []string{"cluster"})
)
