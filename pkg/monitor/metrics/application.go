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

import "github.com/prometheus/client_golang/prometheus"

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

	//UpdateAppLatestRevisionDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	//	Name:        "update_app_latest_revision_time_seconds",
	//	Help:        "update app latest_revision duration distributions.",
	//	Buckets:     histogramBuckets,
	//	ConstLabels: prometheus.Labels{},
	//}, []string{"application"})

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
	}, []string{"controller"})
)

var (
	// ListResourceTrackerCounter report the list resource tracker number.
	ListResourceTrackerCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "list_resourcetracker_num",
		Help: "list resourceTrackers times.",
	}, []string{"controller"})
)
