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

package component

import (
	"context"
	"encoding/json"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

// ValidatingHandler handles Component
type ValidatingHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &ValidatingHandler{}

// Handle handles admission requests.
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1alpha2.Component{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		klog.InfoS("Failed to decode component", "req operation", req.AdmissionRequest.Operation, "req",
			req.AdmissionRequest, "err", err)
		return admission.Denied(err.Error())
	}

	switch req.AdmissionRequest.Operation { //nolint:exhaustive
	case admissionv1.Create:
		if allErrs := ValidateComponentObject(obj); len(allErrs) > 0 {
			klog.InfoS("Failed to create component", "component", klog.KObj(obj), "err", allErrs.ToAggregate().Error())
			return admission.Denied(allErrs.ToAggregate().Error())
		}
	case admissionv1.Update:
		if allErrs := ValidateComponentObject(obj); len(allErrs) > 0 {
			klog.InfoS("Failed to update component", "component", klog.KObj(obj), "err", allErrs.ToAggregate().Error())
			return admission.Denied(allErrs.ToAggregate().Error())
		}
	}

	return admission.Allowed("")
}

// ValidateComponentObject validates the Component on creation
func ValidateComponentObject(obj *v1alpha2.Component) field.ErrorList {
	klog.InfoS("Validate component", "component", klog.KObj(obj))
	allErrs := apimachineryvalidation.ValidateObjectMeta(&obj.ObjectMeta, true,
		apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	fldPath := field.NewPath("spec")
	var content map[string]interface{}
	if err := json.Unmarshal(obj.Spec.Workload.Raw, &content); err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("workload"), string(obj.Spec.Workload.Raw),
			"the workload is malformat"))
		return allErrs
	}
	if content[TypeField] != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("workload"), string(obj.Spec.Workload.Raw),
			"the workload contains type info"))
	}
	workload := unstructured.Unstructured{
		Object: content,
	}
	if len(workload.GetAPIVersion()) == 0 || len(workload.GetKind()) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("workload"), content,
			fmt.Sprintf("the workload data missing GVK, api = %s, kind = %s,", workload.GetAPIVersion(), workload.GetKind())))
	}
	return allErrs
}

/*
var _ inject.Client = &ValidatingHandler{}

// InjectClient injects the client into the ComponentValidatingHandler

	func (h *ValidatingHandler) InjectClient(c client.Client) error {
		h.Client = c
		return nil
	}
*/
var _ admission.DecoderInjector = &ValidatingHandler{}

// InjectDecoder injects the decoder into the ComponentValidatingHandler
func (h *ValidatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

// RegisterValidatingHandler will regsiter component mutation handler to the webhook
func RegisterValidatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register("/validating-core-oam-dev-v1alpha2-components", &webhook.Admission{Handler: &ValidatingHandler{}})
}
