/*
Copyright 2026 The KubeVela Authors.

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

package configtemplate

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
)

const validTemplate = `
template: {
	parameter: {
		username: string
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		stringData: {
			username: parameter.username
		}
	}
}
`

// A template with a well-formed nacos block: the endpoint is concrete, the metadata
// references the parameter, the format is one the writer encodes.
const nacosTemplate = `
template: {
	nacos: {
		endpoint: {
			name:      "nacos"
			namespace: "vela-system"
		}
		format: "properties"
		metadata: {
			dataId: parameter.dataId
			group:  parameter.group
		}
		content: parameter.content
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		stringData: {
			dataId: parameter.dataId
		}
	}
	parameter: {
		dataId: string
		group:  *"DEFAULT_GROUP" | string
		content: {
			host: string
			port: int
		}
	}
}
`

// The same block without endpoint.name: the one field the writer cannot do without and
// cannot take from the parameter.
const nacosTemplateWithoutEndpointName = `
template: {
	nacos: {
		endpoint: {
			namespace: "vela-system"
		}
		format: "properties"
		metadata: {
			dataId: parameter.dataId
			group:  parameter.group
		}
		content: parameter.content
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
	}
	parameter: {
		dataId:  string
		group:   *"DEFAULT_GROUP" | string
		content: {...}
	}
}
`

// A concrete format the writer does not encode (it would silently render yaml today).
const nacosTemplateWithUnknownFormat = `
template: {
	nacos: {
		endpoint: {
			name: "nacos"
		}
		format: "ini"
		metadata: {
			dataId: parameter.dataId
			group:  parameter.group
		}
		content: parameter.content
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
	}
	parameter: {
		dataId:  string
		group:   *"DEFAULT_GROUP" | string
		content: {...}
	}
}
`

func newHandler(t *testing.T) *ValidatingHandler {
	t.Helper()
	return &ValidatingHandler{Decoder: admission.NewDecoder(k8sscheme.Scheme)}
}

func newRequest(t *testing.T, op admissionv1.Operation, gvr metav1.GroupVersionResource, obj interface{}) admission.Request {
	t.Helper()
	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: op,
		Resource:  gvr,
	}}
	if obj != nil {
		raw, err := json.Marshal(obj)
		assert.NoError(t, err)
		req.Object = runtime.RawExtension{Raw: raw}
	}
	return req
}

func TestHandle_WrongResource(t *testing.T) {
	h := newHandler(t)
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource{Group: "not", Version: "v1", Resource: "configtemplates"}, nil)
	resp := h.Handle(context.TODO(), req)
	assert.False(t, resp.Allowed)
	assert.Equal(t, int32(http.StatusBadRequest), resp.Result.Code)
}

func TestHandle_NonCreateUpdateOperationsAreAllowedWithoutDecoding(t *testing.T) {
	h := newHandler(t)
	req := newRequest(t, admissionv1.Delete, metav1.GroupVersionResource(configTemplateGVR), nil)
	resp := h.Handle(context.TODO(), req)
	assert.True(t, resp.Allowed)
}

func TestHandle_DecodeError(t *testing.T) {
	h := newHandler(t)
	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Resource:  metav1.GroupVersionResource(configTemplateGVR),
		Object:    runtime.RawExtension{Raw: []byte(`{"spec": {`)},
	}}
	resp := h.Handle(context.TODO(), req)
	assert.False(t, resp.Allowed)
	assert.Equal(t, int32(http.StatusBadRequest), resp.Result.Code)
}

func TestHandle_ValidCUEIsAdmitted(t *testing.T) {
	h := newHandler(t)
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-valid", Namespace: "default"},
		Spec:       configv1alpha1.ConfigTemplateSpec{Template: validTemplate},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configTemplateGVR), ct)
	resp := h.Handle(context.TODO(), req)
	assert.True(t, resp.Allowed)
}

// A ConfigTemplate with CUE that fails to compile must still be admitted: the
// ConfigTemplate controller is responsible for surfacing compile failures via
// status.phase=Error, not the webhook. See configtemplate_controller.go markError.
func TestHandle_InvalidCUEIsStillAdmitted(t *testing.T) {
	h := newHandler(t)
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-invalid", Namespace: "default"},
		Spec:       configv1alpha1.ConfigTemplateSpec{Template: `this is not valid cue {{{`},
	}

	createReq := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configTemplateGVR), ct)
	resp := h.Handle(context.TODO(), createReq)
	assert.True(t, resp.Allowed)

	updateReq := newRequest(t, admissionv1.Update, metav1.GroupVersionResource(configTemplateGVR), ct)
	resp = h.Handle(context.TODO(), updateReq)
	assert.True(t, resp.Allowed)
}

func TestHandle_NacosBlockIsAdmitted(t *testing.T) {
	h := newHandler(t)
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-nacos", Namespace: "default"},
		Spec:       configv1alpha1.ConfigTemplateSpec{Template: nacosTemplate},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configTemplateGVR), ct)
	resp := h.Handle(context.TODO(), req)
	assert.True(t, resp.Allowed, "%v", resp.Result)
}

// The shape of the nacos block is admission's question: a block without endpoint.name is
// refused on create and on update, with the writer's message, so the author sees it at
// apply time rather than on the first Config made from the template.
func TestHandle_NacosBlockWithoutEndpointNameIsDenied(t *testing.T) {
	h := newHandler(t)
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-nacos-noendpoint", Namespace: "default"},
		Spec:       configv1alpha1.ConfigTemplateSpec{Template: nacosTemplateWithoutEndpointName},
	}
	for _, op := range []admissionv1.Operation{admissionv1.Create, admissionv1.Update} {
		req := newRequest(t, op, metav1.GroupVersionResource(configTemplateGVR), ct)
		resp := h.Handle(context.TODO(), req)
		assert.False(t, resp.Allowed, "operation %s", op)
		assert.Equal(t, int32(http.StatusForbidden), resp.Result.Code, "operation %s", op)
		assert.Contains(t, resp.Result.Message, "nacos.endpoint.name is required", "operation %s", op)
	}
}

func TestHandle_NacosBlockWithUnknownFormatIsDenied(t *testing.T) {
	h := newHandler(t)
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-nacos-ini", Namespace: "default"},
		Spec:       configv1alpha1.ConfigTemplateSpec{Template: nacosTemplateWithUnknownFormat},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configTemplateGVR), ct)
	resp := h.Handle(context.TODO(), req)
	assert.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, `nacos.format "ini" is not one of`)
}

// A template that imports a vela package, as the shipped image-registry template does,
// is parsed with the CueX compiler: its block is checked like any other rather than
// admitted because the plain compiler could not read it.
func TestHandle_NacosBlockBehindAVelaImportIsStillChecked(t *testing.T) {
	h := newHandler(t)
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-nacos-import", Namespace: "default"},
		Spec:       configv1alpha1.ConfigTemplateSpec{Template: nacosTemplateWithImportAndUnknownFormat},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configTemplateGVR), ct)
	resp := h.Handle(context.TODO(), req)
	assert.False(t, resp.Allowed)
	assert.Contains(t, resp.Result.Message, `nacos.format "ini" is not one of`)
}

// nacosTemplateWithImportAndUnknownFormat is nacosTemplateWithUnknownFormat behind an
// import of vela/config, the way the shipped image-registry template validates itself.
const nacosTemplateWithImportAndUnknownFormat = `
import "vela/config"

template: {
	validation: config.#ImageRegistry & {
		$params: registry: "index.docker.io"
	}
	nacos: {
		endpoint: name: "nacos"
		format: "ini"
		metadata: {
			dataId: parameter.dataId
			group:  parameter.group
		}
		content: parameter.content
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
	}
	parameter: {
		dataId:  string
		group:   *"DEFAULT_GROUP" | string
		content: {...}
	}
}
`
