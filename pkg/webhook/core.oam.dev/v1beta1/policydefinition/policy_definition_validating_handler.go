/*
 Copyright 2021. The KubeVela Authors.

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

package policydefinition

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

var policyDefGVR = v1beta1.PolicyDefinitionGVR

// ValidatingHandler handles validation of policy definition
type ValidatingHandler struct {
	// Decoder decodes object
	Decoder admission.Decoder
	Client  client.Client
}

var _ admission.Handler = &ValidatingHandler{}

// Handle validate component definition
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	startTime := time.Now()
	ctx = logging.WithRequestID(ctx, string(req.UID))
	logger := logging.NewHandlerLogger(ctx, req, "PolicyDefinitionValidator")

	logger.WithStep("start").Info("Starting admission validation for PolicyDefinition resource", "operation", req.Operation, "resourceVersion", req.Kind.Version)

	obj := &v1beta1.PolicyDefinition{}
	if req.Resource.String() != policyDefGVR.String() {
		err := fmt.Errorf("expect resource to be %s", policyDefGVR)
		logger.WithStep("resource-check").WithError(err).Error(err, "Admission request targets unexpected resource type - rejecting request",
			"expected", policyDefGVR.String(),
			"actual", req.Resource.String(),
			"operation", req.Operation)
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s (requestUID=%s)", err.Error(), req.UID))
	}

	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		if err := h.Decoder.Decode(req, obj); err != nil {
			logger.WithStep("decode").WithError(err).Error(err, "Unable to decode admission request payload into PolicyDefinition object - malformed request")
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s (requestUID=%s)", err.Error(), req.UID))
		}
		if obj.Spec.Version != "" {
			logger = logger.WithValues("version", obj.Spec.Version)
		}
		logger.WithStep("decode").Info("Successfully decoded PolicyDefinition from admission request",
			"definitionName", obj.Name,
			"namespace", obj.Namespace,
			"hasSchematic", obj.Spec.Schematic != nil,
			"version", obj.Spec.Version)

		if obj.Spec.Schematic != nil && obj.Spec.Schematic.CUE != nil {
			logger.WithStep("validate-cue").Info("Validating CUE template syntax and semantics for PolicyDefinition schematic")
			if err := webhookutils.ValidateCueTemplate(obj.Spec.Schematic.CUE.Template); err != nil {
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
				logger.WithStep("validate-version").WithError(err).Error(err, "PolicyDefinition version does not follow semantic versioning format (x.y.z)", "version", obj.Spec.Version, "expectedFormat", "x.y.z")
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
			logger.WithStep("validate-version").Info("PolicyDefinition version follows semantic versioning format", "version", obj.Spec.Version)
		}

		revisionName := obj.GetAnnotations()[oam.AnnotationDefinitionRevisionName]
		if len(revisionName) != 0 {
			defRevName := fmt.Sprintf("%s-v%s", obj.Name, revisionName)
			if err := webhookutils.ValidateDefinitionRevision(ctx, h.Client, obj, client.ObjectKey{Namespace: obj.Namespace, Name: defRevName}); err != nil {
				logger.WithStep("validate-revision").WithError(err).Error(err, "PolicyDefinition revision conflicts with existing revision or has invalid format", "revisionName", revisionName, "expectedRevisionName", fmt.Sprintf("%s-v%s", obj.Name, revisionName))
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
			logger.WithStep("validate-revision").Info("PolicyDefinition revision validation completed - no conflicts detected", "revisionName", revisionName)
		}

		if err := webhookutils.ValidateMultipleDefVersionsNotPresent(obj.Spec.Version, revisionName, obj.Kind); err != nil {
			logger.WithStep("validate-version-conflict").WithError(err).Error(err, "PolicyDefinition has conflicting version specifications - cannot have both spec.version and revision annotation", "specVersion", obj.Spec.Version, "revisionName", revisionName)
			return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
		}
		logger.WithStep("complete").WithSuccess(true, startTime).Info("PolicyDefinition admission validation completed successfully - resource is valid and will be admitted", "definitionName", obj.Name, "operation", req.Operation)
	} else {
		logger.WithStep("skip-validation").Info("Skipping PolicyDefinition validation - operation does not require validation", "operation", req.Operation, "reason", "only CREATE and UPDATE operations are validated")
	}
	return admission.ValidationResponse(true, "")
}

// RegisterValidatingHandler will register ComponentDefinition validation to webhook
func RegisterValidatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register("/validating-core-oam-dev-v1beta1-policydefinitions", &webhook.Admission{Handler: &ValidatingHandler{
		Client:  mgr.GetClient(),
		Decoder: admission.NewDecoder(mgr.GetScheme()),
	}})
}
