package applicationdeployment

import (
	"context"
	"net/http"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/webhook/common/rollout"
)

// ValidatingHandler handles ApplicationDeployment
type ValidatingHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &ValidatingHandler{}

// Handle handles admission requests.
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1alpha2.ApplicationDeployment{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		klog.Error(err, "decoder failed", "req operation", req.AdmissionRequest.Operation, "req",
			req.AdmissionRequest)
		return admission.Errored(http.StatusBadRequest, err)
	}

	switch req.AdmissionRequest.Operation {
	case admissionv1beta1.Create:
		if allErrs := ValidateCreate(obj); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	case admissionv1beta1.Update:
		oldObj := &v1alpha2.ApplicationDeployment{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if allErrs := ValidateUpdate(obj, oldObj); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	default:
		// Do nothing for DELETE and CONNECT
	}

	return admission.ValidationResponse(true, "")
}

// ValidateCreate validates the ApplicationDeployment on creation
func ValidateCreate(r *v1alpha2.ApplicationDeployment) field.ErrorList {
	klog.InfoS("validate create", "name", r.Name)
	allErrs := apimachineryvalidation.ValidateObjectMeta(&r.ObjectMeta, true,
		apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))

	fldPath := field.NewPath("spec")
	target := r.Spec.TargetApplicationName
	if len(target) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("targetApplicationName"),
			"source application cannot be empty"))
	}

	//TODO: Add ComponentList check
	// 1. there can only be one component or less
	// 2. if there are no components, make sure the applications has only one common component and that's the default
	// 3. it is contained in both source and target application

	allErrs = append(allErrs, rollout.ValidateCreate(&r.Spec.RolloutPlan)...)
	return allErrs
}

// ValidateUpdate validates the ApplicationDeployment on update
func ValidateUpdate(new *v1alpha2.ApplicationDeployment, prev *v1alpha2.ApplicationDeployment) field.ErrorList {
	klog.InfoS("validate update", "name", new.Name)
	errList := ValidateCreate(new)
	if len(errList) > 0 {
		return errList
	}
	return rollout.ValidateUpdate(&new.Spec.RolloutPlan, &prev.Spec.RolloutPlan)
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

// RegisterValidatingHandler will register application configuration validation to webhook
func RegisterValidatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register("/validating-core-oam-dev-v1alpha2-applicationdeployments",
		&webhook.Admission{Handler: &ValidatingHandler{}})
}
