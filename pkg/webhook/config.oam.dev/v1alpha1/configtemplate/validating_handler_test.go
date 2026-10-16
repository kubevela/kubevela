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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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

// newHandler has a client with no SubjectAccessReview answer: a test that reaches a review
// uses reviewingHandler instead.
func newHandler(t *testing.T) *ValidatingHandler {
	t.Helper()
	return &ValidatingHandler{Decoder: admission.NewDecoder(k8sscheme.Scheme), Client: fake.NewClientBuilder().WithScheme(k8sscheme.Scheme).Build()}
}

func newUpdateRequest(t *testing.T, gvr metav1.GroupVersionResource, oldObj, newObj interface{}) admission.Request {
	t.Helper()
	req := newRequest(t, admissionv1.Update, gvr, newObj)
	raw, err := json.Marshal(oldObj)
	assert.NoError(t, err)
	req.OldObject = runtime.RawExtension{Raw: raw}
	return req
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

// reviewingHandler answers every SubjectAccessReview with allowed, and records the reviews.
func reviewingHandler(t *testing.T, allowed bool, reviews *[]authorizationv1.ResourceAttributes) *ValidatingHandler {
	t.Helper()
	base := fake.NewClientBuilder().WithScheme(k8sscheme.Scheme).Build()
	return &ValidatingHandler{
		Decoder: admission.NewDecoder(k8sscheme.Scheme),
		Client: interceptor.NewClient(base, interceptor.Funcs{
			Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
				if review, ok := obj.(*authorizationv1.SubjectAccessReview); ok {
					*reviews = append(*reviews, *review.Spec.ResourceAttributes)
					review.Status.Allowed = allowed
				}
				return nil
			},
		}),
	}
}

func templateIn(name, namespace string, scope configv1alpha1.ConfigTemplateScope, cue string) *configv1alpha1.ConfigTemplate {
	return &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       configv1alpha1.ConfigTemplateSpec{Scope: scope, Template: cue},
	}
}

func TestHandle_SystemScopeLivesInVelaSystem(t *testing.T) {
	gvr := metav1.GroupVersionResource(configTemplateGVR)
	h := newHandler(t)
	plain := "template: {parameter: {}\noutput: {apiVersion: \"v1\", kind: \"Secret\"}}"

	resp := h.Handle(context.TODO(), newRequest(t, admissionv1.Create, gvr, templateIn("t", "wf-a", configv1alpha1.ConfigTemplateScopeSystem, plain)))
	assert.False(t, resp.Allowed, "scope system outside vela-system")
	assert.Contains(t, string(resp.Result.Message), "config template wf-a/t: scope system is only valid in vela-system")

	resp = h.Handle(context.TODO(), newRequest(t, admissionv1.Create, gvr, templateIn("t", "vela-system", configv1alpha1.ConfigTemplateScopeSystem, plain)))
	assert.True(t, resp.Allowed, "scope system in vela-system")

	resp = h.Handle(context.TODO(), newRequest(t, admissionv1.Create, gvr, templateIn("t", "wf-a", configv1alpha1.ConfigTemplateScopeNamespace, plain)))
	assert.True(t, resp.Allowed, "scope namespace anywhere")

	// an update that turns a namespace-scoped template into a system one elsewhere is the same mistake
	resp = h.Handle(context.TODO(), newRequest(t, admissionv1.Update, gvr, templateIn("t", "wf-a", configv1alpha1.ConfigTemplateScopeSystem, plain)))
	assert.False(t, resp.Allowed, "scope system outside vela-system on update")
}

