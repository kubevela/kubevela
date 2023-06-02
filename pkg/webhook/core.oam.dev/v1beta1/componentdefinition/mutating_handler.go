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
	"encoding/json"
	"net/http"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// MutatingHandler handles ComponentDefinition
type MutatingHandler struct {
	Client client.Client
	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &MutatingHandler{}

// Handle handles admission requests.
func (h *MutatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1beta1.ComponentDefinition{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	// mutate the object
	if err := h.Mutate(obj); err != nil {
		klog.ErrorS(err, "failed to mutate the componentDefinition", "name", obj.Name)
		return admission.Errored(http.StatusBadRequest, err)
	}

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
	if len(resp.Patches) > 0 {
		klog.InfoS("admit ComponentDefinition",
			"namespace", obj.Namespace, "name", obj.Name, "patches", util.JSONMarshal(resp.Patches))
	}
	return resp
}

// Mutate sets all the default value for the ComponentDefinition
func (h *MutatingHandler) Mutate(obj *v1beta1.ComponentDefinition) error {
	klog.InfoS("mutate", "name", obj.Name)
	if obj.Spec.Workload.Type == "" {
		obj.Spec.Workload.Type = types.AutoDetectWorkloadDefinition
	}
	return nil
}

var _ admission.DecoderInjector = &MutatingHandler{}

// InjectDecoder injects the decoder into the ComponentDefinitionMutatingHandler
func (h *MutatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

var _ inject.Client = &MutatingHandler{}

// InjectClient injects the client into the ApplicationValidateHandler
func (h *MutatingHandler) InjectClient(c client.Client) error {
	if h.Client != nil {
		return nil
	}
	h.Client = c
	return nil
}

// RegisterMutatingHandler will register component mutation handler to the webhook
func RegisterMutatingHandler(mgr manager.Manager, args controller.Args) {
	server := mgr.GetWebhookServer()
	server.Register("/mutating-core-oam-dev-v1beta1-componentdefinitions", &webhook.Admission{
		Handler: &MutatingHandler{},
	})
}
