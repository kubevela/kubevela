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
	"net/http"

	"github.com/oam-dev/kubevela/pkg/dsl/definition"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ admission.Handler = &ValidatingHandler{}

// ValidatingHandler handles application
type ValidatingHandler struct {
	dm     discoverymapper.DiscoveryMapper
	pd     *definition.PackageDiscover
	Client client.Client
	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ inject.Client = &ValidatingHandler{}

// InjectClient injects the client into the ApplicationValidateHandler
func (h *ValidatingHandler) InjectClient(c client.Client) error {
	if h.Client != nil {
		return nil
	}
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &ValidatingHandler{}

// InjectDecoder injects the decoder into the ApplicationValidateHandler
func (h *ValidatingHandler) InjectDecoder(d *admission.Decoder) error {
	if h.Decoder != nil {
		return nil
	}
	h.Decoder = d
	return nil
}

// Handle validate Application Spec here
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	app := &v1beta1.Application{}
	if err := h.Decoder.Decode(req, app); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	ctx = util.SetNamespaceInCtx(ctx, app.Namespace)
	switch req.Operation {
	case admissionv1beta1.Create:
		if allErrs := h.ValidateCreate(ctx, app); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	case admissionv1beta1.Update:
		oldApp := &v1beta1.Application{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldApp); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if allErrs := h.ValidateUpdate(ctx, app, oldApp); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	default:
		// Do nothing for DELETE and CONNECT
	}
	return admission.ValidationResponse(true, "")
}

// RegisterValidatingHandler will register application validate handler to the webhook
func RegisterValidatingHandler(mgr manager.Manager, args controller.Args) {
	server := mgr.GetWebhookServer()
	server.Register("/validating-core-oam-dev-v1alpha2-applications", &webhook.Admission{Handler: &ValidatingHandler{dm: args.DiscoveryMapper, pd: args.PackageDiscover}})
}
