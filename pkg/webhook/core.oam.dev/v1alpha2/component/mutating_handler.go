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
	"net/http"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/discoverymapper"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

const (
	// TypeField is the special field indicate the type of the workloadDefinition
	TypeField = "type"
)

// MutatingHandler handles Component
type MutatingHandler struct {
	Client client.Client
	Mapper discoverymapper.DiscoveryMapper

	// Decoder decodes objects
	Decoder *admission.Decoder
}

// log is for logging in this package.
var mutatelog = logf.Log.WithName("component mutate webhook")

var _ admission.Handler = &MutatingHandler{}

// Handle handles admission requests.
func (h *MutatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1alpha2.Component{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	// mutate the object
	if err := h.Mutate(obj); err != nil {
		mutatelog.Error(err, "failed to mutate the component", "name", obj.Name)
		return admission.Errored(http.StatusBadRequest, err)
	}
	mutatelog.Info("Print the mutated obj", "obj name", obj.Name, "mutated obj", string(obj.Spec.Workload.Raw))

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
	if len(resp.Patches) > 0 {
		mutatelog.Info("admit Component",
			"namespace", obj.Namespace, "name", obj.Name, "patches", util.JSONMarshal(resp.Patches))
	}
	return resp
}

// Mutate sets all the default value for the Component
func (h *MutatingHandler) Mutate(obj *v1alpha2.Component) error {
	mutatelog.Info("mutate", "name", obj.Name)
	var content map[string]interface{}
	if err := json.Unmarshal(obj.Spec.Workload.Raw, &content); err != nil {
		return err
	}
	if content[TypeField] != nil {
		workloadType, ok := content[TypeField].(string)
		if !ok {
			return fmt.Errorf("workload content has an unknown type field")
		}
		mutatelog.Info("the component refers to workoadDefinition by type", "name", obj.Name, "workload type", workloadType)
		// Fetch the corresponding workloadDefinition CR, the workloadDefinition crd is cluster scoped
		workloadDefinition := &v1alpha2.WorkloadDefinition{}
		if err := h.Client.Get(context.TODO(), types.NamespacedName{Name: workloadType}, workloadDefinition); err != nil {
			return err
		}
		gvk, err := util.GetGVKFromDefinition(h.Mapper, workloadDefinition.Spec.Reference)
		if err != nil {
			return err
		}
		// reconstruct the workload CR
		delete(content, TypeField)
		workload := unstructured.Unstructured{
			Object: content,
		}
		// find out the GVK from the CRD definition and set
		apiVersion := metav1.GroupVersion{
			Group:   gvk.Group,
			Version: gvk.Version,
		}.String()
		workload.SetAPIVersion(apiVersion)
		workload.SetKind(gvk.Kind)
		mutatelog.Info("Set the component workload GVK", "workload api version", workload.GetAPIVersion(), "workload Kind", workload.GetKind())
		// copy namespace/label/annotation to the workload and add workloadType label
		workload.SetNamespace(obj.GetNamespace())
		workload.SetLabels(util.MergeMapOverrideWithDst(obj.GetLabels(), map[string]string{oam.WorkloadTypeLabel: workloadType}))
		workload.SetAnnotations(obj.GetAnnotations())
		// copy back the object
		rawBye, err := json.Marshal(workload.Object)
		if err != nil {
			return err
		}
		obj.Spec.Workload.Raw = rawBye
	}

	return nil
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
func RegisterMutatingHandler(mgr manager.Manager) error {
	mapper, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		return err
	}
	server := mgr.GetWebhookServer()
	server.Register("/mutating-core-oam-dev-v1alpha2-components", &webhook.Admission{Handler: &MutatingHandler{Mapper: mapper}})
	return nil
}
