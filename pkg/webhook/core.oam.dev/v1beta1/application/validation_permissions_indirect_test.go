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
	"strings"
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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// gatekeeper answers access reviews: the names in denied are refused, everything else allowed.
type gatekeeper struct {
	client.Client
	denied map[string]bool
}

func (g *gatekeeper) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if sar, ok := obj.(*authv1.SubjectAccessReview); ok {
		sar.Status.Allowed = !g.denied[sar.Spec.ResourceAttributes.Resource+"/"+sar.Spec.ResourceAttributes.Name]
		return nil
	}
	return g.Client.Create(ctx, obj, opts...)
}

func rawProps(t *testing.T, v interface{}) *runtime.RawExtension {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: b}
}

// The permission check covers every definition an Application reaches, not only those it names
// inline: a Workflow it references, Policy objects its deploy steps load, the types and traits an
// override sets. Otherwise a tenant denied a definition reaches it anyway, and the controller runs
// it with its own identity.
func TestDefinitionPermissionsReachIndirectUses(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ValidateDefinitionPermissions, true)
	oldWithUser := auth.AuthenticationWithUser
	auth.AuthenticationWithUser = true
	defer func() { auth.AuthenticationWithUser = oldWithUser }()

	sys := oam.SystemDefinitionNamespace
	defs := []client.Object{
		&v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Name: "webservice", Namespace: sys}},
		&v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Name: "priv-comp", Namespace: sys}},
		&v1beta1.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: "priv-trait", Namespace: sys}},
		&v1beta1.PolicyDefinition{ObjectMeta: metav1.ObjectMeta{Name: "priv-policy", Namespace: sys}},
		&v1beta1.PolicyDefinition{ObjectMeta: metav1.ObjectMeta{Name: "override", Namespace: sys}},
		&v1beta1.WorkflowStepDefinition{ObjectMeta: metav1.ObjectMeta{Name: "priv-step", Namespace: sys}},
		&v1beta1.WorkflowStepDefinition{ObjectMeta: metav1.ObjectMeta{Name: "deploy", Namespace: sys}},
		&wfTypesv1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: "tenant-wf", Namespace: "team-a"},
			Steps: []wfTypesv1alpha1.WorkflowStep{{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "s", Type: "priv-step"}}}},
		&v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{Name: "tenant-po", Namespace: "team-a"}, Type: "priv-policy"},
		&v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{Name: "tenant-override", Namespace: "team-a"}, Type: v1alpha1.OverridePolicyType,
			Properties: rawProps(t, v1alpha1.OverridePolicySpec{Components: []v1alpha1.EnvComponentPatch{{Name: "c", Type: "priv-comp"}}})},
	}
	handler := func(deniedObjects ...string) *ValidatingHandler {
		scheme := runtime.NewScheme()
		require.NoError(t, v1beta1.AddToScheme(scheme))
		require.NoError(t, v1alpha1.AddToScheme(scheme))
		require.NoError(t, wfTypesv1alpha1.AddToScheme(scheme))
		require.NoError(t, authv1.AddToScheme(scheme))
		denied := map[string]bool{
			"componentdefinitions/priv-comp": true, "traitdefinitions/priv-trait": true,
			"policydefinitions/priv-policy": true, "workflowstepdefinitions/priv-step": true,
		}
		for _, o := range deniedObjects {
			denied[o] = true
		}
		return &ValidatingHandler{Client: &gatekeeper{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(defs...).Build(), denied: denied}}
	}
	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: authenticationv1.UserInfo{Username: "tenant"}}}
	app := func(mutate func(*v1beta1.Application)) *v1beta1.Application {
		a := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "team-a"},
			Spec:       v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{Name: "c", Type: "webservice"}}},
		}
		mutate(a)
		return a
	}
	deploying := func(policies ...string) func(*v1beta1.Application) {
		return func(a *v1beta1.Application) {
			a.Spec.Workflow = &v1beta1.Workflow{Steps: []wfTypesv1alpha1.WorkflowStep{{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
				Name: "d", Type: "deploy", Properties: rawProps(t, map[string]interface{}{"policies": policies})}}}}
		}
	}
	refuses := func(t *testing.T, errs []string, field, says string) {
		t.Helper()
		for _, e := range errs {
			if strings.Contains(e, field) && strings.Contains(e, says) {
				return
			}
		}
		t.Fatalf("no error at %s saying %q in %v", field, says, errs)
	}
	check := func(h *ValidatingHandler, a *v1beta1.Application) []string {
		var out []string
		for _, e := range h.ValidateDefinitionPermissions(context.Background(), a, req) {
			out = append(out, e.Error())
		}
		return out
	}

	t.Run("a step of the Workflow workflow.ref names", func(t *testing.T) {
		errs := check(handler(), app(func(a *v1beta1.Application) { a.Spec.Workflow = &v1beta1.Workflow{Ref: "tenant-wf"} }))
		refuses(t, errs, "spec.workflow.ref", "priv-step")
	})
	t.Run("the type of a Policy object a deploy step loads", func(t *testing.T) {
		refuses(t, check(handler(), app(deploying("tenant-po"))), "spec.workflow.steps[0].properties.policies[0]", "priv-policy")
	})
	t.Run("a component type an external override Policy sets", func(t *testing.T) {
		refuses(t, check(handler(), app(deploying("tenant-override"))), "spec.workflow.steps[0].properties.policies[0]", "priv-comp")
	})
	t.Run("a trait an inline override policy sets", func(t *testing.T) {
		errs := check(handler(), app(func(a *v1beta1.Application) {
			a.Spec.Policies = []v1beta1.AppPolicy{{Name: "o", Type: v1alpha1.OverridePolicyType, Properties: rawProps(t, v1alpha1.OverridePolicySpec{
				Components: []v1alpha1.EnvComponentPatch{{Name: "c", Traits: []v1alpha1.EnvTraitPatch{{Type: "priv-trait"}}}}})}}
		}))
		refuses(t, errs, "spec.policies[0].properties.components[0].traits[0].type", "priv-trait")
	})
	t.Run("a referenced Workflow the user may not read", func(t *testing.T) {
		errs := check(handler("workflows/tenant-wf"), app(func(a *v1beta1.Application) { a.Spec.Workflow = &v1beta1.Workflow{Ref: "tenant-wf"} }))
		refuses(t, errs, "spec.workflow.ref", "tenant-wf")
	})
	t.Run("nothing denied, nothing refused", func(t *testing.T) {
		h := handler()
		g := h.Client.(*gatekeeper)
		g.denied = map[string]bool{}
		require.Empty(t, check(h, app(func(a *v1beta1.Application) { a.Spec.Workflow = &v1beta1.Workflow{Ref: "tenant-wf"} })))
		require.Empty(t, check(h, app(deploying("tenant-po", "tenant-override"))))
	})
}
