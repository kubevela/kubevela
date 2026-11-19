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

package policydefinition

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	commonconfig "github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestGlobalPoliciesOnlyInSystemNamespace(t *testing.T) {
	sc := runtime.NewScheme()
	require.NoError(t, v1beta1.SchemeBuilder.AddToScheme(sc))
	h := &ValidatingHandler{
		Decoder: admission.NewDecoder(sc),
		Client:  fake.NewClientBuilder().WithScheme(sc).Build(),
	}
	build := func(namespace string, global bool, op admissionv1.Operation) admission.Request {
		def := &v1beta1.PolicyDefinition{
			TypeMeta:   metav1.TypeMeta{Kind: "PolicyDefinition", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: "tags", Namespace: namespace},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope:     v1beta1.ApplicationScope,
				Global:    global,
				Schematic: &common.Schematic{CUE: &common.CUE{Template: "parameter: {}\noutput: labels: {a: \"b\"}\n"}},
			},
		}
		raw, err := json.Marshal(def)
		require.NoError(t, err)
		return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       "test-uid",
			Operation: op,
			Resource: metav1.GroupVersionResource{
				Group: policyDefGVR.Group, Version: policyDefGVR.Version, Resource: policyDefGVR.Resource,
			},
			Object: runtime.RawExtension{Raw: raw},
		}}
	}
	setAllowed := func(t *testing.T, allowed bool) {
		orig := commonconfig.AllowNamespaceGlobalPolicies
		t.Cleanup(func() { commonconfig.AllowNamespaceGlobalPolicies = orig })
		commonconfig.AllowNamespaceGlobalPolicies = allowed
	}

	withOld := func(req admission.Request, oldGlobal bool) admission.Request {
		old := build("tenant", oldGlobal, admissionv1.Update)
		req.OldObject = old.Object
		return req
	}

	t.Run("creating a global outside vela-system is denied", func(t *testing.T) {
		setAllowed(t, false)
		resp := h.Handle(context.Background(), build("tenant", true, admissionv1.Create))
		require.False(t, resp.Allowed)
		require.Contains(t, resp.Result.Message, oam.SystemDefinitionNamespace)
	})

	t.Run("turning global on outside vela-system is denied", func(t *testing.T) {
		setAllowed(t, false)
		resp := h.Handle(context.Background(), withOld(build("tenant", true, admissionv1.Update), false))
		require.False(t, resp.Allowed)
		require.Contains(t, resp.Result.Message, oam.SystemDefinitionNamespace)
	})

	t.Run("an update to a global already outside vela-system is admitted with a warning", func(t *testing.T) {
		setAllowed(t, false)
		resp := h.Handle(context.Background(), withOld(build("tenant", true, admissionv1.Update), true))
		require.True(t, resp.Allowed, "%v", resp.Result)
		require.Contains(t, resp.Warnings, "global policy tenant/tags does not apply: global policies apply only from the "+
			oam.SystemDefinitionNamespace+" namespace unless the controller runs with --allow-namespace-global-policies")
	})

	t.Run("a global in vela-system is admitted", func(t *testing.T) {
		setAllowed(t, false)
		resp := h.Handle(context.Background(), build(oam.SystemDefinitionNamespace, true, admissionv1.Create))
		require.True(t, resp.Allowed, "%v", resp.Result)
	})

	t.Run("a non-global policy in a namespace is admitted", func(t *testing.T) {
		setAllowed(t, false)
		resp := h.Handle(context.Background(), build("tenant", false, admissionv1.Create))
		require.True(t, resp.Allowed, "%v", resp.Result)
	})

	t.Run("namespace globals are admitted when the controller allows them", func(t *testing.T) {
		setAllowed(t, true)
		resp := h.Handle(context.Background(), build("tenant", true, admissionv1.Create))
		require.True(t, resp.Allowed, "%v", resp.Result)
	})
}

// Render picks up a policy only by its exact scope, so a misspelled one would be
// admitted and then never render.
func TestPolicyScopeMustBeKnown(t *testing.T) {
	sc := runtime.NewScheme()
	require.NoError(t, v1beta1.SchemeBuilder.AddToScheme(sc))
	h := &ValidatingHandler{Decoder: admission.NewDecoder(sc), Client: fake.NewClientBuilder().WithScheme(sc).Build()}
	handle := func(scope v1beta1.PolicyScope) admission.Response {
		def := &v1beta1.PolicyDefinition{
			TypeMeta:   metav1.TypeMeta{Kind: "PolicyDefinition", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: "tags", Namespace: "tenant"},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope:     scope,
				Schematic: &common.Schematic{CUE: &common.CUE{Template: "parameter: {}\noutput: labels: {a: \"b\"}\n"}},
			},
		}
		raw, err := json.Marshal(def)
		require.NoError(t, err)
		return h.Handle(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			UID: "test-uid", Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{Group: policyDefGVR.Group, Version: policyDefGVR.Version, Resource: policyDefGVR.Resource},
			Object:   runtime.RawExtension{Raw: raw},
		}})
	}
	for _, scope := range []v1beta1.PolicyScope{v1beta1.DefaultScope, v1beta1.ApplicationScope} {
		resp := handle(scope)
		require.True(t, resp.Allowed, "scope %q: %v", scope, resp.Result)
	}
	resp := handle("application")
	require.False(t, resp.Allowed)
	require.Contains(t, resp.Result.Message, `unknown scope "application"`)
}
