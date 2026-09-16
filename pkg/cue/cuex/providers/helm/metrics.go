/*
Copyright 2026 The KubeVela Authors.

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

package helm

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// HelmChartCacheHitsTotal counts the number of cache hits when fetching helm charts.
var HelmChartCacheHitsTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "kubevela_helm_chart_cache_hits_total",
	Help: "Total number of cache hits when fetching helm charts.",
})

// HelmChartCacheMissesTotal counts the number of cache misses when fetching helm charts, by reason.
// Labels:
//   - reason: "absent" (key was never cached), "expired" (TTL expired, removed by TTL sweeper), "evicted"
//     (dropped to stay inside the byte budget), or "corrupt" (entry was discarded
//     because the archive failed to load)
var HelmChartCacheMissesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "kubevela_helm_chart_cache_misses_total",
	Help: "Total number of cache misses when fetching helm charts, by reason (absent, expired, evicted or corrupt).",
}, []string{"reason"})

// HelmChartCacheEvictionsTotal counts cache entries evicted, by reason.
// Labels:
//   - reason: "capacity" (LRU eviction when the byte budget is full) or "ttl" (expired entry removed)
var HelmChartCacheEvictionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "kubevela_helm_chart_cache_evictions_total",
	Help: "Total number of helm chart cache evictions, by reason (capacity, ttl, delete, replace, purge or unknown).",
}, []string{"reason"})

// HelmChartCacheBytes is the current number of bytes held by the helm chart cache.
// A sustained value near the MaxBytes budget combined with capacity evictions means
// the working set exceeds the cache size.
var HelmChartCacheBytes = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "kubevela_helm_chart_cache_bytes",
	Help: "Current number of bytes stored in the helm chart cache.",
})

func init() {
	metrics.Registry.MustRegister(
		HelmChartCacheHitsTotal,
		HelmChartCacheMissesTotal,
		HelmChartCacheEvictionsTotal,
		HelmChartCacheBytes,
	)
}
