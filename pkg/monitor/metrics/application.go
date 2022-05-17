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

	// ApplyPoliciesDurationHistogram report execution duration for applying policies
	ApplyPoliciesDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "apply_policies",
		Help:        "render and dispatch policy duration distributions.",
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

	// HandleHooksDurationHistogram report the handle hooks execution duration.
	HandleHooksDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "handle_hooks_time_seconds",
		Help:        "handle hooks duration distributions.",
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
)
