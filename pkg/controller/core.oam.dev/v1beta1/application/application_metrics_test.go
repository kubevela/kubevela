/*
Copyright 2021 The KubeVela Authors.

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

package application

import (
	"context"
	"testing"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
)

func TestCalculateHealthStatus(t *testing.T) {
	tests := []struct {
		name     string
		services []common.ApplicationComponentStatus
		expected HealthStatus
	}{
		{
			name: "all services healthy",
			services: []common.ApplicationComponentStatus{
				{Healthy: true},
				{Healthy: true},
				{Healthy: true},
			},
			expected: HealthStatus{
				Healthy:        true,
				HealthyCount:   3,
				UnhealthyCount: 0,
			},
		},
		{
			name: "some unhealthy",
			services: []common.ApplicationComponentStatus{
				{Healthy: true},
				{Healthy: false},
				{Healthy: true},
			},
			expected: HealthStatus{
				Healthy:        false,
				HealthyCount:   2,
				UnhealthyCount: 1,
			},
		},
		{
			name: "all services unhealthy",
			services: []common.ApplicationComponentStatus{
				{Healthy: false},
				{Healthy: false},
			},
			expected: HealthStatus{
				Healthy:        false,
				HealthyCount:   0,
				UnhealthyCount: 2,
			},
		},
		{
			name:     "no services",
			services: []common.ApplicationComponentStatus{},
			expected: HealthStatus{
				Healthy:        true,
				HealthyCount:   0,
				UnhealthyCount: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateHealthStatus(tt.services)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestAppPhaseToNumeric(t *testing.T) {
	tests := []struct {
		name  string
		phase common.ApplicationPhase
		want  float64
	}{
		{"starting", common.ApplicationStarting, 0},
		{"running", common.ApplicationRunning, 1},
		{"rendering", common.ApplicationRendering, 2},
		{"policy generating", common.ApplicationPolicyGenerating, 3},
		{"running workflow", common.ApplicationRunningWorkflow, 4},
		{"workflow suspending", common.ApplicationWorkflowSuspending, 5},
		{"workflow terminated", common.ApplicationWorkflowTerminated, 6},
		{"workflow failed", common.ApplicationWorkflowFailed, 7},
		{"unhealthy", common.ApplicationUnhealthy, 8},
		{"deleting", common.ApplicationDeleting, 9},
		{"unknown", common.ApplicationPhase("unknown"), -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appPhaseToNumeric(tt.phase)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWorkflowPhaseToNumeric(t *testing.T) {
	tests := []struct {
		name  string
		phase workflowv1alpha1.WorkflowRunPhase
		want  float64
	}{
		{"initializing", workflowv1alpha1.WorkflowStateInitializing, 0},
		{"succeeded", workflowv1alpha1.WorkflowStateSucceeded, 1},
		{"executing", workflowv1alpha1.WorkflowStateExecuting, 2},
		{"suspending", workflowv1alpha1.WorkflowStateSuspending, 3},
		{"terminated", workflowv1alpha1.WorkflowStateTerminated, 4},
		{"failed", workflowv1alpha1.WorkflowStateFailed, 5},
		{"skipped", workflowv1alpha1.WorkflowStateSkipped, 6},
		{"unknown", workflowv1alpha1.WorkflowRunPhase("unknown"), -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workflowPhaseToNumeric(tt.phase)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildWorkflowStatus(t *testing.T) {
	tests := []struct {
		name     string
		workflow *common.WorkflowStatus
		want     map[string]interface{}
	}{
		{
			name:     "nil workflow",
			workflow: nil,
			want:     map[string]interface{}{},
		},
		{
			name: "workflow with data",
			workflow: &common.WorkflowStatus{
				AppRevision: "rev-1",
				Finished:    true,
				Phase:       workflowv1alpha1.WorkflowStateSucceeded,
				Message:     "Workflow completed",
			},
			want: map[string]interface{}{
				"app_revision": "rev-1",
				"finished":     true,
				"phase":        workflowv1alpha1.WorkflowStateSucceeded,
				"message":      "Workflow completed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildWorkflowStatus(tt.workflow)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildServiceDetails(t *testing.T) {
	tests := []struct {
		name     string
		services []common.ApplicationComponentStatus
		want     []map[string]interface{}
	}{
		{
			name:     "empty services",
			services: []common.ApplicationComponentStatus{},
			want:     []map[string]interface{}{},
		},
		{
			name: "services with details",
			services: []common.ApplicationComponentStatus{
				{
					Name:      "web",
					Namespace: "default",
					Cluster:   "local",
					Healthy:   true,
					Message:   "Running",
					Details:   map[string]string{"replicas": "3"},
				},
				{
					Name:      "db",
					Namespace: "default",
					Cluster:   "local",
					Healthy:   false,
					Message:   "Connection failed",
				},
			},
			want: []map[string]interface{}{
				{
					"name":      "web",
					"namespace": "default",
					"cluster":   "local",
					"healthy":   true,
					"message":   "Running",
					"details":   map[string]string{"replicas": "3"},
				},
				{
					"name":      "db",
					"namespace": "default",
					"cluster":   "local",
					"healthy":   false,
					"message":   "Connection failed",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildServiceDetails(tt.services)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUpdateHealthMetric(t *testing.T) {
	// Reset the metric before testing
	metrics.ApplicationHealthStatus.Reset()

	tests := []struct {
		name          string
		app           *v1beta1.Application
		healthy       bool
		expectedValue float64
	}{
		{
			name: "healthy application",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
				},
			},
			healthy:       true,
			expectedValue: 1,
		},
		{
			name: "unhealthy application",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
				},
			},
			healthy:       false,
			expectedValue: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateHealthMetric(tt.app, tt.healthy)

			value := testutil.ToFloat64(metrics.ApplicationHealthStatus.WithLabelValues(
				tt.app.Name,
				tt.app.Namespace,
			))
			assert.Equal(t, tt.expectedValue, value)
		})
	}
}

func TestUpdatePhaseMetrics(t *testing.T) {
	// Reset metrics before testing
	metrics.ApplicationPhase.Reset()
	metrics.WorkflowPhase.Reset()

	tests := []struct {
		name                  string
		app                   *v1beta1.Application
		expectedAppPhase      float64
		expectedWorkflowPhase float64
		hasWorkflowMetric     bool
	}{
		{
			name: "app with workflow",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
				},
				Status: common.AppStatus{
					Phase: common.ApplicationRunning,
					Workflow: &common.WorkflowStatus{
						Phase: workflowv1alpha1.WorkflowStateSucceeded,
					},
				},
			},
			expectedAppPhase:      1, // ApplicationRunning
			expectedWorkflowPhase: 1, // WorkflowStateSucceeded
			hasWorkflowMetric:     true,
		},
		{
			name: "app without workflow",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-2",
					Namespace: "default",
				},
				Status: common.AppStatus{
					Phase: common.ApplicationStarting,
				},
			},
			expectedAppPhase:  0, // ApplicationStarting
			hasWorkflowMetric: false,
		},
		{
			name: "app with empty workflow phase",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-3",
					Namespace: "default",
				},
				Status: common.AppStatus{
					Phase: common.ApplicationUnhealthy,
					Workflow: &common.WorkflowStatus{
						Phase: "", // Empty phase
					},
				},
			},
			expectedAppPhase:  8, // ApplicationUnhealthy
			hasWorkflowMetric: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updatePhaseMetrics(tt.app)

			appPhaseValue := testutil.ToFloat64(metrics.ApplicationPhase.WithLabelValues(
				tt.app.Name,
				tt.app.Namespace,
			))
			assert.Equal(t, tt.expectedAppPhase, appPhaseValue)

			if tt.hasWorkflowMetric {
				workflowPhaseValue := testutil.ToFloat64(metrics.WorkflowPhase.WithLabelValues(
					tt.app.Name,
					tt.app.Namespace,
				))
				assert.Equal(t, tt.expectedWorkflowPhase, workflowPhaseValue)
			}
		})
	}
}

func TestLogApplicationStatus(t *testing.T) {
	tests := []struct {
		name           string
		app            *v1beta1.Application
		healthStatus   HealthStatus
		workflowStatus map[string]interface{}
		serviceDetails []map[string]interface{}
	}{
		{
			name: "complete status",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
					UID:       "12345",
					Labels:    map[string]string{"env": "prod"},
				},
				Status: common.AppStatus{
					Phase: common.ApplicationRunning,
				},
			},
			healthStatus: HealthStatus{
				Healthy:        true,
				HealthyCount:   2,
				UnhealthyCount: 0,
			},
			workflowStatus: map[string]interface{}{
				"phase":    workflowv1alpha1.WorkflowStateSucceeded,
				"finished": true,
			},
			serviceDetails: []map[string]interface{}{
				{
					"name":    "web",
					"healthy": true,
				},
			},
		},
		{
			name: "minimal status",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-minimal",
					Namespace: "default",
				},
				Status: common.AppStatus{
					Phase: common.ApplicationStarting,
				},
			},
			healthStatus:   HealthStatus{Healthy: true},
			workflowStatus: map[string]interface{}{},
			serviceDetails: []map[string]interface{}{},
		},
		{
			name: "nil values",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-nil",
					Namespace: "default",
				},
			},
			healthStatus:   HealthStatus{},
			workflowStatus: nil,
			serviceDetails: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				traitSummary := buildTraitSummary(tt.app.Status.Services)
				logApplicationStatus(tt.app, tt.healthStatus, tt.workflowStatus, tt.serviceDetails, traitSummary)
			})
		})
	}
}

func TestUpdateMetricsAndLogFunction(t *testing.T) {
	// Reset metrics before testing
	metrics.ApplicationHealthStatus.Reset()
	metrics.ApplicationPhase.Reset()
	metrics.WorkflowPhase.Reset()

	tests := []struct {
		name string
		app  *v1beta1.Application
	}{
		{
			name: "complete application",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
					UID:       "12345",
				},
				Status: common.AppStatus{
					Phase: common.ApplicationRunning,
					Services: []common.ApplicationComponentStatus{
						{
							Name:      "web",
							Namespace: "default",
							Healthy:   true,
							Message:   "Running",
						},
						{
							Name:      "db",
							Namespace: "default",
							Healthy:   false,
							Message:   "Starting",
						},
					},
					Workflow: &common.WorkflowStatus{
						Phase:       workflowv1alpha1.WorkflowStateExecuting,
						Finished:    false,
						AppRevision: "v1",
					},
				},
			},
		},
		{
			name: "application with no services",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-empty",
					Namespace: "test",
				},
				Status: common.AppStatus{
					Phase:    common.ApplicationStarting,
					Services: []common.ApplicationComponentStatus{},
				},
			},
		},
		{
			name: "application with nil workflow",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-no-workflow",
					Namespace: "default",
				},
				Status: common.AppStatus{
					Phase: common.ApplicationUnhealthy,
					Services: []common.ApplicationComponentStatus{
						{
							Name:    "failing-service",
							Healthy: false,
						},
					},
					Workflow: nil,
				},
			},
		},
	}

	r := &Reconciler{}
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				r.updateMetricsAndLog(ctx, tt.app)
			})

			labels := prometheus.Labels{
				"app_name":  tt.app.Name,
				"namespace": tt.app.Namespace,
			}

			_, err := metrics.ApplicationHealthStatus.GetMetricWith(labels)
			assert.NoError(t, err)

			_, err = metrics.ApplicationPhase.GetMetricWith(labels)
			assert.NoError(t, err)
		})
	}
}
