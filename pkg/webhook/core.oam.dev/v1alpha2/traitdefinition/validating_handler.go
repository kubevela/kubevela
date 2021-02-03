package traitdefinition

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

const (
	errValidateDefRef = "error occurs when validating definition reference"

	failInfoDefRefOmitted = "if definition reference is omitted, patch or output with GVK is required"
)

var traitDefGVR = v1alpha2.SchemeGroupVersion.WithResource("traitdefinitions")

// ValidatingHandler handles validation of trait definition
type ValidatingHandler struct {
	Client client.Client
	Mapper discoverymapper.DiscoveryMapper

	// Decoder decodes object
	Decoder *admission.Decoder
	// Validators validate objects
	Validators []TraitDefValidator
}

// TraitDefValidator validate trait definition
type TraitDefValidator interface {
	Validate(context.Context, v1alpha2.TraitDefinition) error
}

// TraitDefValidatorFn implements TraitDefValidator
type TraitDefValidatorFn func(context.Context, v1alpha2.TraitDefinition) error

// Validate implements TraitDefValidator method
func (fn TraitDefValidatorFn) Validate(ctx context.Context, td v1alpha2.TraitDefinition) error {
	return fn(ctx, td)
}

var _ admission.Handler = &ValidatingHandler{}

// Handle validate trait definition
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1alpha2.TraitDefinition{}
	if req.Resource.String() != traitDefGVR.String() {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("expect resource to be %s", traitDefGVR))
	}

	if req.Operation == admissionv1beta1.Create || req.Operation == admissionv1beta1.Update {
		err := h.Decoder.Decode(req, obj)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		klog.Info("validating ", " name: ", obj.Name, " operation: ", string(req.Operation))
		for _, validator := range h.Validators {
			if err := validator.Validate(ctx, *obj); err != nil {
				klog.Info("validation failed ", " name: ", obj.Name, " errMsgi: ", err.Error())
				return admission.Denied(err.Error())
			}
		}
		klog.Info("validation passed ", " name: ", obj.Name, " operation: ", string(req.Operation))
	}
	return admission.ValidationResponse(true, "")
}

var _ inject.Client = &ValidatingHandler{}

// InjectClient injects the client into the ValidatingHandler
func (h *ValidatingHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &ValidatingHandler{}

// InjectDecoder injects the decoder into the ValidatingHandler
func (h *ValidatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

// RegisterValidatingHandler will register TraitDefinition validation to webhook
func RegisterValidatingHandler(mgr manager.Manager) error {
	server := mgr.GetWebhookServer()
	mapper, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		return err
	}
	server.Register("/validating-core-oam-dev-v1alpha2-traitdefinitions", &webhook.Admission{Handler: &ValidatingHandler{
		Mapper: mapper,
		Validators: []TraitDefValidator{
			TraitDefValidatorFn(ValidateDefinitionReference),
			// add more validators here
		},
	}})
	return nil
}

// ValidateDefinitionReference validates whether the trait definition is valid if
// its `.spec.reference` field is unset.
// It's valid if
// it has at least one output, and all outputs must have GVK
// or it has no output but has a patch
// or it has a patch and outputs, and all outputs must have GVK
// TODO(roywang) currently we only validate whether it contains CUE template.
// Further validation, e.g., output with GVK, valid patch, etc, remains to be done.
func ValidateDefinitionReference(_ context.Context, td v1alpha2.TraitDefinition) error {
	if len(td.Spec.Reference.Name) > 0 {
		return nil
	}

	if td.Spec.Extension == nil || len(td.Spec.Extension.Raw) < 1 {
		return errors.New(failInfoDefRefOmitted)
	}

	tmp := map[string]interface{}{}
	if err := json.Unmarshal(td.Spec.Extension.Raw, &tmp); err != nil {
		return errors.Wrap(err, errValidateDefRef)
	}
	template, ok := tmp["template"]
	if !ok || len(fmt.Sprint(template)) < 1 {
		return errors.New(failInfoDefRefOmitted)
	}
	return nil
}
