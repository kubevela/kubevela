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

	logger.LogStep("start")

	obj := &v1beta1.PolicyDefinition{}
	if req.Resource.String() != policyDefGVR.String() {
		err := fmt.Errorf("expect resource to be %s", policyDefGVR)
		logger.LogError(err, "resource-mismatch")
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s (requestUID=%s)", err.Error(), req.UID))
	}

	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		if err := h.Decoder.Decode(req, obj); err != nil {
			logger.LogError(err, "decode")
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s (requestUID=%s)", err.Error(), req.UID))
		}
		if obj.Spec.Version != "" {
			logger = logger.WithValues("version", obj.Spec.Version)
		}
		logger.LogStep("decoded")

		if obj.Spec.Schematic != nil && obj.Spec.Schematic.CUE != nil {
			if err := webhookutils.ValidateCueTemplate(obj.Spec.Schematic.CUE.Template); err != nil {
				logger.LogError(err, "validate-cue")
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
			if err := webhookutils.ValidateOutputResourcesExist(obj.Spec.Schematic.CUE.Template, h.Client.RESTMapper()); err != nil {
				logger.LogError(err, "validate-output-resources")
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
		}

		if obj.Spec.Version != "" {
			if err := webhookutils.ValidateSemanticVersion(obj.Spec.Version); err != nil {
				logger.LogError(err, "validate-version")
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
		}

		revisionName := obj.GetAnnotations()[oam.AnnotationDefinitionRevisionName]
		if len(revisionName) != 0 {
			defRevName := fmt.Sprintf("%s-v%s", obj.Name, revisionName)
			if err := webhookutils.ValidateDefinitionRevision(ctx, h.Client, obj, client.ObjectKey{Namespace: obj.Namespace, Name: defRevName}); err != nil {
				logger.LogError(err, "validate-revision")
				return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
			}
		}

		if err := webhookutils.ValidateMultipleDefVersionsNotPresent(obj.Spec.Version, revisionName, obj.Kind); err != nil {
			logger.LogError(err, "validate-version-conflict")
			return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
		}
		logger.LogSuccess("policy-definition-validation", startTime)
	} else {
		logger.LogStep("skip-validation", "reason", "unsupported-operation")
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
