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

package podspecworkload

import (
	"context"
	"encoding/json"
	"net/http"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common"
	util "github.com/oam-dev/kubevela/pkg/utils"
)

// MutatingHandler handles PodSpec workload
type MutatingHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &MutatingHandler{}

// Handle handles admission requests.
func (h *MutatingHandler) Handle(_ctx context.Context, req admission.Request) admission.Response {
	ctx := common.NewReconcileContext(_ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace})
	ctx.BeginReconcile()
	defer ctx.EndReconcile()
	obj := &v1alpha1.PodSpecWorkload{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	DefaultPodSpecWorkload(ctx, obj)

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
	if len(resp.Patches) > 0 {
		ctx.Info("Admit PodSpecWorkload", "patches", util.DumpJSON(resp.Patches))
	}
	return resp
}

// DefaultPodSpecWorkload will set the default value for the PodSpecWorkload
func DefaultPodSpecWorkload(ctx *common.ReconcileContext, obj *v1alpha1.PodSpecWorkload) {
	ctx.Info("Set the default value for the PodSpecWorkload")
	if obj.Spec.Replicas == nil {
		ctx.Info("Set default replicas as 1")
		obj.Spec.Replicas = pointer.Int32Ptr(1)
	}
}

var _ inject.Client = &MutatingHandler{}

// InjectClient injects the client into the MutatingHandler
func (h *MutatingHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &MutatingHandler{}

// InjectDecoder injects the decoder into the MutatingHandler
func (h *MutatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
