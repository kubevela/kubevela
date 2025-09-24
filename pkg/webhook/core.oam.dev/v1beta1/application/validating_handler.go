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

	logger.LogStep("start", logging.FieldOperation, req.Operation)

	// Decode the application
	app := &v1beta1.Application{}
	if err := h.Decoder.Decode(req, app); err != nil {
		logger.LogError(err, "decode-application")
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode: %w (requestUID=%s)", err, req.UID))
	}

	if req.Namespace != "" {
		app.Namespace = req.Namespace
	}

	logger = logger.WithValues(logging.FieldGeneration, app.Generation)
	logger.LogStep("decoded")

	ctx = util.SetNamespaceInCtx(ctx, app.Namespace)

	switch req.Operation {
	case admissionv1.Create:
		logger.LogStep("validate-create")
		if allErrs := h.ValidateCreate(ctx, app, req); len(allErrs) > 0 {
			mergedErr := mergeErrors(allErrs)
			logger.LogError(mergedErr, "validate-create", "errorCount", len(allErrs))
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("%w (requestUID=%s)", mergedErr, req.UID))
		}
		logger.LogValidation("create", true)

	case admissionv1.Update:
		logger.LogStep("validate-update")
		oldApp := &v1beta1.Application{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldApp); err != nil {
			logger.LogError(err, "decode-old-application")
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("%w (requestUID=%s)", simplifyError(err), req.UID))
		}

		logger = logger.WithValues("oldGeneration", oldApp.Generation)

		if app.ObjectMeta.DeletionTimestamp.IsZero() {
			if allErrs := h.ValidateUpdate(ctx, app, oldApp, req); len(allErrs) > 0 {
				mergedErr := mergeErrors(allErrs)
				logger.LogError(mergedErr, "validate-update", "errorCount", len(allErrs))
				return admission.Errored(http.StatusBadRequest, fmt.Errorf("%w (requestUID=%s)", mergedErr, req.UID))
			}
			logger.LogValidation("update", true)
		} else {
			logger.LogStep("skip-validation", "reason", "deleting")
		}

	case admissionv1.Delete:
		logger.LogStep("skip-validation", "reason", "delete-operation")

	default:
		logger.LogStep("skip-validation", "reason", "unsupported-operation")
	}

	logger.LogSuccess("application-validation", startTime)
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
