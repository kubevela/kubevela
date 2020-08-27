/*
Copyright 2019 The Kruise Authors.

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

package metrics

import (
	"context"
	"fmt"
	"net/http"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/cloud-native-application/rudrx/api/v1alpha1"
)

// MetricsTraitValidatingHandler handles MetricsTrait
type MetricsTraitValidatingHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

// log is for logging in this package.
var validatelog = logf.Log.WithName("metricstrait-validate")

var _ admission.Handler = &MetricsTraitValidatingHandler{}

// Handle handles admission requests.
func (h *MetricsTraitValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1alpha1.MetricsTrait{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	switch req.AdmissionRequest.Operation {
	case admissionv1beta1.Create:
		if allErrs := ValidateCreate(obj); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	case admissionv1beta1.Update:
		oldObj := &v1alpha1.MetricsTrait{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if allErrs := ValidateUpdate(obj, oldObj); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	}

	return admission.ValidationResponse(true, "")
}

// ValidateCreate validates the metricsTrait on creation
func ValidateCreate(r *v1alpha1.MetricsTrait) field.ErrorList {
	validatelog.Info("validate create", "name", r.Name)
	/*allErrs := apivalidation.ValidateObjectMeta(&r.ObjectMeta, true, apimachineryvalidation.NameIsDNSSubdomain,
	field.NewPath("metadata"))*/
	var allErrs field.ErrorList
	fldPath := field.NewPath("spec")
	if r.Spec.ScrapeService.Format != SupportedFormat {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("ScrapeService.Format"), r.Spec.ScrapeService.Format,
			fmt.Sprintf("the data format `%s` is not supported", r.Spec.ScrapeService.Format)))
	}
	if r.Spec.ScrapeService.Scheme != SupportedScheme {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("ScrapeService.Format"), r.Spec.ScrapeService.Scheme,
			fmt.Sprintf("the scheme `%s` is not supported", r.Spec.ScrapeService.Scheme)))
	}
	return allErrs
}

// ValidateUpdate validates the metricsTrait on update
func ValidateUpdate(r *v1alpha1.MetricsTrait, _ *v1alpha1.MetricsTrait) field.ErrorList {
	validatelog.Info("validate update", "name", r.Name)
	return ValidateCreate(r)
}

// ValidateDelete validates the metricsTrait on delete
func ValidateDelete(r *v1alpha1.MetricsTrait) field.ErrorList {
	validatelog.Info("validate delete", "name", r.Name)
	return nil
}

var _ inject.Client = &MetricsTraitValidatingHandler{}

// InjectClient injects the client into the MetricsTraitValidatingHandler
func (h *MetricsTraitValidatingHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &MetricsTraitValidatingHandler{}

// InjectDecoder injects the decoder into the MetricsTraitValidatingHandler
func (h *MetricsTraitValidatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
