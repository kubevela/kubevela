/*
 Copyright 2021. The KubeVela Authors.

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

package componentdefinition

import (
	"context"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	webhookutils "github.com/oam-dev/kubevela/pkg/webhook/utils"
)

var componentDefGVR = v1beta1.SchemeGroupVersion.WithResource("componentdefinitions")

// ValidatingHandler handles validation of component definition
type ValidatingHandler struct {
	// Decoder decodes object
	Decoder *admission.Decoder
	Client  client.Client
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

var _ admission.Handler = &ValidatingHandler{}

func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1beta1.ComponentDefinition{}
	if req.Resource.String() != componentDefGVR.String() {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("expect resource to be %s", componentDefGVR))
	}

	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		err := h.Decoder.Decode(req, obj)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		err = ValidateWorkload(h.Client.RESTMapper(), obj)
		if err != nil {
			return admission.Denied(err.Error())
		}

		// validate cueTemplate
		if obj.Spec.Schematic != nil && obj.Spec.Schematic.CUE != nil {
			err = webhookutils.ValidateCueTemplate(obj.Spec.Schematic.CUE.Template)
			if err != nil {
				return admission.Denied(err.Error())
			}
		}

		if obj.Spec.Version != "" {
			err = webhookutils.ValidSemanticVersion(obj.Spec.Version)
			if err != nil {
				return admission.Denied(err.Error())
			}
		}

		revisionName := obj.GetAnnotations()[oam.AnnotationDefinitionRevisionName]
		if len(revisionName) != 0 {
			defRevName := fmt.Sprintf("%s-v%s", obj.Name, revisionName)
			err = webhookutils.ValidateDefinitionRevision(ctx, h.Client, obj, client.ObjectKey{Namespace: obj.Namespace, Name: defRevName})
			if err != nil {
				return admission.Denied(err.Error())
			}
		}

		version := obj.Spec.Version
		err = webhookutils.ValidateVersionAndRevisionNameAnnotation(version, revisionName, obj.Kind)
		if err != nil {
			return admission.Denied(err.Error())
		}
	}
	return admission.ValidationResponse(true, "")
}

var _ admission.DecoderInjector = &ValidatingHandler{}

// InjectDecoder injects the decoder into the ValidatingHandler
func (h *ValidatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

// RegisterValidatingHandler will register ComponentDefinition validation to webhook
func RegisterValidatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register("/validating-core-oam-dev-v1beta1-componentdefinitions", &webhook.Admission{Handler: &ValidatingHandler{}})
}

// ValidateWorkload validates whether the Workload field is valid
func ValidateWorkload(mapper meta.RESTMapper, cd *v1beta1.ComponentDefinition) error {

	// If the Type and Definition are all empty, it will be rejected.
	if cd.Spec.Workload.Type == "" && cd.Spec.Workload.Definition == (common.WorkloadGVK{}) {
		return fmt.Errorf("neither the type nor the definition of the workload field in the ComponentDefinition %s can be empty", cd.Name)
	}

	// if Type and Definitiondon‘t point to the same workloaddefinition, it will be rejected.
	if cd.Spec.Workload.Type != "" && cd.Spec.Workload.Definition != (common.WorkloadGVK{}) {
		defRef, err := util.ConvertWorkloadGVK2Definition(mapper, cd.Spec.Workload.Definition)
		if err != nil {
			return err
		}
		if defRef.Name != cd.Spec.Workload.Type {
			return fmt.Errorf("the type and the definition of the workload field in ComponentDefinition %s should represent the same workload", cd.Name)
		}
	}
	return nil
}
