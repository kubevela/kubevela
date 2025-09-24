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

	logger.Info("Starting ComponentDefinition validation", logging.FieldStep, "start")

	obj := &v1beta1.ComponentDefinition{}
	if req.Resource.String() != componentDefGVR.String() {
		err := fmt.Errorf("expect resource to be %s", componentDefGVR)
		logger.Error(err, "Resource GVR mismatch",
			logging.FieldStep, "resource-check",
			"expected", componentDefGVR.String(),
			"actual", req.Resource.String())
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s (requestUID=%s)", err.Error(), req.UID))
	}

	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		if err := h.Decoder.Decode(req, obj); err != nil {
			logger.Error(err, "Failed to decode ComponentDefinition",
				logging.FieldStep, "decode",
				logging.FieldSuccess, false)
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s (requestUID=%s)", err.Error(), req.UID))
		}

		// Add definition-specific fields to logger
		if obj.Spec.Version != "" {
			logger = logger.WithValues("version", obj.Spec.Version)
		}
		logger.Info("Successfully decoded ComponentDefinition",
			logging.FieldStep, "decode",
			"definitionName", obj.Name)

		// Validate workload
		if err := ValidateWorkload(h.Client.RESTMapper(), obj); err != nil {
			logger.Error(err, "Workload validation failed",
				logging.FieldStep, "validate-workload",
				logging.FieldSuccess, false)
			return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
		}
		logger.Info("Workload validation passed", logging.FieldStep, "validate-workload")

		// Validate CUE template
		if obj.Spec.Schematic != nil && obj.Spec.Schematic.CUE != nil {
			logger.Info("Validating CUE template", logging.FieldStep, "validate-cue")

			if err := webhookutils.ValidateCuexTemplate(ctx, obj.Spec.Schematic.CUE.Template); err != nil {
				logger.Error(err, "CUE template validation failed",
					logging.FieldStep, "validate-cue",
					logging.FieldSuccess, false)
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}

			if err := webhookutils.ValidateOutputResourcesExist(obj.Spec.Schematic.CUE.Template, h.Client.RESTMapper()); err != nil {
				logger.Error(err, "Output resources validation failed",
					logging.FieldStep, "validate-output-resources",
					logging.FieldSuccess, false)
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
			logger.Info("CUE validation passed", logging.FieldStep, "validate-cue", logging.FieldSuccess, true)
		}

		// Validate semantic version
		if obj.Spec.Version != "" {
			if err := webhookutils.ValidateSemanticVersion(obj.Spec.Version); err != nil {
				logger.Error(err, "Semantic version validation failed",
					logging.FieldStep, "validate-version",
					logging.FieldSuccess, false,
					"version", obj.Spec.Version)
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
			logger.Info("Version validation passed", logging.FieldStep, "validate-version")
		}

		// Validate revision
		revisionName := obj.GetAnnotations()[oam.AnnotationDefinitionRevisionName]
		if len(revisionName) != 0 {
			defRevName := fmt.Sprintf("%s-v%s", obj.Name, revisionName)
			if err := webhookutils.ValidateDefinitionRevision(ctx, h.Client, obj, client.ObjectKey{Namespace: obj.Namespace, Name: defRevName}); err != nil {
				logger.Error(err, "Definition revision validation failed",
					logging.FieldStep, "validate-revision",
					logging.FieldSuccess, false,
					"revisionName", revisionName)
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
			logger.Info("Revision validation passed", logging.FieldStep, "validate-revision")
		}

		// Check version conflicts
		if err := webhookutils.ValidateMultipleDefVersionsNotPresent(obj.Spec.Version, revisionName, obj.Kind); err != nil {
			logger.Error(err, "Multiple definition versions conflict",
				logging.FieldStep, "validate-version-conflict",
				logging.FieldSuccess, false)
			return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
		}

		// Log successful completion
		duration := time.Since(startTime)
		logger.Info("ComponentDefinition validation completed successfully",
			logging.FieldStep, "complete",
			logging.FieldSuccess, true,
			logging.FieldDuration, duration.Milliseconds())
	} else {
		logger.Info("Skipping validation for operation",
			logging.FieldStep, "skip-validation",
			"reason", "unsupported-operation")
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
