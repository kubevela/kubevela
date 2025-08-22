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

	velametrics "github.com/kubevela/pkg/monitor/metrics"
)

var (
	// AppReconcileStageDurationHistogram report staged reconcile time for application
	AppReconcileStageDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "kubevela_app_reconcile_time_seconds",
		Help:        "application reconcile time costs.",
		Buckets:     velametrics.FineGrainedBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"stage"})

	// ApplicationReconcileTimeHistogram report the reconciling time cost of application controller with state transition recorded
	ApplicationReconcileTimeHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "application_reconcile_time_seconds",
		Help:        "application reconcile duration distributions.",
		Buckets:     velametrics.FineGrainedBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"begin_phase", "end_phase"})

	// ApplyComponentTimeHistogram report the time cost of applyComponentFunc
	ApplyComponentTimeHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "apply_component_time_seconds",
		Help:        "apply component duration distributions.",
		Buckets:     velametrics.FineGrainedBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"stage"})

	// WorkflowFinishedTimeHistogram report the time for finished workflow
	WorkflowFinishedTimeHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "workflow_finished_time_seconds",
		Help:        "workflow finished time distributions.",
		Buckets:     velametrics.FineGrainedBuckets,
		ConstLabels: prometheus.Labels{},
	}, []string{"phase"})

	// ApplicationPhaseCounter report the number of application phase
	ApplicationPhaseCounter = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "application_phase_number",
		Help: "application phase number",
	}, []string{"phase"})

	// WorkflowStepPhaseGauge report the number of workflow step state
	WorkflowStepPhaseGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "workflow_step_phase_number",
		Help: "workflow step phase number",
	}, []string{"step_type", "phase"})

	// ApplicationHealthStatus reports the overall health status of each application
	ApplicationHealthStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_application_health_status",
		Help: "Application health status (1 = healthy, 0 = unhealthy)",
	}, []string{"app_name", "namespace"})

	// ApplicationPhase reports the numeric phase of each application
	ApplicationPhase = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_application_phase",
		Help: "Application phase as numeric value (0=starting, 1=running, 2=rendering, 3=policy_generating, 4=running_workflow, " +
			"5=workflow_suspending, 6=workflow_terminated, 7=workflow_failed, 8=unhealthy, 9=deleting, " +
			"-1=unknown)",
	}, []string{"app_name", "namespace"})

	// WorkflowPhase reports the numeric phase of each workflow
	WorkflowPhase = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_application_workflow_phase",
		Help: "Workflow phase as numeric value (0=initializing, 1=succeeded, 2=executing, 3=suspending, 4=terminated, " +
			"5=failed, 6=skipped, -1=unknown)",
	}, []string{"app_name", "namespace"})
)

var (
	// ListResourceTrackerCounter report the list resource tracker number.
	ListResourceTrackerCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "list_resourcetracker_num",
		Help: "list resourceTrackers times.",
	}, []string{"controller"})
)
