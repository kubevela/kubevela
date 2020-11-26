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

package component

import (
	"context"
	"encoding/json"
	"fmt"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
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

// log is for logging in this package.
var validatelog = logf.Log.WithName("component validate webhook")

var _ admission.Handler = &ValidatingHandler{}

// Handle handles admission requests.
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1alpha2.Component{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		validatelog.Error(err, "decoder failed", "req operation", req.AdmissionRequest.Operation, "req",
			req.AdmissionRequest)
		return admission.Denied(err.Error())
	}

	switch req.AdmissionRequest.Operation { //nolint:exhaustive
	case admissionv1beta1.Create:
		if allErrs := ValidateComponentObject(obj); len(allErrs) > 0 {
			validatelog.Info("create failed", "name", obj.Name, "errMsg", allErrs.ToAggregate().Error())
			return admission.Denied(allErrs.ToAggregate().Error())
		}
	case admissionv1beta1.Update:
		if allErrs := ValidateComponentObject(obj); len(allErrs) > 0 {
			validatelog.Info("update failed", "name", obj.Name, "errMsg", allErrs.ToAggregate().Error())
			return admission.Denied(allErrs.ToAggregate().Error())
		}
	}

	return admission.Allowed("")
}

// ValidateComponentObject validates the Component on creation
func ValidateComponentObject(obj *v1alpha2.Component) field.ErrorList {
	validatelog.Info("validate component", "name", obj.Name)
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
