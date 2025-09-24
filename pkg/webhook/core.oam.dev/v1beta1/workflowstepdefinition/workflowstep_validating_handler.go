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
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
)

var (
	workflowStepDefGVR = v1beta1.WorkflowStepDefinitionGVR
)

// ValidatingHandler handles validation of WorkflowStepDefinition resources.
type ValidatingHandler struct {
	Decoder admission.Decoder
	Client  client.Client
}

// InjectClient injects the Kubernetes client into the handler.
func (h *ValidatingHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

// InjectDecoder injects the admission decoder into the handler.
func (h *ValidatingHandler) InjectDecoder(d admission.Decoder) error {
	h.Decoder = d
	return nil
}

// Handle validates WorkflowStepDefinition resources during admission control.
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	startTime := time.Now()
	ctx = logging.WithRequestID(ctx, string(req.UID))
	logger := logging.NewHandlerLogger(ctx, req, "WorkflowStepDefinitionValidator")

	logger.LogStep("start")

	// Validate resource type
	if req.Resource.String() != workflowStepDefGVR.String() {
		err := fmt.Errorf("expected resource to be %s, got %s", workflowStepDefGVR, req.Resource.String())
		logger.LogError(err, "resource-mismatch")
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s (requestUID=%s)", err.Error(), req.UID))
	}

	// Only validate create and update operations
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		logger.LogStep("skip-validation", "reason", "unsupported-operation")
		return admission.ValidationResponse(true, "Operation does not require validation")
	}

	// Decode the object
	obj := &v1beta1.WorkflowStepDefinition{}
	if err := h.Decoder.Decode(req, obj); err != nil {
		logger.LogError(err, "decode")
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode: %s (requestUID=%s)", err.Error(), req.UID))
	}

	if obj.Spec.Version != "" {
		logger = logger.WithValues("version", obj.Spec.Version)
	}
	logger.LogStep("decoded")

	// Validate output resources
	if obj.Spec.Schematic != nil && obj.Spec.Schematic.CUE != nil {
		if err := webhookutils.ValidateOutputResourcesExist(obj.Spec.Schematic.CUE.Template, h.Client.RESTMapper()); err != nil {
			logger.LogError(err, "validate-output-resources")
			return admission.Denied(fmt.Sprintf("output resource validation failed: %s (requestUID=%s)", err.Error(), req.UID))
		}
		logger.LogValidation("output-resources", true)
	}

	// Validate semantic version
	if obj.Spec.Version != "" {
		if err := webhookutils.ValidateSemanticVersion(obj.Spec.Version); err != nil {
			logger.LogError(err, "validate-version", "version", obj.Spec.Version)
			return admission.Denied(fmt.Sprintf("semantic version validation failed: %s (requestUID=%s)", err.Error(), req.UID))
		}
		logger.LogValidation("version", true)
	}

	// Validate version conflicts
	revisionName := obj.Annotations[oam.AnnotationDefinitionRevisionName]
	if err := webhookutils.ValidateMultipleDefVersionsNotPresent(obj.Spec.Version, revisionName, obj.Kind); err != nil {
		logger.LogError(err, "validate-version-conflict", "revisionName", revisionName)
		return admission.Denied(fmt.Sprintf("definition version conflict: %s (requestUID=%s)", err.Error(), req.UID))
	}

	logger.LogSuccess("workflow-step-definition-validation", startTime)
	return admission.ValidationResponse(true, "Validation passed")
}

// RegisterValidatingHandler registers the WorkflowStepDefinition validation webhook with the manager.
func RegisterValidatingHandler(mgr manager.Manager) {
	logger := logging.New()
	logger.Info("Registering WorkflowStepDefinition validation webhook", "path", ValidationWebhookPath)

	server := mgr.GetWebhookServer()
	server.Register(ValidationWebhookPath, &webhook.Admission{
		Handler: &ValidatingHandler{
			Client:  mgr.GetClient(),
			Decoder: admission.NewDecoder(mgr.GetScheme()),
		},
	})
}