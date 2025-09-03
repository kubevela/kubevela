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

package traitdefinition

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
	admissionv1 "k8s.io/api/admission/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/logging"
	"github.com/oam-dev/kubevela/pkg/oam"
	webhookutils "github.com/oam-dev/kubevela/pkg/webhook/utils"
)

const (
	errValidateDefRef = "error occurs when validating definition reference"

	failInfoDefRefOmitted = "if definition reference is omitted, patch or output with GVK is required"
)

var traitDefGVR = v1beta1.TraitDefinitionGVR

// ValidatingHandler handles validation of trait definition
type ValidatingHandler struct {
	Client client.Client

	// Decoder decodes object
	Decoder admission.Decoder
	// Validators validate objects
	Validators []TraitDefValidator
}

// TraitDefValidator validate trait definition
type TraitDefValidator interface {
	Validate(context.Context, v1beta1.TraitDefinition) error
}

// TraitDefValidatorFn implements TraitDefValidator
type TraitDefValidatorFn func(context.Context, v1beta1.TraitDefinition) error

// Validate implements TraitDefValidator method
func (fn TraitDefValidatorFn) Validate(ctx context.Context, td v1beta1.TraitDefinition) error {
	return fn(ctx, td)
}

var _ admission.Handler = &ValidatingHandler{}

// Handle validate trait definition
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	startTime := time.Now()
	ctx = logging.WithRequestID(ctx, string(req.UID))
	logger := logging.NewHandlerLogger(ctx, req, "TraitDefinitionValidator")

	logger.WithStep("start").Info("Starting admission validation for TraitDefinition resource", "operation", req.Operation, "resourceVersion", req.Kind.Version)

	obj := &v1beta1.TraitDefinition{}
	if req.Resource.String() != traitDefGVR.String() {
		err := fmt.Errorf("expect resource to be %s", traitDefGVR)
		logger.WithStep("resource-check").WithError(err).Error(err, "Admission request targets unexpected resource type - rejecting request",
			"expected", traitDefGVR.String(),
			"actual", req.Resource.String(),
			"operation", req.Operation)
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s (requestUID=%s)", err.Error(), req.UID))
	}

	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		if err := h.Decoder.Decode(req, obj); err != nil {
			logger.WithStep("decode").WithError(err).Error(err, "Unable to decode admission request payload into TraitDefinition object - malformed request")
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s (requestUID=%s)", err.Error(), req.UID))
		}
		if obj.Spec.Version != "" {
			logger = logger.WithValues("version", obj.Spec.Version)
		}
		logger.WithStep("decode").Info("Successfully decoded TraitDefinition from admission request",
			"definitionName", obj.Name,
			"namespace", obj.Namespace,
			"hasReference", len(obj.Spec.Reference.Name) > 0,
			"hasSchematic", obj.Spec.Schematic != nil)

		for i, validator := range h.Validators {
			if err := validator.Validate(ctx, *obj); err != nil {
				logger.WithStep(fmt.Sprintf("validator-%d", i)).WithError(err).Error(err, "TraitDefinition custom validator failed - definition does not meet validation requirements", "validatorIndex", i)
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
		}

		// validate cueTemplate
		if obj.Spec.Schematic != nil && obj.Spec.Schematic.CUE != nil {
			logger.WithStep("validate-cue").Info("Validating CUE template syntax and semantics for TraitDefinition schematic")
			if err := webhookutils.ValidateCuexTemplate(ctx, obj.Spec.Schematic.CUE.Template); err != nil {
				logger.WithStep("validate-cue").WithError(err).Error(err, "CUE template contains syntax errors or invalid constructs - template compilation failed")
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
			if err := webhookutils.ValidateOutputResourcesExist(obj.Spec.Schematic.CUE.Template, h.Client.RESTMapper(), obj); err != nil {
				logger.WithStep("validate-output-resources").WithError(err).Error(err, "CUE template references output resources that don't exist in cluster - unknown resource types detected")
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
			logger.WithStep("validate-cue").WithSuccess(true).Info("CUE template validation completed successfully - template is syntactically correct and all output resources exist")
		}

		if obj.Spec.Version != "" {
			if err := webhookutils.ValidateSemanticVersion(obj.Spec.Version); err != nil {
				logger.WithStep("validate-version").WithError(err).Error(err, "TraitDefinition version does not follow semantic versioning format (x.y.z)", "version", obj.Spec.Version, "expectedFormat", "x.y.z")
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
		}

		revisionName := obj.GetAnnotations()[oam.AnnotationDefinitionRevisionName]
		if len(revisionName) != 0 {
			defRevName := fmt.Sprintf("%s-v%s", obj.Name, revisionName)
			if err := webhookutils.ValidateDefinitionRevision(ctx, h.Client, obj, client.ObjectKey{Namespace: obj.Namespace, Name: defRevName}); err != nil {
				logger.WithStep("validate-revision").WithError(err).Error(err, "TraitDefinition revision conflicts with existing revision or has invalid format", "revisionName", revisionName, "expectedRevisionName", fmt.Sprintf("%s-v%s", obj.Name, revisionName))
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
		}

		version := obj.Spec.Version
		if err := webhookutils.ValidateMultipleDefVersionsNotPresent(version, revisionName, obj.Kind); err != nil {
			logger.WithStep("validate-version-conflict").WithError(err).Error(err, "TraitDefinition has conflicting version specifications - cannot have both spec.version and revision annotation", "specVersion", version, "revisionName", revisionName)
			return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
		}
		logger.WithStep("complete").WithSuccess(true, startTime).Info("TraitDefinition admission validation completed successfully - resource is valid and will be admitted", "definitionName", obj.Name, "operation", req.Operation)
	} else {
		logger.WithStep("skip-validation").Info("Skipping TraitDefinition validation - operation does not require validation", "operation", req.Operation, "reason", "only CREATE and UPDATE operations are validated")
	}
	return admission.ValidationResponse(true, "")
}

// RegisterValidatingHandler will register TraitDefinition validation to webhook
func RegisterValidatingHandler(mgr manager.Manager, _ controller.Args) {
	server := mgr.GetWebhookServer()
	server.Register("/validating-core-oam-dev-v1beta1-traitdefinitions", &webhook.Admission{Handler: &ValidatingHandler{
		Client:  mgr.GetClient(),
		Decoder: admission.NewDecoder(mgr.GetScheme()),
		Validators: []TraitDefValidator{
			TraitDefValidatorFn(ValidateDefinitionReference),
			// add more validators here
		},
	}})
}

// ValidateDefinitionReference validates whether the trait definition is valid if
// its `.spec.reference` field is unset.
// It's valid if
// it has at least one output, and all outputs must have GVK
// or it has no output but has a patch
// or it has a patch and outputs, and all outputs must have GVK
// TODO(roywang) currently we only validate whether it contains CUE template.
// Further validation, e.g., output with GVK, valid patch, etc, remains to be done.
func ValidateDefinitionReference(_ context.Context, td v1beta1.TraitDefinition) error {
	if len(td.Spec.Reference.Name) > 0 {
		return nil
	}
	capability, err := appfile.ConvertTemplateJSON2Object(td.Name, td.Spec.Extension, td.Spec.Schematic)
	if err != nil {
		return errors.WithMessage(err, errValidateDefRef)
	}
	if capability.CueTemplate == "" {
		return errors.New(failInfoDefRefOmitted)

	}
	return nil
}
