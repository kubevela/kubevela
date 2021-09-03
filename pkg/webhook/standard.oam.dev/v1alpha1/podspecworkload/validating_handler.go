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

	admissionv1 "k8s.io/api/admission/v1"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common"
)

// ValidatingHandler handles PodSpecWorkload
type ValidatingHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &ValidatingHandler{}

// Handle handles admission requests.
func (h *ValidatingHandler) Handle(_ctx context.Context, req admission.Request) admission.Response {
	ctx := common.NewReconcileContext(_ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace})
	ctx.BeginReconcile()
	defer ctx.EndReconcile()
	obj := &v1alpha1.PodSpecWorkload{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		ctx.Error(err, "Failed to decode", "req operation", req.AdmissionRequest.Operation, "req",
			req.AdmissionRequest)
		return admission.Errored(http.StatusBadRequest, err)
	}

	switch req.AdmissionRequest.Operation {
	case admissionv1.Create:
		if allErrs := ValidateCreate(ctx, obj); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	case admissionv1.Update:
		oldObj := &v1alpha1.PodSpecWorkload{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if allErrs := ValidateUpdate(ctx, obj, oldObj); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	default:
		// Do nothing for DELETE and CONNECT
	}

	return admission.ValidationResponse(true, "")
}

// ValidateCreate validates the PodSpecWorkload on creation
func ValidateCreate(ctx *common.ReconcileContext, r *v1alpha1.PodSpecWorkload) field.ErrorList {
	ctx.Info("Validate create podSpecWorkload")
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
func ValidateUpdate(ctx *common.ReconcileContext, r *v1alpha1.PodSpecWorkload, _ *v1alpha1.PodSpecWorkload) field.ErrorList {
	ctx.Info("Validate update podSpecWorkload")
	return ValidateCreate(ctx, r)
}

// ValidateDelete validates the PodSpecWorkload on delete
func ValidateDelete(ctx *common.ReconcileContext, r *v1alpha1.PodSpecWorkload) field.ErrorList {
	ctx.Info("Validate delete PodSpecWorkload")
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
