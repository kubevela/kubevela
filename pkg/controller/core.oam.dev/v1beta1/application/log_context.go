/*
Copyright 2025 The KubeVela Authors.

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

	"github.com/google/uuid"
	ctrl "sigs.k8s.io/controller-runtime"

	monitorContext "github.com/kubevela/pkg/monitor/context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/logging"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// ApplicationLogContext builds the monitorContext for one Application
// reconcile. It resolves the trace ID from the app's annotation
// (stamped by the mutating webhook) or mints a fresh UUID when the
// webhook was bypassed, and mints a separate fresh UUID for this
// reconcile's root span ID. The trace ID stays stable across reconciles
// of the same app; the span ID is unique per reconcile so log lines
// from different reconciles of the same app can be co-related
// independently via the spanID field.
//
// The trace ID is also stored on the std context (as requestID) so
// non-monitor consumers see the same value via logging.FromContext,
// and is added as the requestID tag so it shares the log field name
// used by both webhooks.
//
// Returns the log context and the resolved trace ID. Callers need the
// trace ID separately to seed downstream std contexts (logCtx.GetID()
// now returns the reconcile span ID, not the trace ID).
func ApplicationLogContext(ctx context.Context, app *v1beta1.Application, req ctrl.Request) (monitorContext.Context, string) {
	traceID, _ := logging.TraceIDFromObject(app)
	if traceID == "" {
		traceID = uuid.NewString()
	}
	// Per-reconcile span ID — unique even when the trace ID persists
	// across many reconciles of the same Application.
	reconcileSpanID := uuid.NewString()

	ctx = logging.WithRequestID(ctx, traceID)

	logCtx := monitorContext.NewTraceContext(ctx, reconcileSpanID)
	return logCtx.AddTag(
		logging.FieldRequestID, traceID,
		"application", req.String(),
		"controller", "application",
		logging.FieldName, app.Name,
		logging.FieldNamespace, app.Namespace,
		"app_uid", string(app.UID),
		logging.FieldGeneration, app.Generation,
		"resource_version", app.ResourceVersion,
		"publish_version", app.GetAnnotations()[oam.AnnotationPublishVersion],
		"reconcile_reason", determineApplicationReconcileReason(app),
	), traceID
}

// determineApplicationReconcileReason returns a coarse hint about why
// the reconciler woke up. Conservative on purpose: only the cases we
// can read unambiguously from app state. Update / periodic / manual
// expansion is intentionally deferred.
func determineApplicationReconcileReason(app *v1beta1.Application) string {
	if app == nil {
		return "unknown"
	}
	if !app.DeletionTimestamp.IsZero() {
		return "delete"
	}
	if v, ok := app.Annotations[oam.AnnotationWorkflowRestart]; ok && v != "" {
		return "workflow_restart"
	}
	return "unknown"
}
