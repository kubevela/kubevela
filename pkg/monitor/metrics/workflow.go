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

var (
	// StepDurationSummary report the step execution duration summary.
	StepDurationSummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name:        "step_duration_ms",
		Help:        "step latency distributions.",
		Objectives:  map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		ConstLabels: prometheus.Labels{},
	}, []string{"application", "workflow_revision", "step_name", "step_type"})
)

func init() {
	if err := metrics.Registry.Register(StepDurationSummary); err != nil {
		klog.Error(err)
	}
}
