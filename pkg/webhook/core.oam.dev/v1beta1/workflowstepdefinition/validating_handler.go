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

package workflowstepdefinition

import (
	"context"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	webhookutils "github.com/oam-dev/kubevela/pkg/webhook/utils"
)

var workflowStepDefGVR = v1beta1.SchemeGroupVersion.WithResource("workflowstepdefinitions")

// ValidatingHandler handles validation of workflow step definition
type ValidatingHandler struct {
	// Decoder decodes object
	Decoder admission.Decoder
	Client  client.Client
}

// InjectClient injects the client into the ValidatingHandler
func (h *ValidatingHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

// InjectDecoder injects the decoder into the ValidatingHandler
func (h *ValidatingHandler) InjectDecoder(d admission.Decoder) error {
	h.Decoder = d
	return nil
}

// Handle validate WorkflowStepDefinition Spec here
func (h *ValidatingHandler) Handle(_ context.Context, req admission.Request) admission.Response {
	obj := &v1beta1.WorkflowStepDefinition{}
	if req.Resource.String() != workflowStepDefGVR.String() {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("expect resource to be %s", workflowStepDefGVR))
	}

	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		err := h.Decoder.Decode(req, obj)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if obj.Spec.Version != "" {
			err = webhookutils.ValidateSemanticVersion(obj.Spec.Version)
			if err != nil {
				return admission.Denied(err.Error())
			}
		}
		revisionName := obj.Annotations[oam.AnnotationDefinitionRevisionName]
		version := obj.Spec.Version
		err = webhookutils.ValidateMultipleDefVersionsNotPresent(version, revisionName, obj.Kind)
		if err != nil {
			return admission.Denied(err.Error())
		}
	}
	return admission.ValidationResponse(true, "")
}

// RegisterValidatingHandler will register WorkflowStepDefinition validation to webhook
func RegisterValidatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register("/validating-core-oam-dev-v1beta1-workflowstepdefinitions", &webhook.Admission{Handler: &ValidatingHandler{}})
}
