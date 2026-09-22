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

package componentdefinition

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
	"github.com/oam-dev/kubevela/pkg/oam"
)

// A malformed glob denies every namespace, so the handler catches it on write.
func TestHandleValidatesNamespaceRestrictions(t *testing.T) {
	sc := runtime.NewScheme()
	require.NoError(t, v1beta1.SchemeBuilder.AddToScheme(sc))
	h := &ValidatingHandler{
		Decoder: admission.NewDecoder(sc),
		Client:  fake.NewClientBuilder().WithScheme(sc).Build(),
	}

	build := func(spec []string, annotation string) admission.Request {
		def := &v1beta1.ComponentDefinition{
			TypeMeta: metav1.TypeMeta{Kind: "ComponentDefinition", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "restricted-comp", Namespace: "vela-system",
			},
			Spec: v1beta1.ComponentDefinitionSpec{
				Workload:     common.WorkloadTypeDescriptor{Type: "deployments.apps"},
				Restrictions: nsRestrictions(spec),
			},
		}
		if annotation != "" {
			def.Annotations = map[string]string{oam.AnnotationRestrictNamespaces: annotation}
		}
		raw, err := json.Marshal(def)
		require.NoError(t, err)
		return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       "test-uid",
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group: componentDefGVR.Group, Version: componentDefGVR.Version, Resource: componentDefGVR.Resource,
			},
			Object: runtime.RawExtension{Raw: raw},
		}}
	}

	t.Run("names and globs are admitted", func(t *testing.T) {
		resp := h.Handle(context.Background(), build([]string{"vela-system", "tenant-*"}, ""))
		require.True(t, resp.Allowed, "%v", resp.Result)
	})

	t.Run("no restriction at all is admitted", func(t *testing.T) {
		resp := h.Handle(context.Background(), build(nil, ""))
		require.True(t, resp.Allowed, "%v", resp.Result)
	})

	t.Run("a valid annotation is admitted", func(t *testing.T) {
		resp := h.Handle(context.Background(), build(nil, "vela-system, tenant-*"))
		require.True(t, resp.Allowed, "%v", resp.Result)
	})

	t.Run("an update is validated too", func(t *testing.T) {
		req := build([]string{"tenant-["}, "")
		req.Operation = admissionv1.Update
		resp := h.Handle(context.Background(), req)
		require.False(t, resp.Allowed)
		require.Contains(t, resp.Result.Message, "tenant-[")
	})

	t.Run("an empty entry is denied", func(t *testing.T) {
		resp := h.Handle(context.Background(), build([]string{"vela-system", ""}, ""))
		require.False(t, resp.Allowed)
		require.Contains(t, resp.Result.Message, "empty")
	})

	t.Run("a malformed glob in the spec is denied", func(t *testing.T) {
		resp := h.Handle(context.Background(), build([]string{"tenant-["}, ""))
		require.False(t, resp.Allowed)
		require.Contains(t, resp.Result.Message, "tenant-[")
	})

	t.Run("a malformed glob in the annotation is denied", func(t *testing.T) {
		resp := h.Handle(context.Background(), build(nil, "tenant-["))
		require.False(t, resp.Allowed)
		require.Contains(t, resp.Result.Message, oam.AnnotationRestrictNamespaces)
	})
}

// nsRestrictions builds the spec block these tests vary; nil means the
// definition declares nothing.
func nsRestrictions(patterns []string) *common.DefinitionRestrictions {
	if patterns == nil {
		return nil
	}
	return &common.DefinitionRestrictions{Namespaces: patterns}
}
