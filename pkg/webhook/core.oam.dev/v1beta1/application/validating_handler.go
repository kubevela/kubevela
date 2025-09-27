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
	"errors"
	"fmt"
	"net/http"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/logging"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ admission.Handler = &ValidatingHandler{}

// ValidatingHandler handles application
type ValidatingHandler struct {
	Client client.Client
	// Decoder decodes objects
	Decoder admission.Decoder
}

func simplifyError(err error) error {
	switch e := err.(type) { // nolint
	case *field.Error:
		return fmt.Errorf("field \"%s\": %s error encountered, %s. ", e.Field, e.Type, e.Detail)
	default:
		return err
	}
}

func mergeErrors(errs field.ErrorList) error {
	s := ""
	for _, err := range errs {
		s += fmt.Sprintf("field \"%s\": %s error encountered, %s. ", err.Field, err.Type, err.Detail)
	}
	return errors.New(s)
}

// Handle validate Application Spec here
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	startTime := time.Now()
	ctx = logging.WithRequestID(ctx, string(req.UID))
	logger := logging.NewHandlerLogger(ctx, req, "ApplicationValidator")

	logger.WithStep("start").Info("Starting admission validation for Application resource", "operation", req.Operation, "applicationName", req.Name, "namespace", req.Namespace)

	// Decode the application
	app := &v1beta1.Application{}
	if err := h.Decoder.Decode(req, app); err != nil {
		logger.WithStep("decode").WithError(err).Error(err, "Unable to decode admission request payload into Application object - malformed request")
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode: %w (requestUID=%s)", err, req.UID))
	}

	if req.Namespace != "" {
		app.Namespace = req.Namespace
	}

	logger = logger.WithValues(logging.FieldGeneration, app.Generation)

	workflowSteps := 0
	if app.Spec.Workflow != nil {
		workflowSteps = len(app.Spec.Workflow.Steps)
	}

	logger.WithStep("decode").Info("Successfully decoded Application from admission request",
		"applicationName", app.Name,
		"namespace", app.Namespace,
		"componentCount", len(app.Spec.Components),
		"policyCount", len(app.Spec.Policies),
		"workflowSteps", workflowSteps)

	ctx = util.SetNamespaceInCtx(ctx, app.Namespace)

	switch req.Operation {
	case admissionv1.Create:
		logger.WithStep("validate-create").Info("Validating Application creation - checking components, policies, and workflow configuration")
		if allErrs := h.ValidateCreate(ctx, app, req); len(allErrs) > 0 {
			mergedErr := mergeErrors(allErrs)
			logger.WithStep("validate-create").WithError(mergedErr).Error(mergedErr, "Application creation validation failed - contains invalid components, policies, or workflow steps", "errorCount", len(allErrs), "applicationName", app.Name)
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("%w (requestUID=%s)", mergedErr, req.UID))
		}
		logger.WithStep("validate-create").WithSuccess(true).Info("Application creation validation completed successfully - all components, policies, and workflows are valid", "applicationName", app.Name)

	case admissionv1.Update:
		logger.WithStep("validate-update").Info("Validating Application update - comparing new configuration with existing state")
		oldApp := &v1beta1.Application{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldApp); err != nil {
			logger.WithStep("decode-old").WithError(err).Error(err, "Unable to decode previous Application state from admission request - cannot validate update")
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("%w (requestUID=%s)", simplifyError(err), req.UID))
		}

		logger = logger.WithValues("oldGeneration", oldApp.Generation)

		if app.ObjectMeta.DeletionTimestamp.IsZero() {
			if allErrs := h.ValidateUpdate(ctx, app, oldApp, req); len(allErrs) > 0 {
				mergedErr := mergeErrors(allErrs)
				logger.WithStep("validate-update").WithError(mergedErr).Error(mergedErr, "Application update validation failed - new configuration contains invalid changes", "errorCount", len(allErrs), "applicationName", app.Name, "oldGeneration", oldApp.Generation, "newGeneration", app.Generation)
				return admission.Errored(http.StatusBadRequest, fmt.Errorf("%w (requestUID=%s)", mergedErr, req.UID))
			}
			logger.WithStep("validate-update").WithSuccess(true).Info("Application update validation completed successfully - configuration changes are valid", "applicationName", app.Name, "generationChange", fmt.Sprintf("%d->%d", oldApp.Generation, app.Generation))
		} else {
			logger.WithStep("skip-validation").Info("Skipping Application validation - resource is being deleted and validation is not required", "reason", "deletion-in-progress", "deletionTimestamp", app.DeletionTimestamp)
		}

	case admissionv1.Delete:
		logger.WithStep("skip-validation").Info("Skipping Application validation - DELETE operations do not require content validation", "reason", "delete-operation-no-validation-needed")

	default:
		logger.WithStep("skip-validation").Info("Skipping Application validation - operation type is not supported by validator", "operation", req.Operation, "reason", "only CREATE, UPDATE, and DELETE operations are handled")
	}

	logger.WithStep("complete").WithSuccess(true, startTime).Info("Application admission validation completed successfully - resource will be admitted", "applicationName", req.Name, "operation", req.Operation, "namespace", req.Namespace)
	return admission.ValidationResponse(true, "")
}

// RegisterValidatingHandler will register application validate handler to the webhook
func RegisterValidatingHandler(mgr manager.Manager, _ controller.Args) {
	server := mgr.GetWebhookServer()
	server.Register("/validating-core-oam-dev-v1beta1-applications", &webhook.Admission{Handler: &ValidatingHandler{
		Client:  mgr.GetClient(),
		Decoder: admission.NewDecoder(mgr.GetScheme()),
	}})
}
