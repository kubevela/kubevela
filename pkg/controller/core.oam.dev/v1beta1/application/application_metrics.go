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

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
)

// HealthStatus represents the health status of an application
type HealthStatus struct {
	Healthy                 bool
	HealthyCount            int
	UnhealthyCount          int
	DispatchHealthyCount    int // Components ready for PostDispatch traits
	PendingPostDispatchCount int // Components with pending PostDispatch traits
}

// updateMetricsAndLog updates Prometheus metrics and logs application status with service details
func (r *Reconciler) updateMetricsAndLog(_ context.Context, app *v1beta1.Application) {
	healthStatus := calculateHealthStatus(app.Status.Services)

	updateHealthMetric(app, healthStatus.Healthy)
	updatePhaseMetrics(app)

	workflowStatus := buildWorkflowStatus(app.Status.Workflow)
	serviceDetails := buildServiceDetails(app.Status.Services)
	traitSummary := buildTraitSummary(app.Status.Services)
	logApplicationStatus(app, healthStatus, workflowStatus, serviceDetails, traitSummary)
}

// calculateHealthStatus calculates the health status from services
func calculateHealthStatus(services []common.ApplicationComponentStatus) HealthStatus {
	status := HealthStatus{
		Healthy: true,
	}

	for _, svc := range services {
		// Count by detailed health status if available
		switch svc.HealthStatus {
		case "DispatchHealthy":
			status.DispatchHealthyCount++
			// Check if has pending PostDispatch traits
			hasPendingPostDispatch := false
			for _, trait := range svc.Traits {
				if trait.Stage == "PostDispatch" && trait.GetEffectiveState() != common.StateDispatched {
					hasPendingPostDispatch = true
					break
				}
			}
			if hasPendingPostDispatch {
				status.PendingPostDispatchCount++
			}
		case "Healthy":
			status.HealthyCount++
		case "Unhealthy":
			status.UnhealthyCount++
			status.Healthy = false
		default:
			// Fallback to boolean logic for compatibility
			if svc.Healthy {
				status.HealthyCount++
			} else {
				status.UnhealthyCount++
				status.Healthy = false
			}
		}
	}

	// Application is only fully healthy if no unhealthy components and no pending PostDispatch
	if status.UnhealthyCount > 0 || status.PendingPostDispatchCount > 0 {
		status.Healthy = false
	}

	return status
}

// buildTraitSummary creates a summary of all traits across services
func buildTraitSummary(services []common.ApplicationComponentStatus) map[string]interface{} {
	summary := map[string]interface{}{
		"total":     0,
		"healthy":   0,
		"unhealthy": 0,
		"pending":   0,
	}

	for _, svc := range services {
		for _, trait := range svc.Traits {
			summary["total"] = summary["total"].(int) + 1

			if trait.GetEffectiveState() != common.StateDispatched {
				summary["pending"] = summary["pending"].(int) + 1
			} else if trait.Healthy {
				summary["healthy"] = summary["healthy"].(int) + 1
			} else {
				summary["unhealthy"] = summary["unhealthy"].(int) + 1
			}
		}
	}

	return summary
}

// updateHealthMetric updates the application health status metric
func updateHealthMetric(app *v1beta1.Application, healthy bool) {
	healthValue := float64(1)
	if !healthy {
		healthValue = float64(0)
	}

	metrics.ApplicationHealthStatus.WithLabelValues(
		app.Name,
		app.Namespace,
	).Set(healthValue)
}

// updatePhaseMetrics updates the application and workflow phase metrics
func updatePhaseMetrics(app *v1beta1.Application) {
	metrics.ApplicationPhase.WithLabelValues(
		app.Name,
		app.Namespace,
	).Set(appPhaseToNumeric(app.Status.Phase))

	if app.Status.Workflow != nil && app.Status.Workflow.Phase != "" {
		metrics.WorkflowPhase.WithLabelValues(
			app.Name,
			app.Namespace,
		).Set(workflowPhaseToNumeric(app.Status.Workflow.Phase))
	}
}

// buildWorkflowStatus builds workflow status information for logging
func buildWorkflowStatus(workflow *common.WorkflowStatus) map[string]interface{} {
	if workflow == nil {
		return make(map[string]interface{})
	}

	return map[string]interface{}{
		"app_revision": workflow.AppRevision,
		"finished":     workflow.Finished,
		"phase":        workflow.Phase,
		"message":      workflow.Message,
	}
}

