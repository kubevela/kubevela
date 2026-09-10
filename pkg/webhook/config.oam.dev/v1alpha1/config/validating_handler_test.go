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

package config

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	legacyconfig "github.com/oam-dev/kubevela/pkg/config"
)

const templateWithRequiredUsername = `
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

func newHandler(objs ...client.Object) *ValidatingHandler {
	return &ValidatingHandler{
		Decoder: admission.NewDecoder(k8sscheme.Scheme),
		Client:  fake.NewClientBuilder().WithScheme(k8sscheme.Scheme).WithObjects(objs...).Build(),
	}
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
	h := newHandler()
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource{Group: "not", Version: "v1", Resource: "configs"}, nil)
	resp := h.Handle(context.TODO(), req)
	assert.False(t, resp.Allowed)
	assert.Equal(t, int32(http.StatusBadRequest), resp.Result.Code)
}

func TestHandle_NonCreateUpdateOperationsAreAllowedWithoutDecoding(t *testing.T) {
	h := newHandler()
	req := newRequest(t, admissionv1.Delete, metav1.GroupVersionResource(configGVR), nil)
	resp := h.Handle(context.TODO(), req)
	assert.True(t, resp.Allowed)
}

func TestHandle_DecodeError(t *testing.T) {
	h := newHandler()
	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Resource:  metav1.GroupVersionResource(configGVR),
		Object:    runtime.RawExtension{Raw: []byte(`{"spec": {`)},
	}}
	resp := h.Handle(context.TODO(), req)
	assert.False(t, resp.Allowed)
	assert.Equal(t, int32(http.StatusBadRequest), resp.Result.Code)
}

// spec.properties and spec.propertiesFrom being mutually exclusive is enforced by the
// Config controller (which sets status.phase=Error), not the webhook, so this must be
// admitted rather than denied. See config_controller.go's ErrMutuallyExclusiveProperties.
func TestHandle_MutuallyExclusivePropertiesIsAdmitted(t *testing.T) {
	h := newHandler()
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg-bad", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			Properties:     rawExtension(map[string]string{"username": "alice"}),
			PropertiesFrom: &configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: "any-secret"}},
		},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configGVR), cfg)
	resp := h.Handle(context.TODO(), req)
	assert.True(t, resp.Allowed)
}

func TestHandle_NoTemplateRefIsAllowed(t *testing.T) {
	h := newHandler()
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg-notmpl", Namespace: "default"},
		Spec:       configv1alpha1.ConfigSpec{Properties: rawExtension(map[string]string{"key": "value"})},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configGVR), cfg)
	resp := h.Handle(context.TODO(), req)
	assert.True(t, resp.Allowed)
}

func TestHandle_UnresolvableTemplateRefIsAllowed(t *testing.T) {
	h := newHandler()
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "missing", Namespace: "default"},
			Properties:  rawExtension(map[string]string{"username": "alice"}),
		},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configGVR), cfg)
	resp := h.Handle(context.TODO(), req)
	assert.True(t, resp.Allowed)
}

func TestHandle_NotYetAvailableTemplateRefIsAllowed(t *testing.T) {
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-pending", Namespace: "default"},
		Spec:       configv1alpha1.ConfigTemplateSpec{Template: templateWithRequiredUsername},
	}
	h := newHandler(ct)
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "ct-pending", Namespace: "default"},
			Properties:  rawExtension(map[string]string{"username": "alice"}),
		},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configGVR), cfg)
	resp := h.Handle(context.TODO(), req)
	assert.True(t, resp.Allowed)
}

func TestHandle_PropertiesMatchingSchemaAreAllowed(t *testing.T) {
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-avail", Namespace: "default"},
		Spec:       configv1alpha1.ConfigTemplateSpec{Template: templateWithRequiredUsername},
		Status:     configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseAvailable},
	}
	h := newHandler(ct)
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "ct-avail", Namespace: "default"},
			Properties:  rawExtension(map[string]string{"username": "alice"}),
		},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configGVR), cfg)
	resp := h.Handle(context.TODO(), req)
	assert.True(t, resp.Allowed)
}

func TestHandle_PropertiesNotMatchingSchemaAreDenied(t *testing.T) {
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-avail", Namespace: "default"},
		Spec:       configv1alpha1.ConfigTemplateSpec{Template: templateWithRequiredUsername},
		Status:     configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseAvailable},
	}
	h := newHandler(ct)
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "ct-avail", Namespace: "default"},
			Properties:  rawExtension(map[string]string{}),
		},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configGVR), cfg)
	resp := h.Handle(context.TODO(), req)
	assert.False(t, resp.Allowed)
	assert.Contains(t, string(resp.Result.Message), "properties do not match template schema")
}

func TestHandle_PropertiesFromMissingSecretIsDenied(t *testing.T) {
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-avail", Namespace: "default"},
		Spec:       configv1alpha1.ConfigTemplateSpec{Template: templateWithRequiredUsername},
		Status:     configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseAvailable},
	}
	h := newHandler(ct)
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			TemplateRef:    &configv1alpha1.ConfigTemplateReference{Name: "ct-avail", Namespace: "default"},
			PropertiesFrom: &configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: "missing-secret"}},
		},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configGVR), cfg)
	resp := h.Handle(context.TODO(), req)
	assert.False(t, resp.Allowed)
	assert.Contains(t, string(resp.Result.Message), "failed to load spec.propertiesFrom secret")
}

func TestHandle_FallsBackToLegacyConfigMapTemplate(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: legacyconfig.TemplateConfigMapNamePrefix + "legacy", Namespace: "default"},
		Data:       map[string]string{legacyconfig.SaveTemplateKey: templateWithRequiredUsername},
	}
	h := newHandler(cm)
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "legacy", Namespace: "default"},
			Properties:  rawExtension(map[string]string{"username": "alice"}),
		},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configGVR), cfg)
	resp := h.Handle(context.TODO(), req)
	assert.True(t, resp.Allowed)
}

func rawExtension(v interface{}) *runtime.RawExtension {
	data, _ := json.Marshal(v)
	return &runtime.RawExtension{Raw: data}
}

func newInterceptedHandler(funcs interceptor.Funcs, objs ...client.Object) *ValidatingHandler {
	base := fake.NewClientBuilder().WithScheme(k8sscheme.Scheme).WithObjects(objs...).Build()
	return &ValidatingHandler{
		Decoder: admission.NewDecoder(k8sscheme.Scheme),
		Client:  interceptor.NewClient(base, funcs),
	}
}

func TestHandle_ResolveTemplateGetErrorIsInternalError(t *testing.T) {
	h := newInterceptedHandler(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*configv1alpha1.ConfigTemplate); ok {
				return errors.New("boom")
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "whatever", Namespace: "default"},
			Properties:  rawExtension(map[string]string{"username": "alice"}),
		},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configGVR), cfg)
	resp := h.Handle(context.TODO(), req)
	assert.False(t, resp.Allowed)
	assert.Equal(t, int32(http.StatusInternalServerError), resp.Result.Code)
}

func TestResolveTemplate_DefaultsEmptyNamespace(t *testing.T) {
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "tmpl-ns", Namespace: "vela-system"},
		Spec:       configv1alpha1.ConfigTemplateSpec{Template: templateWithRequiredUsername},
		Status:     configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseAvailable},
	}
	h := newHandler(ct)
	_, resolved, err := h.resolveTemplate(context.TODO(), &configv1alpha1.ConfigTemplateReference{Name: "tmpl-ns"})
	require.NoError(t, err)
	assert.True(t, resolved)
}

func TestResolveTemplate_LegacyConfigMapGetErrorPropagates(t *testing.T) {
	h := newInterceptedHandler(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.ConfigMap); ok {
				return errors.New("configmap boom")
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
	_, _, err := h.resolveTemplate(context.TODO(), &configv1alpha1.ConfigTemplateReference{Name: "whatever", Namespace: "default"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "configmap boom")
}

func TestHandle_TemplateRunErrorIsDenied(t *testing.T) {
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-runerr", Namespace: "default"},
		Spec: configv1alpha1.ConfigTemplateSpec{
			Template: `
template: {
	parameter: {
		username: string
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		stringData: {
			username: parameter.username
			bad:      1 & "x"
		}
	}
}
`,
		},
		Status: configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseAvailable},
	}
	h := newHandler(ct)
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "ct-runerr", Namespace: "default"},
			Properties:  rawExtension(map[string]string{"username": "alice"}),
		},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configGVR), cfg)
	resp := h.Handle(context.TODO(), req)
	assert.False(t, resp.Allowed)
	assert.Contains(t, string(resp.Result.Message), "failed to render config template")
}

func TestHandle_ValidationReturnsDecodeErrorIsDenied(t *testing.T) {
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-baddecode", Namespace: "default"},
		Spec: configv1alpha1.ConfigTemplateSpec{
			Template: `
template: {
	parameter: {
		username: string
	}
	validation: $returns: "not-a-validation-object"
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		stringData: {
			username: parameter.username
		}
	}
}
`,
		},
		Status: configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseAvailable},
	}
	h := newHandler(ct)
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "ct-baddecode", Namespace: "default"},
			Properties:  rawExtension(map[string]string{"username": "alice"}),
		},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configGVR), cfg)
	resp := h.Handle(context.TODO(), req)
	assert.False(t, resp.Allowed)
	assert.Contains(t, string(resp.Result.Message), "template.validation.$returns format must be a validation object")
}

func TestHandle_ValidationMessageIsDenied(t *testing.T) {
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-rejects", Namespace: "default"},
		Spec: configv1alpha1.ConfigTemplateSpec{
			Template: `
template: {
	parameter: {
		username: string
	}
	validation: $returns: {
		result:  false
		message: "custom validation failure"
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		stringData: {
			username: parameter.username
		}
	}
}
`,
		},
		Status: configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseAvailable},
	}
	h := newHandler(ct)
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "ct-rejects", Namespace: "default"},
			Properties:  rawExtension(map[string]string{"username": "alice"}),
		},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configGVR), cfg)
	resp := h.Handle(context.TODO(), req)
	assert.False(t, resp.Allowed)
	assert.Contains(t, string(resp.Result.Message), "custom validation failure")
}

func TestHandle_OutputDecodeErrorIsDenied(t *testing.T) {
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-badoutput", Namespace: "default"},
		Spec: configv1alpha1.ConfigTemplateSpec{
			Template: `
template: {
	parameter: {
		username: string
	}
	output: "just-a-string"
}
`,
		},
		Status: configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseAvailable},
	}
	h := newHandler(ct)
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "ct-badoutput", Namespace: "default"},
			Properties:  rawExtension(map[string]string{"username": "alice"}),
		},
	}
	req := newRequest(t, admissionv1.Create, metav1.GroupVersionResource(configGVR), cfg)
	resp := h.Handle(context.TODO(), req)
	assert.False(t, resp.Allowed)
	assert.Contains(t, string(resp.Result.Message), "template.output format must be a secret")
}

func TestResolveProperties_PropertiesInvalidJSON(t *testing.T) {
	h := newHandler()
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			Properties: &runtime.RawExtension{Raw: []byte("5")},
		},
	}
	_, err := h.resolveProperties(context.TODO(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode spec.properties")
}

func TestResolveProperties_SecretMissingKey(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "default"},
		Data:       map[string][]byte{"other-key": []byte("x")},
	}
	h := newHandler(secret)
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			PropertiesFrom: &configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: "creds"}},
		},
	}
	_, err := h.resolveProperties(context.TODO(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no key")
}

func TestResolveProperties_SecretInvalidJSON(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "default"},
		Data:       map[string][]byte{"properties": []byte("not-json")},
	}
	h := newHandler(secret)
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: configv1alpha1.ConfigSpec{
			PropertiesFrom: &configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: "creds"}},
		},
	}
	_, err := h.resolveProperties(context.TODO(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode properties from secret key")
}
