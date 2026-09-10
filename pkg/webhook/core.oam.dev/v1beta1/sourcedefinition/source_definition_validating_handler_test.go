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

package sourcedefinition

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// A template that clears every gate the handler runs, so a denial in these
// tests is always the gate under test rather than an unrelated one.
const validSourceTemplate = `
$internal: {key: "probe-source", keyInputs: []}
schema: {host: string}
output: {host: "example.com"}
`

func handler(t *testing.T) *ValidatingHandler {
	t.Helper()
	sc := runtime.NewScheme()
	require.NoError(t, v1beta1.SchemeBuilder.AddToScheme(sc))
	return &ValidatingHandler{
		Decoder: admission.NewDecoder(sc),
		Client:  fake.NewClientBuilder().WithScheme(sc).Build(),
	}
}

func sourceDefRequest(t *testing.T, name, template string, op admissionv1.Operation) admission.Request {
	t.Helper()
	def := &v1beta1.SourceDefinition{
		TypeMeta:   metav1.TypeMeta{Kind: "SourceDefinition", APIVersion: "core.oam.dev/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system"},
	}
	if template != "" {
		def.Spec.Schematic = &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: template}}
	}
	raw, err := json.Marshal(def)
	require.NoError(t, err)
	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:       "test-uid",
		Operation: op,
		Resource: metav1.GroupVersionResource{
			Group: sourceDefGVR.Group, Version: sourceDefGVR.Version, Resource: sourceDefGVR.Resource,
		},
		Object: runtime.RawExtension{Raw: raw},
	}}
}

func TestHandleAdmitsAValidDefinition(t *testing.T) {
	for _, op := range []admissionv1.Operation{admissionv1.Create, admissionv1.Update} {
		t.Run(string(op), func(t *testing.T) {
			resp := handler(t).Handle(context.Background(),
				sourceDefRequest(t, "probe-source", validSourceTemplate, op))
			require.True(t, resp.Allowed, "%v", resp.Result)
		})
	}
}

// Delete carries no object to validate, and refusing it would make a definition
// impossible to remove.
func TestHandleSkipsOperationsItDoesNotValidate(t *testing.T) {
	for _, op := range []admissionv1.Operation{admissionv1.Delete, admissionv1.Connect} {
		resp := handler(t).Handle(context.Background(),
			sourceDefRequest(t, "probe-source", "", op))
		require.True(t, resp.Allowed, "%s should be admitted untouched", op)
	}
}

// The handler is registered for one resource. Anything else reaching it means
// the registration is wrong, and admitting it would validate nothing while
// looking like it had.
func TestHandleRefusesAnUnexpectedResource(t *testing.T) {
	req := sourceDefRequest(t, "probe-source", validSourceTemplate, admissionv1.Create)
	req.Resource = metav1.GroupVersionResource{
		Group: "core.oam.dev", Version: "v1beta1", Resource: "componentdefinitions"}

	resp := handler(t).Handle(context.Background(), req)
	require.False(t, resp.Allowed)
	require.Equal(t, int32(http.StatusBadRequest), resp.Result.Code)
	require.Contains(t, resp.Result.Message, "expect resource to be")
}

func TestHandleRefusesAnUndecodablePayload(t *testing.T) {
	req := sourceDefRequest(t, "probe-source", validSourceTemplate, admissionv1.Create)
	req.Object = runtime.RawExtension{Raw: []byte("{not json")}

	resp := handler(t).Handle(context.Background(), req)
	require.False(t, resp.Allowed)
	require.Equal(t, int32(http.StatusBadRequest), resp.Result.Code)
}

// Each denial names its own cause. An operator reading "denied" with the wrong
// reason attached debugs the wrong thing.
func TestHandleDeniesAndSaysWhy(t *testing.T) {
	for _, tc := range []struct {
		name     string
		template string
		want     string
	}{
		{
			name:     "no schematic at all",
			template: "",
			want:     "must declare spec.schematic.cue",
		},
		{
			name:     "cue that does not compile",
			template: `output: {this is not cue`,
			want:     "expected",
		},
		{
			name: "no $internal block, so no cache identity",
			template: `
schema: {host: string}
output: {host: "example.com"}`,
			want: "$internal",
		},
		{
			name: "a cache key that does not match the context the template reads",
			template: `
$internal: {key: "wrong-key", keyInputs: []}
schema: {host: string}
output: {host: "example.com"}`,
			want: "does not match",
		},
		{
			name: "no schema block, so reads go unvalidated in both layers",
			template: `
$internal: {key: "probe-source", keyInputs: []}
output: {host: "example.com"}`,
			want: "schema",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := handler(t).Handle(context.Background(),
				sourceDefRequest(t, "probe-source", tc.template, admissionv1.Create))
			require.False(t, resp.Allowed)
			require.Contains(t, resp.Result.Message, tc.want)
			require.Contains(t, resp.Result.Message, "requestUID=test-uid",
				"the request id is what correlates a denial with the controller log")
		})
	}
}

