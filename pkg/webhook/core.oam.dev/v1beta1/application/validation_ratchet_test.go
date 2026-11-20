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
	"strings"
	"testing"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/kubevela/pkg/controller/sharding"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/module/naming"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// An Application a 1.11 webhook admitted with two traits 1.12 refuses together.
func TestValidateUpdateLetsAStoredProblemThroughAnUnchangedSpec(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ValidateDefinitionPermissions, false)
	// Sharding skips the rendering phase, which needs component definitions.
	oldSharding := sharding.EnableSharding
	sharding.EnableSharding = true
	t.Cleanup(func() { sharding.EnableSharding = oldSharding })

	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	handler := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "conflict-a", Namespace: oam.SystemDefinitionNamespace},
			Spec:       v1beta1.TraitDefinitionSpec{ConflictsWith: []string{"conflict-b"}},
		},
		&v1beta1.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: "conflict-b", Namespace: oam.SystemDefinitionNamespace}},
		&v1beta1.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: "scaler", Namespace: oam.SystemDefinitionNamespace}},
		&v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Name: "webservice", Namespace: oam.SystemDefinitionNamespace}},
	).Build()}
	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UserInfo: authenticationv1.UserInfo{Username: "gitops", Groups: []string{"system:authenticated"}},
	}}

	stored := func() *v1beta1.Application {
		return &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default"},
			Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{
				Name: "web", Type: "webservice",
				Traits: []common.ApplicationTrait{{Type: "conflict-a"}, {Type: "conflict-b"}},
			}}},
		}
	}

	cases := []struct {
		name     string
		update   func(*v1beta1.Application)
		refused  string
		warnedOf string
	}{
		{
			name:     "a label change is admitted, warning of the stored conflict",
			update:   func(a *v1beta1.Application) { a.Labels = map[string]string{"team": "shop"} },
			warnedOf: "conflicts with trait",
		},
		{
			name:     "a publishVersion bump is admitted, warning of the stored conflict",
			update:   func(a *v1beta1.Application) { a.Annotations = map[string]string{oam.AnnotationPublishVersion: "v2"} },
			warnedOf: "conflicts with trait",
		},
		{
			name:     "an identical update is admitted",
			update:   func(*v1beta1.Application) {},
			warnedOf: "conflicts with trait",
		},
		{
			name: "a spec edit is validated in full",
			update: func(a *v1beta1.Application) {
				a.Spec.Components[0].Traits = append(a.Spec.Components[0].Traits, common.ApplicationTrait{Type: "scaler"})
			},
			refused: "conflicts with trait",
		},
		{
			name: "a new metadata problem is refused, the stored one let through",
			update: func(a *v1beta1.Application) {
				a.Annotations = map[string]string{oam.AnnotationAutoUpdate: "true", oam.AnnotationPublishVersion: "v2"}
			},
			refused:  oam.AnnotationAutoUpdate,
			warnedOf: "conflicts with trait",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updated := stored()
			tc.update(updated)
			errs, warnings := handler.ValidateUpdate(context.Background(), updated, stored(), req)
			if tc.refused == "" {
				require.Empty(t, errs)
			} else {
				require.NotEmpty(t, errs)
				require.Contains(t, errs.ToAggregate().Error(), tc.refused)
				if tc.warnedOf != "" {
					require.NotContains(t, errs.ToAggregate().Error(), tc.warnedOf)
				}
			}
			if tc.warnedOf != "" {
				require.Contains(t, strings.Join(warnings, "\n"), tc.warnedOf)
			}
		})
	}
}

func TestWithoutStoredProblems(t *testing.T) {
	path := field.NewPath("spec")
	// Rendering errors carry the whole Application as their value, labels and all.
	newErr := field.Invalid(path, map[string]string{"labels": "team=shop"}, "component web does not render")
	storedErr := field.Invalid(path, map[string]string{"labels": ""}, "component web does not render")
	other := field.Invalid(path, nil, "component api does not render")
	stored := func() field.ErrorList { return field.ErrorList{storedErr} }

	refused, warnings := withoutStoredProblems(field.ErrorList{newErr, other}, true, stored)
	require.Equal(t, field.ErrorList{other}, refused)
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0], "component web does not render")

	refused, warnings = withoutStoredProblems(field.ErrorList{newErr}, false, stored)
	require.Equal(t, field.ErrorList{newErr}, refused, "a changed spec is validated in full")
	require.Empty(t, warnings)

	refused, _ = withoutStoredProblems(nil, true, func() field.ErrorList {
		t.Fatal("the stored Application is validated only when the update fails")
		return nil
	})
	require.Empty(t, refused)
}

