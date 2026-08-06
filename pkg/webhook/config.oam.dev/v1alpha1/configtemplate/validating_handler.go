/*
Copyright 2026 The KubeVela Authors.

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

package configtemplate

import (
	"context"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/cue/script"
)

var configTemplateGVR = configv1alpha1.ConfigTemplateGVR

// ValidatingHandler validates ConfigTemplate resources.
type ValidatingHandler struct {
	Decoder admission.Decoder
}

var _ admission.Handler = &ValidatingHandler{}

// Handle validates the ConfigTemplate's CUE template syntax.
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Resource.String() != configTemplateGVR.String() {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("expect resource to be %s", configTemplateGVR))
	}
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return admission.ValidationResponse(true, "")
	}

	obj := &configv1alpha1.ConfigTemplate{}
	if err := h.Decoder.Decode(req, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if _, err := script.CUE(obj.Spec.Template).ParseToTemplateValueWithCueX(ctx); err != nil {
		return admission.Denied(fmt.Sprintf("invalid template: %s (requestUID=%s)", err.Error(), req.UID))
	}

	return admission.ValidationResponse(true, "")
}

// RegisterValidatingHandler registers ConfigTemplate validation to the webhook server.
func RegisterValidatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register("/validating-config-oam-dev-v1alpha1-configtemplates", &webhook.Admission{Handler: &ValidatingHandler{
		Decoder: admission.NewDecoder(mgr.GetScheme()),
	}})
}
