/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package definition

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// CUERenderDuration measures the end-to-end duration of a definition Complete call,
// covering CUE version compatibility rewriting, compilation, and validation.
// Labels:
//   - definition_kind: definition type, e.g. "Component", "Trait"
//   - status: "ok" on success, "error" on failure
//
// Buckets are tuned for the expected millisecond range of CUE compilation and validation.
// Use histogram_quantile() in PromQL to compute aggregated latency percentiles across replicas.
var CUERenderDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "kubevela_cue_render_duration_seconds",
	Help:    "End-to-end duration of CUE definition rendering (compat rewrite + compile + validate), by definition kind and status.",
	Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
}, []string{"definition_kind", "status"})

func init() {
	metrics.Registry.MustRegister(CUERenderDuration)
}