// A definition a module removed is not a rule that changed: the Application can no longer render,
// so even an update that leaves its spec as it is gets validated in full and refused.
func TestValidateUpdateRefusesAnApplicationWhoseDefinitionIsGone(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ValidateDefinitionPermissions, false)
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	handler := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: oam.SystemDefinitionNamespace}},
		&v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Name: naming.DefinitionName("kit", "v1", "widget"), Namespace: oam.SystemDefinitionNamespace}},
		&v1beta1.SourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "config-map", Namespace: oam.SystemDefinitionNamespace}},
	).Build()}
	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UserInfo: authenticationv1.UserInfo{Username: "gitops", Groups: []string{"system:authenticated"}},
	}}
	app := func(compType, traitType string) *v1beta1.Application {
		a := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default"},
			Spec:       v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{Name: "web", Type: compType}}},
		}
		if traitType != "" {
			a.Spec.Components[0].Traits = []common.ApplicationTrait{{Type: traitType}}
		}
		return a
	}
	bump := func(a *v1beta1.Application) *v1beta1.Application {
		a.Annotations = map[string]string{oam.AnnotationPublishVersion: "v2"}
		return a
	}

	require.True(t, handler.usesMissingDefinition(context.Background(), app("gadget", "")), "a component type that is gone")
	require.False(t, handler.usesMissingDefinition(context.Background(), app("kit/v1/widget", "")), "a Form 3 reference to an installed definition")
	require.True(t, handler.usesMissingDefinition(context.Background(), app("kit/v1/gadget", "")), "a Form 3 reference whose definition is gone")
	withPolicy := app("worker", "")
	withPolicy.Spec.Policies = []v1beta1.AppPolicy{{Name: "t", Type: "topology"}, {Name: "c", Type: "custom-policy"}}
	require.True(t, handler.usesMissingDefinition(context.Background(), withPolicy), "a custom policy type that is gone")
	withPolicy.Spec.Policies = withPolicy.Spec.Policies[:1]
	require.False(t, handler.usesMissingDefinition(context.Background(), withPolicy), "a built-in policy type has no definition")
	withSteps := app("worker", "")
	withSteps.Spec.Workflow = &v1beta1.Workflow{Steps: []wfTypesv1alpha1.WorkflowStep{{
		WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "g", Type: "step-group"},
		SubSteps:         []wfTypesv1alpha1.WorkflowStepBase{{Name: "s", Type: "suspend"}},
	}}}
	require.False(t, handler.usesMissingDefinition(context.Background(), withSteps), "built-in step types have no definition")
	withSteps.Spec.Workflow.Steps[0].SubSteps = append(withSteps.Spec.Workflow.Steps[0].SubSteps, wfTypesv1alpha1.WorkflowStepBase{Name: "n", Type: "notify-team"})
	require.True(t, handler.usesMissingDefinition(context.Background(), withSteps), "a custom step type that is gone")
	withSource := app("worker", "")
	withSource.Spec.Sources = []v1beta1.ApplicationSource{{Name: "db", Type: "vault-secret"}}
	require.True(t, handler.usesMissingDefinition(context.Background(), withSource), "a source type that is gone")
	withSource.Spec.Sources[0].Type = "config-map"
	require.False(t, handler.usesMissingDefinition(context.Background(), withSource), "an installed source type")
	require.True(t, handler.usesMissingDefinition(context.Background(), app("worker", "gauge")), "a trait type that is gone")
	require.False(t, handler.usesMissingDefinition(context.Background(), app("worker", "")))

	errs, _ := handler.ValidateUpdate(context.Background(), bump(app("gadget", "")), app("gadget", ""), req)
	require.NotEmpty(t, errs, "a publishVersion bump of an Application whose definition is gone")
}
