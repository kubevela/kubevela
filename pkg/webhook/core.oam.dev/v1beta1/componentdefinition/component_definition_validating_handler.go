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

package componentdefinition

import (
	"context"
	"fmt"
	"net/http"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/logging"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	webhookutils "github.com/oam-dev/kubevela/pkg/webhook/utils"
)

var componentDefGVR = v1beta1.ComponentDefinitionGVR

// ValidatingHandler handles validation of component definition
type ValidatingHandler struct {
	// Decoder decodes object
	Decoder admission.Decoder
	Client  client.Client
}

var _ admission.Handler = &ValidatingHandler{}

// Handle validate ComponentDefinition Spec here
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	startTime := time.Now()
	ctx = logging.WithRequestID(ctx, string(req.UID))
	logger := logging.NewHandlerLogger(ctx, req, "ComponentDefinitionValidator")

	// Using the logger methods directly will show the correct file location
	logger.WithStep("start").Info("Starting admission validation for ComponentDefinition resource", "operation", req.Operation, "resourceVersion", req.Kind.Version)

	obj := &v1beta1.ComponentDefinition{}
	if req.Resource.String() != componentDefGVR.String() {
		err := fmt.Errorf("expect resource to be %s", componentDefGVR)
		logger.WithStep("resource-check").WithError(err).Error(err, "Admission request targets unexpected resource type - rejecting request",
			"expected", componentDefGVR.String(),
			"actual", req.Resource.String(),
			"operation", req.Operation)
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s (requestUID=%s)", err.Error(), req.UID))
	}

	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		if err := h.Decoder.Decode(req, obj); err != nil {
			logger.WithStep("decode").WithError(err).Error(err, "Unable to decode admission request payload into ComponentDefinition object - malformed request")
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s (requestUID=%s)", err.Error(), req.UID))
		}

		// Add definition-specific fields to logger
		if obj.Spec.Version != "" {
			logger = logger.WithValues("version", obj.Spec.Version)
		}
		logger.WithStep("decode").Info("Successfully decoded ComponentDefinition from admission request",
			"definitionName", obj.Name,
			"namespace", obj.Namespace,
			"workloadType", obj.Spec.Workload.Type,
			"hasSchematic", obj.Spec.Schematic != nil)

		// Validate workload
		if err := ValidateWorkload(h.Client.RESTMapper(), obj); err != nil {
			logger.WithStep("validate-workload").WithError(err).Error(err, "ComponentDefinition workload configuration is invalid - type and definition must be consistent")
			return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
		}
		logger.WithStep("validate-workload").Info("ComponentDefinition workload configuration validated successfully", "workloadType", obj.Spec.Workload.Type)

		// Validate CUE template
		if obj.Spec.Schematic != nil && obj.Spec.Schematic.CUE != nil {
			logger.WithStep("validate-cue").Info("Validating CUE template syntax and semantics for ComponentDefinition schematic")

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

		// Validate semantic version
		if obj.Spec.Version != "" {
			if err := webhookutils.ValidateSemanticVersion(obj.Spec.Version); err != nil {
				logger.WithStep("validate-version").WithError(err).Error(err, "ComponentDefinition version does not follow semantic versioning format (x.y.z)", "version", obj.Spec.Version, "expectedFormat", "x.y.z")
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
			logger.WithStep("validate-version").Info("ComponentDefinition version follows semantic versioning format", "version", obj.Spec.Version)
		}

		// Validate revision
		revisionName := obj.GetAnnotations()[oam.AnnotationDefinitionRevisionName]
		if len(revisionName) != 0 {
			defRevName := fmt.Sprintf("%s-v%s", obj.Name, revisionName)
			if err := webhookutils.ValidateDefinitionRevision(ctx, h.Client, obj, client.ObjectKey{Namespace: obj.Namespace, Name: defRevName}); err != nil {
				logger.WithStep("validate-revision").WithError(err).Error(err, "ComponentDefinition revision conflicts with existing revision or has invalid format", "revisionName", revisionName, "expectedRevisionName", fmt.Sprintf("%s-v%s", obj.Name, revisionName))
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
			logger.WithStep("validate-revision").Info("ComponentDefinition revision validation completed - no conflicts detected", "revisionName", revisionName)
		}

		// Check version conflicts
		if err := webhookutils.ValidateMultipleDefVersionsNotPresent(obj.Spec.Version, revisionName, obj.Kind); err != nil {
			logger.WithStep("validate-version-conflict").WithError(err).Error(err, "ComponentDefinition has conflicting version specifications - cannot have both spec.version and revision annotation", "specVersion", obj.Spec.Version, "revisionName", revisionName)
			return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
		}

		// Log successful completion
		logger.WithStep("complete").WithSuccess(true, startTime).Info("ComponentDefinition admission validation completed successfully - resource is valid and will be admitted", "definitionName", obj.Name, "operation", req.Operation)
	} else {
		logger.WithStep("skip-validation").Info("Skipping ComponentDefinition validation - operation does not require validation", "operation", req.Operation, "reason", "only CREATE and UPDATE operations are validated")
	}
	return admission.ValidationResponse(true, "")
}

// RegisterValidatingHandler will register ComponentDefinition validation to webhook
func RegisterValidatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register("/validating-core-oam-dev-v1beta1-componentdefinitions", &webhook.Admission{Handler: &ValidatingHandler{
		Client:  mgr.GetClient(),
		Decoder: admission.NewDecoder(mgr.GetScheme()),
	}})
}

// ValidateWorkload validates whether the Workload field is valid
func ValidateWorkload(mapper meta.RESTMapper, cd *v1beta1.ComponentDefinition) error {

	// If the Type and Definition are all empty, it will be rejected.
	if cd.Spec.Workload.Type == "" && cd.Spec.Workload.Definition == (common.WorkloadGVK{}) {
		return fmt.Errorf("neither the type nor the definition of the workload field in the ComponentDefinition %s can be empty", cd.Name)
	}

	// if Type and Definitiondon‘t point to the same workloaddefinition, it will be rejected.
	if cd.Spec.Workload.Type != "" && cd.Spec.Workload.Definition != (common.WorkloadGVK{}) {
		defRef, err := util.ConvertWorkloadGVK2Definition(mapper, cd.Spec.Workload.Definition)
		if err != nil {
			return err
		}
		if defRef.Name != cd.Spec.Workload.Type {
			return fmt.Errorf("the type and the definition of the workload field in ComponentDefinition %s should represent the same workload", cd.Name)
		}
	}
	return nil
}