// buildServiceDetails builds service details for logging
func buildServiceDetails(services []common.ApplicationComponentStatus) []map[string]interface{} {
	serviceDetails := make([]map[string]interface{}, 0, len(services))

	for _, svc := range services {
		svcDetails := map[string]interface{}{
			"name":      svc.Name,
			"namespace": svc.Namespace,
			"cluster":   svc.Cluster,
			"healthy":   svc.Healthy,
			"message":   svc.Message,
		}
		
		// Add enhanced health status if available
		if svc.HealthStatus != "" {
			svcDetails["health_status"] = svc.HealthStatus
		}
		
		if len(svc.Details) > 0 {
			svcDetails["details"] = svc.Details
		}
		
		// Add trait information with stage details and summary
		if len(svc.Traits) > 0 {
			traitDetails := make([]map[string]interface{}, 0, len(svc.Traits))
			traitSummary := map[string]interface{}{
				"total":     0,
				"healthy":   0,
				"unhealthy": 0,
				"pending":   0,
			}
			
			for _, trait := range svc.Traits {
				traitDetail := map[string]interface{}{
					"type":    trait.Type,
					"healthy": trait.Healthy,
					"state":   string(trait.GetEffectiveState()),
				}
				if trait.Stage != "" {
					traitDetail["stage"] = trait.Stage
				}
				if trait.Message != "" {
					traitDetail["message"] = trait.Message
				}
				if len(trait.Details) > 0 {
					traitDetail["details"] = trait.Details
				}
				traitDetails = append(traitDetails, traitDetail)
				
				// Update per-service trait summary
				traitSummary["total"] = traitSummary["total"].(int) + 1
				if trait.GetEffectiveState() != common.StateDispatched {
					traitSummary["pending"] = traitSummary["pending"].(int) + 1
				} else if trait.Healthy {
					traitSummary["healthy"] = traitSummary["healthy"].(int) + 1
				} else {
					traitSummary["unhealthy"] = traitSummary["unhealthy"].(int) + 1
				}
			}
			
			svcDetails["traits"] = traitDetails
			svcDetails["trait_summary"] = traitSummary
		}
		
		serviceDetails = append(serviceDetails, svcDetails)
	}

	return serviceDetails
}

// logApplicationStatus logs the application status with structured data
func logApplicationStatus(app *v1beta1.Application, healthStatus HealthStatus, workflowStatus map[string]interface{}, serviceDetails []map[string]interface{}, traitSummary map[string]interface{}) {
	statusDetails := map[string]interface{}{
		"app_uid":   app.UID,
		"app_name":  app.Name,
		"version":   app.ResourceVersion,
		"namespace": app.Namespace,
		"labels":    app.Labels,
		"status": map[string]interface{}{
			"phase":                         string(app.Status.Phase),
			"healthy":                       healthStatus.Healthy,
			"healthy_services_count":        healthStatus.HealthyCount,
			"unhealthy_services_count":      healthStatus.UnhealthyCount,
			"dispatch_healthy_count":        healthStatus.DispatchHealthyCount,
			"pending_postdispatch_count":    healthStatus.PendingPostDispatchCount,
			"services":                      serviceDetails,
			"traits":                        traitSummary,
			"workflow":                      workflowStatus,
		},
	}

	klog.InfoS("application update",
		"app_uid", app.UID,
		"app_name", app.Name,
		"namespace", app.Namespace,
		"phase", string(app.Status.Phase),
		"healthy", healthStatus.Healthy,
		"data", statusDetails,
	)
}

// appPhaseToNumeric converts application phase to numeric value for metrics
func appPhaseToNumeric(phase common.ApplicationPhase) float64 {
	switch phase {
	case common.ApplicationStarting:
		return 0
	case common.ApplicationRunning:
		return 1
	case common.ApplicationRendering:
		return 2
	case common.ApplicationPolicyGenerating:
		return 3
	case common.ApplicationRunningWorkflow:
		return 4
	case common.ApplicationWorkflowSuspending:
		return 5
	case common.ApplicationWorkflowTerminated:
		return 6
	case common.ApplicationWorkflowFailed:
		return 7
	case common.ApplicationUnhealthy:
		return 8
	case common.ApplicationDeleting:
		return 9
	default:
		return -1
	}
}

// workflowPhaseToNumeric converts workflow phase to numeric value for metrics
func workflowPhaseToNumeric(phase workflowv1alpha1.WorkflowRunPhase) float64 {
	switch phase {
	case workflowv1alpha1.WorkflowStateInitializing:
		return 0
	case workflowv1alpha1.WorkflowStateSucceeded:
		return 1
	case workflowv1alpha1.WorkflowStateExecuting:
		return 2
	case workflowv1alpha1.WorkflowStateSuspending:
		return 3
	case workflowv1alpha1.WorkflowStateTerminated:
		return 4
	case workflowv1alpha1.WorkflowStateFailed:
		return 5
	case workflowv1alpha1.WorkflowStateSkipped:
		return 6
	default:
		return -1
	}
}
