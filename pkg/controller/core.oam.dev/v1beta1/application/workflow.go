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
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	monitorContext "github.com/kubevela/pkg/monitor/context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// handleWorkflowRestartAnnotation processes the app.oam.dev/restart-workflow annotation
// and converts it to status.workflowRestartScheduledAt for GitOps safety.
// For timestamps, it deletes the annotation after copying to status (persisted via Client.Update).
// For durations, it keeps the annotation and reschedules after each execution based on time comparison.
func (r *Reconciler) handleWorkflowRestartAnnotation(ctx context.Context, app *v1beta1.Application) {
	if !metav1.HasAnnotation(app.ObjectMeta, oam.AnnotationWorkflowRestart) {
		return
	}

	restartValue := app.Annotations[oam.AnnotationWorkflowRestart]

	var scheduledTime time.Time
	var isDuration bool
	var statusFieldNeedsUpdate bool

	if restartValue == "true" {
		// "true" is a convenience value supplied for an immediate restart
		scheduledTime = time.Now()
		isDuration = false
		statusFieldNeedsUpdate = true
	} else if parsedTime, err := time.Parse(time.RFC3339, restartValue); err == nil {
		// explicit timestamp - restart on first reconcile > time
		scheduledTime = parsedTime
		isDuration = false
		statusFieldNeedsUpdate = true
	} else if duration, err := time.ParseDuration(restartValue); err == nil {
		// recurring duration - calculate relative to last successful workflow completion
		baseTime := time.Now()
		if app.Status.Workflow != nil && app.Status.Workflow.Finished && !app.Status.Workflow.EndTime.IsZero() {
			baseTime = app.Status.Workflow.EndTime.Time
		}
		scheduledTime = baseTime.Add(duration)
		isDuration = true

		// Only update if status is nil OR the calculated value differs from current status
		statusFieldNeedsUpdate = app.Status.WorkflowRestartScheduledAt == nil ||
			!app.Status.WorkflowRestartScheduledAt.Time.Equal(scheduledTime)
	} else {
		klog.Warningf("Invalid workflow restart annotation value for Application %s/%s: %q. Expected 'true', RFC3339 timestamp, or duration (e.g., '5m', '1h')",
			app.Namespace, app.Name, restartValue)
		return
	}

	if statusFieldNeedsUpdate {
		app.Status.WorkflowRestartScheduledAt = &metav1.Time{Time: scheduledTime}
		if err := r.Status().Update(ctx, app); err != nil {
			klog.Errorf("Failed to update workflow restart status for Application %s/%s: %v. Will retry on next reconcile.",
				app.Namespace, app.Name, err)
			// Don't fail reconciliation - will retry naturally on next reconcile
		}
	}

	// For timestamps, delete the annotation (one-time behavior)
	// For durations, keep the annotation (recurring behavior)
	if !isDuration {
		delete(app.Annotations, oam.AnnotationWorkflowRestart)
		if err := r.Client.Update(ctx, app); err != nil {
			klog.Errorf("Failed to remove workflow restart annotation for Application %s/%s: %v. Will retry on next reconcile.",
				app.Namespace, app.Name, err)
			// Don't fail reconciliation - will retry naturally on next reconcile
		}
	}
}

// checkWorkflowRestart checks if application workflow needs restart.
// Handles three restart scenarios:
// 1. Scheduled restart (via workflowRestartScheduledAt status field)
// 2. PublishVersion annotation change
// 3. Application revision change
func (r *Reconciler) checkWorkflowRestart(ctx monitorContext.Context, app *v1beta1.Application, handler *AppHandler) {
	// Check for scheduled restart in status field
	if app.Status.WorkflowRestartScheduledAt != nil {
		restartTime := app.Status.WorkflowRestartScheduledAt.Time

		if time.Now().Before(restartTime) {
			// Not yet time to restart, skip for now
			return
		}
		if app.Status.Workflow == nil || !app.Status.Workflow.Finished {
			// Workflow is still running or hasn't started - don't restart yet
			return
		}
		if app.Status.Workflow != nil && !app.Status.Workflow.EndTime.IsZero() {
			lastEndTime := app.Status.Workflow.EndTime.Time
			if !restartTime.After(lastEndTime) {
				// Restart time is not after last execution, skip
				return
			}
		}

		// All conditions met: time arrived, workflow finished, and restart time > last execution
		// Clear the status field and proceed with restart
		app.Status.WorkflowRestartScheduledAt = nil
		if err := r.Status().Update(ctx, app); err != nil {
			ctx.Error(err, "failed to clear workflow restart scheduled time")
			return
		}
		if app.Status.Workflow != nil {
			if handler.latestAppRev != nil && handler.latestAppRev.Status.Workflow == nil {
				app.Status.Workflow.Terminated = true
				app.Status.Workflow.Finished = true
				if app.Status.Workflow.EndTime.IsZero() {
					app.Status.Workflow.EndTime = metav1.Now()
				}
				handler.UpdateApplicationRevisionStatus(ctx, handler.latestAppRev, app.Status.Workflow)
			}
		}

		app.Status.Services = nil
		app.Status.AppliedResources = nil
		var reservedConditions []condition.Condition
		for i, cond := range app.Status.Conditions {
			condTpy, err := common.ParseApplicationConditionType(string(cond.Type))
			if err == nil {
				if condTpy <= common.RenderCondition {
					reservedConditions = append(reservedConditions, app.Status.Conditions[i])
				}
			}
		}
		app.Status.Conditions = reservedConditions
		app.Status.Workflow = &common.WorkflowStatus{
			AppRevision: handler.currentAppRev.Name,
		}
		return
	}

	// Check for revision-based restart (publishVersion or normal revision change)
	desiredRev, currentRev := handler.currentAppRev.Name, ""
	if app.Status.Workflow != nil {
		currentRev = app.Status.Workflow.AppRevision
	}
	if metav1.HasAnnotation(app.ObjectMeta, oam.AnnotationPublishVersion) {
		desiredRev = app.GetAnnotations()[oam.AnnotationPublishVersion]
	} else { // nolint
		// backward compatibility
		// legacy versions use <rev>:<hash> as currentRev, extract <rev>
		if idx := strings.LastIndexAny(currentRev, ":"); idx >= 0 {
			currentRev = currentRev[:idx]
		}
	}
	if currentRev != "" && desiredRev == currentRev {
		return
	}

	// Restart needed - record in revision and clean up
	if app.Status.Workflow != nil {
		if handler.latestAppRev != nil && handler.latestAppRev.Status.Workflow == nil {
			app.Status.Workflow.Terminated = true
			app.Status.Workflow.Finished = true
			if app.Status.Workflow.EndTime.IsZero() {
				app.Status.Workflow.EndTime = metav1.Now()
			}
			handler.UpdateApplicationRevisionStatus(ctx, handler.latestAppRev, app.Status.Workflow)
		}
	}

	app.Status.Services = nil
	app.Status.AppliedResources = nil
	var reservedConditions []condition.Condition
	for i, cond := range app.Status.Conditions {
		condTpy, err := common.ParseApplicationConditionType(string(cond.Type))
		if err == nil {
			if condTpy <= common.RenderCondition {
				reservedConditions = append(reservedConditions, app.Status.Conditions[i])
			}
		}
	}
	app.Status.Conditions = reservedConditions
	app.Status.Workflow = &common.WorkflowStatus{
		AppRevision: desiredRev,
	}
}
