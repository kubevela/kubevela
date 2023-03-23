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
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	velametrics "github.com/kubevela/pkg/monitor/metrics"
)

var (
	// StepDurationHistogram report the step execution duration.
	StepDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "step_duration_ms",
		Help:        "step latency distributions.",
		Buckets:     velametrics.FineGrainedBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"controller", "step_type"})
)

var collectorGroup = []prometheus.Collector{
	AppReconcileStageDurationHistogram,
	StepDurationHistogram,
	ListResourceTrackerCounter,
	ApplicationReconcileTimeHistogram,
	ApplyComponentTimeHistogram,
	WorkflowFinishedTimeHistogram,
	ApplicationPhaseCounter,
	WorkflowStepPhaseGauge,
	ClusterIsConnectedGauge,
	ClusterWorkerNumberGauge,
	ClusterMasterNumberGauge,
	ClusterMemoryCapacityGauge,
	ClusterCPUCapacityGauge,
	ClusterPodCapacityGauge,
	ClusterMemoryAllocatableGauge,
	ClusterCPUAllocatableGauge,
	ClusterPodAllocatableGauge,
	ClusterMemoryUsageGauge,
	ClusterCPUUsageGauge,
}

func init() {
	for _, collector := range collectorGroup {
		if err := metrics.Registry.Register(collector); err != nil {
			klog.Error(err)
		}
	}
}
