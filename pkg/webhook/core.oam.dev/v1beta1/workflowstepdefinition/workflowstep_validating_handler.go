/*
Copyright 2024 The KubeVela Authors.

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

// Package workflowstepdefinition provides admission control validation
// for WorkflowStepDefinition resources in KubeVela.
package workflowstepdefinition

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/logging"
	"github.com/oam-dev/kubevela/pkg/oam"
	webhookutils "github.com/oam-dev/kubevela/pkg/webhook/utils"
)

const (
	// ValidationWebhookPath defines the HTTP path for the validation webhook
	ValidationWebhookPath = "/validating-core-oam-dev-v1beta1-workflowstepdefinitions"
	LoggerName            = "workflowstepdefinition-validator"
)

var (
	workflowStepDefGVR = v1beta1.WorkflowStepDefinitionGVR
)

// ValidatingHandler handles validation of WorkflowStepDefinition resources.
// It implements admission.Handler interface to provide webhook-based validation
// for create and update operations on WorkflowStepDefinition objects.
type ValidatingHandler struct {
	Decoder admission.Decoder
	Client  client.Client
}

// InjectClient injects the Kubernetes client into the handler.
// Called by controller-runtime during webhook setup.
func (h *ValidatingHandler) InjectClient(c client.Client) error {
	logger := log.Log.WithName(LoggerName)
	logger.Info("Injecting Kubernetes client into ValidatingHandler")

	h.Client = c
	return nil
}

// InjectDecoder injects the admission decoder into the handler.
// Called by controller-runtime during webhook setup.
func (h *ValidatingHandler) InjectDecoder(d admission.Decoder) error {
	logger := log.Log.WithName(LoggerName)
	logger.Info("Injecting admission decoder into ValidatingHandler")

	h.Decoder = d
	return nil
}

// Handle validates WorkflowStepDefinition resources during admission control.
// Performs validation for create/update operations including output resources,
// semantic version format, and definition version conflicts.
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	// create per-request ID and add to context
	ctx = logging.WithRequestID(ctx, string(req.UID))
	logger := logging.NewHandlerLogger(ctx, LoggerName, req)

	logger.Info("Processing admission request for WorkflowStepDefinition")

	// Validate that the request is for the expected resource type
	if req.Resource.String() != workflowStepDefGVR.String() {
		err := fmt.Errorf("expected resource to be %s, got %s", workflowStepDefGVR, req.Resource.String())
		logger.Error(err, "Resource type mismatch in admission request")
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Only validate create and update operations
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		logger.Info("Skipping validation for non-create/update operation")
		return admission.ValidationResponse(true, "Operation does not require validation")
	}

	// Decode the WorkflowStepDefinition object from the request
	obj := &v1beta1.WorkflowStepDefinition{}
	if err := h.Decoder.Decode(req, obj); err != nil {
		logger.Error(err, "Failed to decode WorkflowStepDefinition from admission request")
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode object: %w", err))
	}

	logger = logging.WithValuesCtx(ctx, logger,
		"definitionName", obj.Name,
		"definitionVersion", obj.Spec.Version,
	)
	logger.Info("Successfully decoded WorkflowStepDefinition object")

	// Perform validation checks
	if err := h.validateOutputResources(ctx, obj, logger); err != nil {
		return admission.Denied(err.Error())
	}

	if err := h.validateSemanticVersion(ctx, obj, logger); err != nil {
		return admission.Denied(err.Error())
	}

	if err := h.validateVersionConflicts(ctx, obj, logger); err != nil {
		return admission.Denied(err.Error())
	}

	logger.Info("WorkflowStepDefinition validation completed successfully")
	return admission.ValidationResponse(true, "Validation passed")
}

// validateOutputResources ensures all resources referenced in CUE template outputs exist on cluster.
func (h *ValidatingHandler) validateOutputResources(ctx context.Context, obj *v1beta1.WorkflowStepDefinition, logger logr.Logger) error {
	_ = ctx // reserved for future use (timeouts, tracing, etc.)
	if obj.Spec.Schematic == nil || obj.Spec.Schematic.CUE == nil {
		logger.Info("No CUE template found, skipping output resource validation")
		return nil
	}

	logger.Info("Validating output resources exist on cluster")

	if err := webhookutils.ValidateOutputResourcesExist(obj.Spec.Schematic.CUE.Template, h.Client.RESTMapper()); err != nil {
		logger.Error(err, "Output resource validation failed",
			"template", obj.Spec.Schematic.CUE.Template)
		return fmt.Errorf("output resource validation failed: %w", err)
	}

	logger.Info("Output resource validation passed")
	return nil
}

// validateSemanticVersion validates the version field follows semantic versioning rules.
func (h *ValidatingHandler) validateSemanticVersion(ctx context.Context, obj *v1beta1.WorkflowStepDefinition, logger logr.Logger) error {
	_ = ctx
	if obj.Spec.Version == "" {
		logger.Info("No version specified, skipping semantic version validation")
		return nil
	}

	logger.Info("Validating semantic version format", "version", obj.Spec.Version)

	if err := webhookutils.ValidateSemanticVersion(obj.Spec.Version); err != nil {
		logger.Error(err, "Semantic version validation failed", "version", obj.Spec.Version)
		return fmt.Errorf("semantic version validation failed: %w", err)
	}

	logger.Info("Semantic version validation passed")
	return nil
}

// validateVersionConflicts checks for conflicts between version and revision annotations.
func (h *ValidatingHandler) validateVersionConflicts(ctx context.Context, obj *v1beta1.WorkflowStepDefinition, logger logr.Logger) error {
	_ = ctx
	revisionName := obj.Annotations[oam.AnnotationDefinitionRevisionName]
	version := obj.Spec.Version

	logger.Info("Validating definition version conflicts",
		"revisionName", revisionName,
		"version", version,
		"kind", obj.Kind)

	if err := webhookutils.ValidateMultipleDefVersionsNotPresent(version, revisionName, obj.Kind); err != nil {
		logger.Error(err, "Definition version conflict detected",
			"revisionName", revisionName,
			"version", version,
			"kind", obj.Kind)
		return fmt.Errorf("definition version conflict: %w", err)
	}

	logger.Info("Version conflict validation passed")
	return nil
}

// RegisterValidatingHandler registers the WorkflowStepDefinition validation webhook with the manager.
// Sets up the HTTP endpoint to handle admission requests for WorkflowStepDefinition resources.
func RegisterValidatingHandler(mgr manager.Manager) {
	logger := log.Log.WithName(LoggerName)
	logger.Info("Registering WorkflowStepDefinition validation webhook", "path", ValidationWebhookPath)

	server := mgr.GetWebhookServer()
	server.Register(ValidationWebhookPath, &webhook.Admission{
		Handler: &ValidatingHandler{
			Client:  mgr.GetClient(),
			Decoder: admission.NewDecoder(mgr.GetScheme()),
		},
	})
	logger.Info("WorkflowStepDefinition validation webhook registered successfully")
}
