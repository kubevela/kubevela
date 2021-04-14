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
	"net/http"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// ValidatingHandler handles PodSpecWorkload
type ValidatingHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

// log is for logging in this package.
var validatelog = logf.Log.WithName("PodSpecWorkload-validate")

var _ admission.Handler = &ValidatingHandler{}

// Handle handles admission requests.
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1alpha1.PodSpecWorkload{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		validatelog.Error(err, "decoder failed", "req operation", req.AdmissionRequest.Operation, "req",
			req.AdmissionRequest)
		return admission.Errored(http.StatusBadRequest, err)
	}

	switch req.AdmissionRequest.Operation {
	case admissionv1beta1.Create:
		if allErrs := ValidateCreate(obj); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	case admissionv1beta1.Update:
		oldObj := &v1alpha1.PodSpecWorkload{}
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

// ValidateCreate validates the PodSpecWorkload on creation
func ValidateCreate(r *v1alpha1.PodSpecWorkload) field.ErrorList {
	validatelog.Info("validate create", "name", r.Name)
	allErrs := apimachineryvalidation.ValidateObjectMeta(&r.ObjectMeta, true,
		apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))

	fldPath := field.NewPath("spec")
	allErrs = append(allErrs, apimachineryvalidation.ValidateNonnegativeField(int64(*r.Spec.Replicas),
		fldPath.Child("Replicas"))...)

	fldPath = fldPath.Child("podSpec")
	spec := r.Spec.PodSpec
	if len(spec.Containers) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("Containers"), spec.Containers,
			"You need at least one container"))
	}
	return allErrs
}

// ValidateUpdate validates the PodSpecWorkload on update
func ValidateUpdate(r *v1alpha1.PodSpecWorkload, _ *v1alpha1.PodSpecWorkload) field.ErrorList {
	validatelog.Info("validate update", "name", r.Name)
	return ValidateCreate(r)
}

// ValidateDelete validates the PodSpecWorkload on delete
func ValidateDelete(r *v1alpha1.PodSpecWorkload) field.ErrorList {
	validatelog.Info("validate delete", "name", r.Name)
	return nil
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
