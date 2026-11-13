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

package application

import (
	"context"
	"encoding/json"
	"testing"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// A Workflow or Policy object is checked when it is written, as an Application naming its
// definitions inline would be: otherwise an Application admitted against a harmless object could
// have the object changed under it to name a definition its author may not use.
func TestReferencedObjectsAreCheckedWhenWritten(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ValidateDefinitionPermissions, true)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.RestrictDefinitionNamespaces, true)
	oldWithUser := auth.AuthenticationWithUser
	auth.AuthenticationWithUser = true
	defer func() { auth.AuthenticationWithUser = oldWithUser }()

	sys := oam.SystemDefinitionNamespace
	platformOnly := &v1beta1.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: "platform-trait", Namespace: sys},
		Spec: v1beta1.TraitDefinitionSpec{Restrictions: nsRestrictions([]string{"platform"})}}
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, wfTypesv1alpha1.AddToScheme(scheme))
	require.NoError(t, authv1.AddToScheme(scheme))
	cli := &gatekeeper{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&v1beta1.WorkflowStepDefinition{ObjectMeta: metav1.ObjectMeta{Name: "priv-step", Namespace: sys}},
			&v1beta1.WorkflowStepDefinition{ObjectMeta: metav1.ObjectMeta{Name: "notify", Namespace: sys}},
			&v1beta1.PolicyDefinition{ObjectMeta: metav1.ObjectMeta{Name: "priv-policy", Namespace: sys}},
			&v1beta1.PolicyDefinition{ObjectMeta: metav1.ObjectMeta{Name: "override", Namespace: sys}},
			&v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Name: "priv-comp", Namespace: sys}},
			platformOnly,
		).Build(),
		denied: map[string]bool{"workflowstepdefinitions/priv-step": true, "policydefinitions/priv-policy": true, "componentdefinitions/priv-comp": true},
	}
	h := &ReferencedObjectHandler{app: &ValidatingHandler{Client: cli, controllerUsername: "system:serviceaccount:vela-system:kubevela"},
		Decoder: admission.NewDecoder(scheme)}

	write := func(user string, obj client.Object, kind string) admission.Response {
		raw, err := json.Marshal(obj)
		require.NoError(t, err)
		return h.Handle(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create, Namespace: "team-a", Name: obj.GetName(),
			Kind:     metav1.GroupVersionKind{Group: "core.oam.dev", Version: "v1alpha1", Kind: kind},
			UserInfo: authenticationv1.UserInfo{Username: user}, Object: runtime.RawExtension{Raw: raw},
		}})
	}
	workflow := func(stepType string) *wfTypesv1alpha1.Workflow {
		return &wfTypesv1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: "wf", Namespace: "team-a"},
			Steps: []wfTypesv1alpha1.WorkflowStep{{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "s", Type: stepType}}}}
	}
	override := func(patch v1alpha1.EnvComponentPatch) *v1alpha1.Policy {
		return &v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{Name: "po", Namespace: "team-a"}, Type: v1alpha1.OverridePolicyType,
			Properties: rawProps(t, v1alpha1.OverridePolicySpec{Components: []v1alpha1.EnvComponentPatch{patch}})}
	}

	resp := write("tenant", workflow("priv-step"), "Workflow")
	require.False(t, resp.Allowed)
	require.Contains(t, resp.Result.Message, "steps[0].type")
	require.True(t, write("tenant", workflow("notify"), "Workflow").Allowed)

	resp = write("tenant", &v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{Name: "po", Namespace: "team-a"}, Type: "priv-policy"}, "Policy")
	require.False(t, resp.Allowed)
	require.Contains(t, resp.Result.Message, "priv-policy")

	resp = write("tenant", override(v1alpha1.EnvComponentPatch{Name: "c", Type: "priv-comp"}), "Policy")
	require.False(t, resp.Allowed)
	require.Contains(t, resp.Result.Message, "properties.components[0].type")

	resp = write("tenant", override(v1alpha1.EnvComponentPatch{Name: "c", Traits: []v1alpha1.EnvTraitPatch{{Type: "platform-trait"}}}), "Policy")
	require.False(t, resp.Allowed, "a definition restricted to other namespaces")
	require.Contains(t, resp.Result.Message, "platform-trait")

	require.True(t, write("system:serviceaccount:vela-system:kubevela", workflow("priv-step"), "Workflow").Allowed,
		"the controller's own writes come from Applications already checked")
}
