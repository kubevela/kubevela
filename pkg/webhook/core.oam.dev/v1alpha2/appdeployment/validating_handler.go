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

package appdeployment

import (
	"context"
	"fmt"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ValidatingHandler handles AppDeployment
type ValidatingHandler struct {
	client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &ValidatingHandler{}

// Handle handles admission requests.
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1beta1.AppDeployment{}
	if err := h.Decoder.Decode(req, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if req.Operation == admissionv1beta1.Create || req.Operation == admissionv1beta1.Update {
		if allErrs := h.validateRevisions(obj); len(allErrs) > 0 {
			return admission.Denied(allErrs.ToAggregate().Error())
		}
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

// RegisterValidatingHandler will register AppDeployment validation to webhook
func RegisterValidatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register("/validating-core-oam-dev-v1beta1-appdeployments",
		&webhook.Admission{Handler: &ValidatingHandler{}})
}

// ValidateRevisions validates whether the ApplicationRevisions referenced by the RevisionName field are valid
func (h *ValidatingHandler) validateRevisions(appDeployment *v1beta1.AppDeployment) field.ErrorList {
	allErrs := apimachineryvalidation.ValidateObjectMeta(&appDeployment.ObjectMeta, true,
		apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))

	if appDeployment.DeletionTimestamp.IsZero() {
		fldPath := field.NewPath("spec").Child("apprevisions")
		for i, appRevision := range appDeployment.Spec.AppRevisions {
			appRevisionName := appRevision.RevisionName
			if len(appRevisionName) == 0 {
				allErrs = append(allErrs, field.Required(fldPath.Child(fmt.Sprintf("[%d]", i)),
					"target application revision name cannot be empty"))
				continue
			}

			targetAppRevision := &v1beta1.ApplicationRevision{}
			if err := h.Get(context.Background(), ktypes.NamespacedName{Namespace: appDeployment.Namespace, Name: appRevisionName},
				targetAppRevision); err != nil {
				// Other errors will be handled in the Reconcile
				if apierrors.IsNotFound(err) {
					allErrs = append(allErrs, field.NotFound(fldPath.Child("revisionName"), appRevisionName))
				}
			}
		}
	}
	return allErrs
}
