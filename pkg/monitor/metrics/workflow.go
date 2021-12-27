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
)

var histogramBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
	1.25, 1.5, 1.75, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5, 6, 7, 8, 9, 10, 15, 20, 25, 30, 40, 50, 60}

var (
	// StepDurationHistogram report the step execution duration.
	StepDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "step_duration_ms",
		Help:        "step latency distributions.",
		Buckets:     histogramBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"controller", "step_type"})
)

var collectorGroup = []prometheus.Collector{
	CreateAppHandlerDurationHistogram,
	HandleFinalizersDurationHistogram,
	ParseAppFileDurationHistogram,
	PrepareCurrentAppRevisionDurationHistogram,
	ApplyAppRevisionDurationHistogram,
	PrepareWorkflowAndPolicyDurationHistogram,
	StepDurationHistogram,
	GCResourceTrackersDurationHistogram,
	ListResourceTrackerCounter,
}

func init() {
	for _, collector := range collectorGroup {
		if err := metrics.Registry.Register(collector); err != nil {
			klog.Error(err)
		}
	}
	for _, collector := range GCResourceTrackersDurationDetailHistograms {
		if err := metrics.Registry.Register(collector); err != nil {
			klog.Error(err)
		}
	}
}