// A metadata-only update carries a spec this build may have no opinion it can
// act on: a definition generated by an older release records rules this binary
// does not embed, and re-validating it turns `kubectl label` into a denial
// naming cache-key rules. The object then cannot be labelled, annotated or
// adopted by Helm until somebody rewrites its spec, and the message says
// nothing about why a label was refused.
//
// Nothing is lost by skipping: the spec was validated when it was written, and
// an unchanged spec is not a new claim.
func TestHandleAdmitsAMetadataOnlyUpdate(t *testing.T) {
	// A template that would be denied if it were being introduced now.
	const staleTemplate = `
$internal: {key: "wrong-key", keyInputs: []}
schema: {host: string}
output: {host: "example.com"}
`
	req := sourceDefRequest(t, "probe-source", staleTemplate, admissionv1.Update)

	t.Run("still denied when it is the spec that changed", func(t *testing.T) {
		old := sourceDefRequest(t, "probe-source", validSourceTemplate, admissionv1.Update)
		r := req
		r.OldObject = old.Object
		resp := handler(t).Handle(context.Background(), r)
		require.False(t, resp.Allowed, "a changed spec must still be validated")
	})

	t.Run("admitted when only metadata changed", func(t *testing.T) {
		def := &v1beta1.SourceDefinition{
			TypeMeta:   metav1.TypeMeta{Kind: "SourceDefinition", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: "probe-source", Namespace: "vela-system"},
			Spec: v1beta1.SourceDefinitionSpec{
				Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: staleTemplate}},
			},
		}
		oldRaw, err := json.Marshal(def)
		require.NoError(t, err)

		def.Labels = map[string]string{"app.kubernetes.io/managed-by": "Helm"}
		newRaw, err := json.Marshal(def)
		require.NoError(t, err)

		r := req
		r.Object = runtime.RawExtension{Raw: newRaw}
		r.OldObject = runtime.RawExtension{Raw: oldRaw}
		resp := handler(t).Handle(context.Background(), r)
		require.True(t, resp.Allowed, "%v", resp.Result)
	})

	// A create has no old object to compare against, and an update from a client
	// that sends none is indistinguishable from one. Validate rather than assume.
	t.Run("denied when no old object was sent", func(t *testing.T) {
		resp := handler(t).Handle(context.Background(), req)
		require.False(t, resp.Allowed)
	})
}

// spec.version is what makes each version a distinct DefinitionRevision, so a
// malformed one is accepted and then fails when reconciliation parses it,
// leaving the definition unusable with nothing to say why. Every other
// definition kind validates it at admission.
func TestHandleValidatesTheDeclaredVersion(t *testing.T) {
	build := func(version string, annotations map[string]string) admission.Request {
		def := &v1beta1.SourceDefinition{
			TypeMeta: metav1.TypeMeta{Kind: "SourceDefinition", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "probe-source", Namespace: "vela-system", Annotations: annotations,
			},
			Spec: v1beta1.SourceDefinitionSpec{
				Version:   version,
				Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: validSourceTemplate}},
			},
		}
		raw, err := json.Marshal(def)
		require.NoError(t, err)
		req := sourceDefRequest(t, "probe-source", validSourceTemplate, admissionv1.Create)
		req.Object = runtime.RawExtension{Raw: raw}
		return req
	}

	t.Run("a semantic version is admitted", func(t *testing.T) {
		resp := handler(t).Handle(context.Background(), build("1.2.3", nil))
		require.True(t, resp.Allowed, "%v", resp.Result)
	})

	t.Run("no version at all is admitted", func(t *testing.T) {
		resp := handler(t).Handle(context.Background(), build("", nil))
		require.True(t, resp.Allowed, "%v", resp.Result)
	})

	t.Run("a malformed version is denied", func(t *testing.T) {
		resp := handler(t).Handle(context.Background(), build("v1", nil))
		require.False(t, resp.Allowed)
		require.Contains(t, resp.Result.Message, "requestUID=test-uid")
	})

	// Both name the revision, and they can disagree.
	t.Run("a version and a revision annotation together are denied", func(t *testing.T) {
		resp := handler(t).Handle(context.Background(),
			build("1.2.3", map[string]string{oam.AnnotationDefinitionRevisionName: "1"}))
		require.False(t, resp.Allowed)
	})
}
