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

package application

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	monitorContext "github.com/kubevela/pkg/monitor/context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	reconcileReasonDelete          = "delete"
	reconcileReasonUnknown         = "unknown"
	reconcileReasonWorkflowRestart = "workflow_restart"
)

// NewApplicationRequestContext creates the root logging context for an Application
// reconcile request. At this point the Application may not have been fetched yet,
// so only request-level fields are attached.
func NewApplicationRequestContext(ctx context.Context, req ctrl.Request) monitorContext.Context {
	logCtx := monitorContext.NewTraceContext(ctx, "")
	return logCtx.AddTag(
		"trace_id", logCtx.GetID(),
		"application", req.String(),
		"controller", "application",
	)
}

// EnrichApplicationReconcileContext adds Application metadata to the root
// reconcile logging context once the Application has been loaded.
func EnrichApplicationReconcileContext(logCtx monitorContext.Context, app *v1beta1.Application) monitorContext.Context {
	return logCtx.AddTag(
		"app_name", app.Name,
		"app_namespace", app.Namespace,
		"app_uid", string(app.UID),
		"resource_version", app.ResourceVersion,
		"generation", app.Generation,
		"publish_version", app.GetAnnotations()[oam.AnnotationPublishVersion],
		"reconcile_reason", determineApplicationReconcileReason(app),
	)
}

func determineApplicationReconcileReason(app *v1beta1.Application) string {
	if app == nil {
		return reconcileReasonUnknown
	}
	if !app.ObjectMeta.DeletionTimestamp.IsZero() {
		return reconcileReasonDelete
	}
	if metav1.HasAnnotation(app.ObjectMeta, oam.AnnotationWorkflowRestart) {
		return reconcileReasonWorkflowRestart
	}
	return reconcileReasonUnknown
}
