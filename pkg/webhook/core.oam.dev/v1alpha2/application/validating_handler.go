package application

import (
	"context"
	"net/http"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

var _ admission.Handler = &ValidatingHandler{}

// ValidatingHandler handles application
type ValidatingHandler struct {
	dm     discoverymapper.DiscoveryMapper
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
	app := &v1alpha2.Application{}
	if err := h.Decoder.Decode(req, app); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if req.Operation == admissionv1beta1.Delete || app.DeletionTimestamp != nil {
		return admission.ValidationResponse(true, "")
	}

	// try render to validate
	appParser := application.NewApplicationParser(h.Client, h.dm)
	if _, err := appParser.GenerateAppFile(app.Name, app); err != nil {
		return admission.Denied(err.Error())
	}
	return admission.ValidationResponse(true, "")
}

// RegisterValidatingHandler will regsiter application validate handler to the webhook
func RegisterValidatingHandler(mgr manager.Manager) error {
	mapper, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		return err
	}
	server := mgr.GetWebhookServer()
	server.Register("/validating-core-oam-dev-v1alpha2-applications", &webhook.Admission{Handler: &ValidatingHandler{dm: mapper}})
	return nil
}
