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

package upgrade

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// CUECompatRewriteTotal counts the number of CUE compatibility rewrites applied at render time.
// Labels:
//   - upgrade_id: stable fix identifier, e.g. "list-arithmetic"
//   - upgrade_version: KubeVela version that introduced the incompatibility, e.g. "1.11"
//   - definition_kind: definition type, e.g. "Component"
//   - template_area: which part of the definition was rewritten, e.g. "template", "health"
//
// Use this metric to track which legacy syntax fixes are still active cluster-wide.
// The metric is the smoke detector; run `vela def compat definitions` to identify which
// specific definitions need remediation.
var CUECompatRewriteTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "kubevela_cue_compat_rewrite_total",
	Help: "Total number of CUE compatibility rewrites applied at render time, by upgrade ID, version introduced, definition kind, and template area.",
}, []string{"upgrade_id", "upgrade_version", "definition_kind", "template_area"})

// CUECompatUpgradeDuration measures how long EnsureCueVersionCompatibility takes per call.
// Only recorded on cache misses (i.e. when the actual upgrade logic runs).
// Labels:
//   - definition_kind: definition type, e.g. "Component"
//
// Use histogram_quantile() in PromQL to compute aggregated latency percentiles across replicas.
// Cache hits are not recorded — they are too cheap to be meaningful.
var CUECompatUpgradeDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "kubevela_cue_compat_upgrade_duration_seconds",
	Help:    "Duration of CUE version compatibility checks at render time (cache misses only), by definition kind.",
	Buckets: []float64{0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
}, []string{"definition_kind"})

// CUECompatCacheEvictionsTotal counts cache entries evicted, by reason.
// Labels:
//   - reason: "capacity" (LRU eviction when cache is full) or "ttl" (idle entry swept by background loop)
//
// A sustained capacity eviction rate means the working set exceeds cache size;
// increase --cue-compatibility-cache-size or migrate definitions to compliant CUE
// syntax (run `vela def compat` to identify which definitions need remediation).
var CUECompatCacheEvictionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "kubevela_cue_compat_cache_evictions_total",
	Help: "Total number of CUE compatibility cache evictions, by reason (capacity or ttl).",
}, []string{"reason"})

func init() {
	metrics.Registry.MustRegister(
		CUECompatRewriteTotal,
		CUECompatUpgradeDuration,
		CUECompatCacheEvictionsTotal,
	)
}