func TestHandle_NacosEndpointInAnotherNamespaceNeedsTheAuthorToReadIt(t *testing.T) {
	gvr := metav1.GroupVersionResource(configTemplateGVR)
	elsewhere := strings.Replace(nacosTemplate, `namespace: "vela-system"`, `namespace: "wf-b"`, 1)
	assert.NotEqual(t, nacosTemplate, elsewhere, "the fixture names the vela-system endpoint")
	as := func(req admission.Request) admission.Request {
		req.UserInfo = authenticationv1.UserInfo{Username: "alice", Groups: []string{"scope-users"}}
		return req
	}

	t.Run("the template's own namespace: no review", func(t *testing.T) {
		var reviews []authorizationv1.ResourceAttributes
		h := reviewingHandler(t, false, &reviews)
		own := strings.Replace(nacosTemplate, `namespace: "vela-system"`, `namespace: "wf-a"`, 1)
		resp := h.Handle(context.TODO(), as(newRequest(t, admissionv1.Create, gvr, templateIn("t", "wf-a", configv1alpha1.ConfigTemplateScopeNamespace, own))))
		assert.True(t, resp.Allowed, string(resp.Result.Message))
		assert.Empty(t, reviews)
	})
	t.Run("vela-system, the shared endpoints: no review", func(t *testing.T) {
		var reviews []authorizationv1.ResourceAttributes
		h := reviewingHandler(t, false, &reviews)
		resp := h.Handle(context.TODO(), as(newRequest(t, admissionv1.Create, gvr, templateIn("t", "wf-a", configv1alpha1.ConfigTemplateScopeNamespace, nacosTemplate))))
		assert.True(t, resp.Allowed, string(resp.Result.Message))
		assert.Empty(t, reviews)
	})
	t.Run("another namespace, the author may not read the Secret: denied", func(t *testing.T) {
		var reviews []authorizationv1.ResourceAttributes
		h := reviewingHandler(t, false, &reviews)
		resp := h.Handle(context.TODO(), as(newRequest(t, admissionv1.Create, gvr, templateIn("t", "wf-a", configv1alpha1.ConfigTemplateScopeNamespace, elsewhere))))
		assert.False(t, resp.Allowed)
		assert.Contains(t, string(resp.Result.Message), "names the nacos endpoint wf-b/nacos in another namespace, which alice may not read")
		if assert.Len(t, reviews, 1) {
			assert.Equal(t, authorizationv1.ResourceAttributes{Verb: "get", Version: "v1", Resource: "secrets", Namespace: "wf-b", Name: "nacos"}, reviews[0])
		}
	})
	t.Run("another namespace, the author may read it: admitted", func(t *testing.T) {
		var reviews []authorizationv1.ResourceAttributes
		h := reviewingHandler(t, true, &reviews)
		resp := h.Handle(context.TODO(), as(newRequest(t, admissionv1.Create, gvr, templateIn("t", "wf-a", configv1alpha1.ConfigTemplateScopeNamespace, elsewhere))))
		assert.True(t, resp.Allowed, string(resp.Result.Message))
		assert.Len(t, reviews, 1)
	})
	t.Run("an endpoint namespace left to the parameter: refused, the rule could not read it", func(t *testing.T) {
		var reviews []authorizationv1.ResourceAttributes
		h := reviewingHandler(t, true, &reviews)
		parameterized := strings.Replace(nacosTemplate, `namespace: "vela-system"`, `namespace: parameter.ns`, 1)
		parameterized = strings.Replace(parameterized, "parameter: {", "parameter: {\n\t\tns: string", 1)
		resp := h.Handle(context.TODO(), as(newRequest(t, admissionv1.Create, gvr, templateIn("t", "wf-a", configv1alpha1.ConfigTemplateScopeNamespace, parameterized))))
		assert.False(t, resp.Allowed)
		assert.Contains(t, string(resp.Result.Message), "nacos.endpoint.namespace must be a concrete, non-empty string")
		assert.Empty(t, reviews)
	})
	t.Run("an update that leaves the template text alone: no review (admitted before the rule)", func(t *testing.T) {
		var reviews []authorizationv1.ResourceAttributes
		h := reviewingHandler(t, false, &reviews)
		stored := templateIn("t", "wf-a", configv1alpha1.ConfigTemplateScopeNamespace, elsewhere)
		edited := stored.DeepCopy()
		edited.Labels = map[string]string{"team": "payments"}
		resp := h.Handle(context.TODO(), as(newUpdateRequest(t, gvr, stored, edited)))
		assert.True(t, resp.Allowed, string(resp.Result.Message))
		assert.Empty(t, reviews)
	})
	t.Run("an update that introduces the endpoint: reviewed", func(t *testing.T) {
		var reviews []authorizationv1.ResourceAttributes
		h := reviewingHandler(t, false, &reviews)
		resp := h.Handle(context.TODO(), as(newUpdateRequest(t, gvr,
			templateIn("t", "wf-a", configv1alpha1.ConfigTemplateScopeNamespace, nacosTemplate),
			templateIn("t", "wf-a", configv1alpha1.ConfigTemplateScopeNamespace, elsewhere))))
		assert.False(t, resp.Allowed)
		assert.Len(t, reviews, 1)
	})
}
