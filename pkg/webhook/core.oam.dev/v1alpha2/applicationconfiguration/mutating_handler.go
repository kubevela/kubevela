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

package applicationconfiguration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	// TraitTypeField is the special field indicate the type of the traitDefinition
	TraitTypeField = "name"
	// TraitSpecField indicate the spec of the trait in ApplicationConfiguration
	TraitSpecField = "properties"
)

// MutatingHandler handles Component
type MutatingHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &MutatingHandler{}

// Handle handles admission requests.
func (h *MutatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1alpha2.ApplicationConfiguration{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	// mutate the object
	if err := h.Mutate(ctx, obj); err != nil {
		klog.InfoS("Failed to mutate the applicationConfiguration", "applicationConfiguration", klog.KObj(obj),
			"err", err)
		return admission.Errored(http.StatusBadRequest, err)
	}
	klog.InfoS("Print the mutated applicationConfiguration", "applicationConfiguration",
		klog.KObj(obj), "mutated applicationConfiguration", string(util.JSONMarshal(obj.Spec)))

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
	if len(resp.Patches) > 0 {
		klog.InfoS("Admit applicationConfiguration", "applicationConfiguration", klog.KObj(obj),
			"patches", util.JSONMarshal(resp.Patches))
	}
	return resp
}

// Mutate sets all the default value for the Component
func (h *MutatingHandler) Mutate(ctx context.Context, obj *v1alpha2.ApplicationConfiguration) error {
	klog.InfoS("Mutate applicationConfiguration", "applicationConfiguration", klog.KObj(obj))

	for compIdx, comp := range obj.Spec.Components {
		var updated bool
		for idx, tr := range comp.Traits {
			var content map[string]interface{}
			if err := json.Unmarshal(tr.Trait.Raw, &content); err != nil {
				return err
			}
			rawByte, mutated, err := h.mutateTrait(ctx, content, comp.ComponentName)
			if err != nil {
				return err
			}
			if !mutated {
				continue
			}
			tr.Trait.Raw = rawByte
			comp.Traits[idx] = tr
			updated = true
		}
		if updated {
			obj.Spec.Components[compIdx] = comp
		}
	}

	return nil
}

func (h *MutatingHandler) mutateTrait(ctx context.Context, content map[string]interface{}, compName string) ([]byte, bool, error) {
	if content[TraitTypeField] == nil {
		return nil, false, nil
	}
	traitType, ok := content[TraitTypeField].(string)
	if !ok {
		return nil, false, fmt.Errorf("name of trait should be string instead of %s", reflect.TypeOf(content[TraitTypeField]))
	}
	klog.InfoS("Trait refers to traitDefinition by name", "compName", compName, "trait name", traitType)
	// Fetch the corresponding traitDefinition CR, the traitDefinition crd is cluster scoped
	traitDefinition := &v1alpha2.TraitDefinition{}
	if err := h.Client.Get(ctx, types.NamespacedName{Name: traitType}, traitDefinition); err != nil {
		return nil, false, err
	}
	// fetch the CRDs definition
	customResourceDefinition := &crdv1.CustomResourceDefinition{}
	if err := h.Client.Get(ctx, types.NamespacedName{Name: traitDefinition.Spec.Reference.Name}, customResourceDefinition); err != nil {
		return nil, false, err
	}
	// reconstruct the trait CR
	delete(content, TraitTypeField)

	if content[TraitSpecField] != nil {
		content["spec"] = content[TraitSpecField]
		delete(content, TraitSpecField)
	}

	trait := unstructured.Unstructured{
		Object: content,
	}
	// find out the GVK from the CRD definition and set
	apiVersion := metav1.GroupVersion{
		Group:   customResourceDefinition.Spec.Group,
		Version: customResourceDefinition.Spec.Versions[0].Name,
	}.String()
	trait.SetAPIVersion(apiVersion)
	trait.SetKind(customResourceDefinition.Spec.Names.Kind)
	klog.InfoS("Set the trait GVK", "trait apiVersion", trait.GetAPIVersion(), "trait Kind", trait.GetKind())
	// add traitType label
	trait.SetLabels(util.MergeMapOverrideWithDst(trait.GetLabels(), map[string]string{oam.TraitTypeLabel: traitType}))
	// copy back the object
	rawBye, err := json.Marshal(trait.Object)
	if err != nil {
		return nil, false, err
	}
	return rawBye, true, nil
}

var _ inject.Client = &MutatingHandler{}

// InjectClient injects the client into the ComponentMutatingHandler
func (h *MutatingHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &MutatingHandler{}

// InjectDecoder injects the decoder into the ComponentMutatingHandler
func (h *MutatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

// RegisterMutatingHandler will register component mutation handler to the webhook
func RegisterMutatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register("/mutating-core-oam-dev-v1alpha2-applicationconfigurations", &webhook.Admission{Handler: &MutatingHandler{}})
}
