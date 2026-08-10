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
