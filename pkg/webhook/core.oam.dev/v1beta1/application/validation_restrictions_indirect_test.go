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

	wfv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/module/naming"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func overridePolicy(t *testing.T, spec v1alpha1.OverridePolicySpec) v1beta1.AppPolicy {
	raw, err := json.Marshal(spec)
	require.NoError(t, err)
	return v1beta1.AppPolicy{Name: "override", Type: v1alpha1.OverridePolicyType, Properties: &runtime.RawExtension{Raw: raw}}
}

// A restriction binds every definition an Application uses, however it names it: inline, through
// an override policy, by its module name, or in a Workflow its workflow.ref names.
func TestValidateDefinitionNamespacesThroughIndirection(t *testing.T) {
	restrictedTrait := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "privileged", Namespace: oam.SystemDefinitionNamespace},
		Spec:       v1beta1.TraitDefinitionSpec{Restrictions: nsRestrictions([]string{"platform"})},
	}
	restrictedStep := &v1beta1.WorkflowStepDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-step", Namespace: oam.SystemDefinitionNamespace},
		Spec:       v1beta1.WorkflowStepDefinitionSpec{Restrictions: nsRestrictions([]string{"platform"})},
	}

	t.Run("a component type set by an override policy", func(t *testing.T) {
		h := nsRestrictHandler(t, restrictedCompDef("admin-only", oam.SystemDefinitionNamespace, "platform"))
		app := appUsing("tenant-a", "webservice")
		app.Spec.Policies = []v1beta1.AppPolicy{overridePolicy(t, v1alpha1.OverridePolicySpec{
			Components: []v1alpha1.EnvComponentPatch{{Name: "web", Type: "admin-only"}},
		})}
		errs := restrictionErrs(h, app)
		require.Len(t, errs, 1)
		assert.Equal(t, "spec.policies[0].properties.components[0].type", errs[0].Field)
	})

	t.Run("a trait an override policy adds", func(t *testing.T) {
		h := nsRestrictHandler(t, restrictedTrait)
		app := appUsing("tenant-a", "webservice")
		app.Spec.Policies = []v1beta1.AppPolicy{overridePolicy(t, v1alpha1.OverridePolicySpec{
			Components: []v1alpha1.EnvComponentPatch{{Name: "web", Traits: []v1alpha1.EnvTraitPatch{{Type: "privileged"}}}},
		})}
		errs := restrictionErrs(h, app)
		require.Len(t, errs, 1)
		assert.Equal(t, "spec.policies[0].properties.components[0].traits[0].type", errs[0].Field)
	})

	t.Run("a module type, named by module, version and name", func(t *testing.T) {
		installed := restrictedCompDef(naming.DefinitionName("storage", "v1", "bucket"), oam.SystemDefinitionNamespace, "platform")
		h := nsRestrictHandler(t, installed)
		errs := restrictionErrs(h, appUsing("tenant-a", "storage/v1/bucket"))
		require.Len(t, errs, 1)
		assert.Equal(t, "spec.components[0].type", errs[0].Field)
	})

	t.Run("a step of the Workflow workflow.ref names", func(t *testing.T) {
		wf := &wfv1alpha1.Workflow{
			ObjectMeta:   metav1.ObjectMeta{Name: "shared-release", Namespace: "tenant-a"},
			WorkflowSpec: wfv1alpha1.WorkflowSpec{Steps: []wfv1alpha1.WorkflowStep{{WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: "go", Type: "admin-step"}}}},
		}
		h := nsRestrictHandlerWithWorkflows(t, restrictedStep, wf)
		app := appUsing("tenant-a", "webservice")
		app.Spec.Workflow = &v1beta1.Workflow{Ref: "shared-release"}
		errs := restrictionErrs(h, app)
		require.Len(t, errs, 1)
		assert.Equal(t, "spec.workflow.ref", errs[0].Field)
	})
}

// A trait that does not declare conflictsWith inherits its parent's, as rendering does.
func TestValidateTraitConflictsInheritsThroughExtends(t *testing.T) {
	h := nsRestrictHandler(t,
		&v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "expose", Namespace: oam.SystemDefinitionNamespace},
			Spec:       v1beta1.TraitDefinitionSpec{ConflictsWith: []string{"gateway"}},
		},
		&v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "expose-internal", Namespace: oam.SystemDefinitionNamespace},
			Spec:       v1beta1.TraitDefinitionSpec{Extends: "expose"},
		},
		&v1beta1.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: oam.SystemDefinitionNamespace}},
	)
	app := appUsing("default", "webservice")
	app.Spec.Components[0].Traits = []common.ApplicationTrait{{Type: "expose-internal"}, {Type: "gateway"}}
	errs := h.ValidateTraitConflicts(context.Background(), app)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Detail, `"expose-internal" conflicts with trait "gateway"`)
}

// nsRestrictHandlerWithWorkflows is nsRestrictHandler with the Workflow kind registered.
func nsRestrictHandlerWithWorkflows(t *testing.T, objs ...client.Object) *ValidatingHandler {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, wfv1alpha1.AddToScheme(scheme))
	return &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()}
}

// The controller reports restricted uses an Application already has; a quota is a budget for
// new uses, so it is not reported there.
func TestRestrictionViolationsReportsRestrictionsNotQuotas(t *testing.T) {
	restricted := restrictedCompDef("admin-only", oam.SystemDefinitionNamespace, "platform")
	full := quotaCompDef("webservice", common.NamespaceQuota{Limit: i32(0)})
	h := nsRestrictHandler(t, restricted, full)
	app := appUsing("tenant-a", "webservice")
	app.Spec.Policies = []v1beta1.AppPolicy{overridePolicy(t, v1alpha1.OverridePolicySpec{
		Components: []v1alpha1.EnvComponentPatch{{Name: "web", Type: "admin-only"}},
	})}

	errs := RestrictionViolations(context.Background(), h.Client, app)
	require.Len(t, errs, 1)
	assert.Equal(t, "spec.policies[0].properties.components[0].type", errs[0].Field)
}
