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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func nsRestrictHandler(t *testing.T, objs ...client.Object) *ValidatingHandler {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	// A namespace selector reads the Application's Namespace.
	require.NoError(t, corev1.AddToScheme(scheme))
	return &ValidatingHandler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
	}
}

// restrictionErrs runs the check for a test that cares only about the errors.
// Warnings have their own tests.
func restrictionErrs(h *ValidatingHandler, app *v1beta1.Application) field.ErrorList {
	errs, _ := h.ValidateDefinitionRestrictions(context.Background(), app, nil)
	return errs
}

func restrictedCompDef(name, namespace string, patterns ...string) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       v1beta1.ComponentDefinitionSpec{Restrictions: nsRestrictions(patterns)},
	}
}

func appUsing(namespace, compType string) *v1beta1.Application {
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: namespace},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{Name: "web", Type: compType}},
		},
	}
}

func TestValidateDefinitionNamespaces(t *testing.T) {
	def := restrictedCompDef("webservice", oam.SystemDefinitionNamespace, "vela-system", "tenant-*")

	t.Run("an allowed namespace passes", func(t *testing.T) {
		h := nsRestrictHandler(t, def)
		assert.Empty(t, restrictionErrs(h, appUsing("tenant-a", "webservice")))
	})

	t.Run("a denied namespace reports the offending component", func(t *testing.T) {
		h := nsRestrictHandler(t, def)
		errs := restrictionErrs(h, appUsing("default", "webservice"))
		require.Len(t, errs, 1)
		assert.Equal(t, "spec.components[0].type", errs[0].Field)
		assert.Contains(t, errs[0].Detail, "ComponentDefinition")
		assert.Contains(t, errs[0].Detail, "default")
		// The allowed namespaces are the operator's business, not the author's.
		assert.NotContains(t, errs[0].Detail, "tenant-*")
		assert.NotContains(t, errs[0].Detail, "vela-system")
	})

	t.Run("an unrestricted definition passes", func(t *testing.T) {
		h := nsRestrictHandler(t, restrictedCompDef("webservice", oam.SystemDefinitionNamespace))
		assert.Empty(t, restrictionErrs(h, appUsing("default", "webservice")))
	})

	// The annotation is the channel for a definition whose spec helm owns.
	t.Run("the annotation restricts just as the spec does", func(t *testing.T) {
		annotated := restrictedCompDef("webservice", oam.SystemDefinitionNamespace)
		annotated.Annotations = map[string]string{oam.AnnotationRestrictNamespaces: "tenant-*"}
		h := nsRestrictHandler(t, annotated)
		assert.Len(t, restrictionErrs(h, appUsing("default", "webservice")), 1)
		assert.Empty(t, restrictionErrs(h, appUsing("tenant-a", "webservice")))
	})

	// ValidateComponents reports a missing definition.
	t.Run("a missing definition is not this check's to report", func(t *testing.T) {
		h := nsRestrictHandler(t)
		assert.Empty(t, restrictionErrs(h, appUsing("default", "nonexistent")))
	})

	// A pinned type renders from a frozen revision, but the live restriction
	// applies.
	t.Run("a pinned type is checked against the live definition", func(t *testing.T) {
		h := nsRestrictHandler(t, def)
		errs := restrictionErrs(h, appUsing("default", "webservice@v1"))
		require.Len(t, errs, 1)
		assert.Equal(t, "spec.components[0].type", errs[0].Field)
	})

	// A restriction frozen into a DefinitionRevision must not revive one that has
	// since been cleared.
	t.Run("a frozen revision never supplies the restriction", func(t *testing.T) {
		frozen := &v1beta1.DefinitionRevision{
			ObjectMeta: metav1.ObjectMeta{Name: "webservice-v1", Namespace: oam.SystemDefinitionNamespace},
			Spec: v1beta1.DefinitionRevisionSpec{
				DefinitionType:      common.ComponentType,
				ComponentDefinition: *restrictedCompDef("webservice", oam.SystemDefinitionNamespace, "tenant-*"),
			},
		}
		live := restrictedCompDef("webservice", oam.SystemDefinitionNamespace)
		h := nsRestrictHandler(t, live, frozen)
		assert.Empty(t, restrictionErrs(h, appUsing("default", "webservice@v1")))
	})

	t.Run("a definition in the app's own namespace is checked too", func(t *testing.T) {
		local := restrictedCompDef("local-type", "tenant-a", "tenant-b")
		h := nsRestrictHandler(t, local)
		assert.Len(t, restrictionErrs(h, appUsing("tenant-a", "local-type")), 1)
	})

	t.Run("every use of the definition is reported", func(t *testing.T) {
		h := nsRestrictHandler(t, def)
		app := appUsing("default", "webservice")
		app.Spec.Components = append(app.Spec.Components,
			common.ApplicationComponent{Name: "api", Type: "webservice"})
		errs := restrictionErrs(h, app)
		require.Len(t, errs, 2)
		fields := []string{errs[0].Field, errs[1].Field}
		assert.ElementsMatch(t, []string{"spec.components[0].type", "spec.components[1].type"}, fields)
	})

	t.Run("the feature gate turns it off", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
			features.RestrictDefinitionNamespaces, false)
		h := nsRestrictHandler(t, def)
		assert.Empty(t, restrictionErrs(h, appUsing("default", "webservice")))
	})
}

func TestValidateDefinitionNamespacesAcrossKinds(t *testing.T) {
	objs := []client.Object{
		&v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "scaler", Namespace: oam.SystemDefinitionNamespace},
			Spec:       v1beta1.TraitDefinitionSpec{Restrictions: nsRestrictions([]string{"tenant-*"})},
		},
		&v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "restricted-policy", Namespace: oam.SystemDefinitionNamespace},
			Spec:       v1beta1.PolicyDefinitionSpec{Restrictions: nsRestrictions([]string{"tenant-*"})},
		},
		&v1beta1.WorkflowStepDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "restricted-step", Namespace: oam.SystemDefinitionNamespace},
			Spec:       v1beta1.WorkflowStepDefinitionSpec{Restrictions: nsRestrictions([]string{"tenant-*"})},
		},
		&v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "restricted-source", Namespace: oam.SystemDefinitionNamespace},
			Spec:       v1beta1.SourceDefinitionSpec{Restrictions: nsRestrictions([]string{"tenant-*"})},
		},
	}
	h := nsRestrictHandler(t, objs...)

	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: "default"},
		Spec: v1beta1.ApplicationSpec{
			Sources: []v1beta1.ApplicationSource{{Name: "src", Type: "restricted-source"}},
			Components: []common.ApplicationComponent{{
				Name:   "web",
				Type:   "unrestricted",
				Traits: []common.ApplicationTrait{{Type: "scaler"}},
			}},
			Policies: []v1beta1.AppPolicy{{Name: "p", Type: "restricted-policy"}},
			Workflow: &v1beta1.Workflow{Steps: []wfTypesv1alpha1.WorkflowStep{{
				WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "s", Type: "restricted-step"},
				SubSteps: []wfTypesv1alpha1.WorkflowStepBase{
					{Name: "sub", Type: "restricted-step"},
				},
			}}},
		},
	}

	errs := restrictionErrs(h, app)
	fields := make([]string, 0, len(errs))
	for _, e := range errs {
		fields = append(fields, e.Field)
	}
	assert.ElementsMatch(t, []string{
		"spec.components[0].traits[0].type",
		"spec.policies[0].type",
		"spec.workflow.steps[0].type",
		"spec.workflow.steps[0].subSteps[0].type",
		"spec.sources[0].type",
	}, fields)
}

// A builtin step has no WorkflowStepDefinition, so it must not be looked up or
// reported.
func TestValidateDefinitionNamespacesSkipsBuiltinSteps(t *testing.T) {
	h := nsRestrictHandler(t)
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: "default"},
		Spec: v1beta1.ApplicationSpec{
			Workflow: &v1beta1.Workflow{Steps: []wfTypesv1alpha1.WorkflowStep{{
				WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "s", Type: "suspend"},
			}}},
		},
	}
	assert.Empty(t, restrictionErrs(h, app))
}

// nsRestrictions builds the spec block these tests vary; nil means the
// definition declares nothing.
func nsRestrictions(patterns []string) *common.DefinitionRestrictions {
	if patterns == nil {
		return nil
	}
	return &common.DefinitionRestrictions{Namespaces: patterns}
}

func labelledNamespace(name string, labels map[string]string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
}

func selectorCompDef(name, namespace string, sel *metav1.LabelSelector, patterns ...string) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1beta1.ComponentDefinitionSpec{Restrictions: &common.DefinitionRestrictions{
			Namespaces: patterns, NamespaceSelector: sel,
		}},
	}
}

// A selector matches namespaces whose membership is not in their name.
func TestValidateDefinitionNamespacesBySelector(t *testing.T) {
	tenantSel := &metav1.LabelSelector{MatchLabels: map[string]string{"tenant": "true"}}

	t.Run("a labelled namespace is allowed whatever it is called", func(t *testing.T) {
		h := nsRestrictHandler(t,
			selectorCompDef("webservice", oam.SystemDefinitionNamespace, tenantSel),
			labelledNamespace("acme-prod", map[string]string{"tenant": "true"}))
		assert.Empty(t, restrictionErrs(h, appUsing("acme-prod", "webservice")))
	})

	t.Run("an unlabelled namespace is denied without naming the selector", func(t *testing.T) {
		h := nsRestrictHandler(t,
			selectorCompDef("webservice", oam.SystemDefinitionNamespace, tenantSel),
			labelledNamespace("default", nil))
		errs := restrictionErrs(h, appUsing("default", "webservice"))
		require.Len(t, errs, 1)
		assert.Equal(t, "spec.components[0].type", errs[0].Field)
		assert.NotContains(t, errs[0].Detail, "tenant=true", "the selector must not reach the author")
	})

	// The fields are alternatives, so a name glob admits a namespace the selector
	// rejects.
	t.Run("the name glob still admits a namespace the selector rejects", func(t *testing.T) {
		h := nsRestrictHandler(t,
			selectorCompDef("webservice", oam.SystemDefinitionNamespace, tenantSel, "vela-system"),
			labelledNamespace("vela-system", nil))
		assert.Empty(t, restrictionErrs(h, appUsing("vela-system", "webservice")))
	})

	// nil labels mean the namespace could not be read, which must not widen access.
	t.Run("an unreadable namespace denies a selector", func(t *testing.T) {
		h := nsRestrictHandler(t, selectorCompDef("webservice", oam.SystemDefinitionNamespace, tenantSel))
		assert.Len(t, restrictionErrs(h, appUsing("missing-ns", "webservice")), 1)
	})

	// The namespace is read only when something selects on labels.
	t.Run("a name-only restriction never reads the namespace", func(t *testing.T) {
		h := nsRestrictHandler(t, restrictedCompDef("webservice", oam.SystemDefinitionNamespace, "tenant-*"))
		// No Namespace object exists in the fake client at all; a lookup would
		// surface as a denial for tenant-a.
		assert.Empty(t, restrictionErrs(h, appUsing("tenant-a", "webservice")))
	})
}

// A namespace that cannot be read is a server-side failure, not a policy
// violation, and must not be reported as one.
func TestValidateDefinitionNamespacesReportsNamespaceReadFailure(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	def := selectorCompDef("webservice", oam.SystemDefinitionNamespace,
		&metav1.LabelSelector{MatchLabels: map[string]string{"tenant": "true"}})

	h := &ValidatingHandler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(def).Build(),
		APIReader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(def).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*corev1.Namespace); ok {
						return apierrors.NewInternalError(errors.New("informer not synced"))
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}).Build(),
	}

	errs := restrictionErrs(h, appUsing("default", "webservice"))
	require.Len(t, errs, 1)
	assert.Equal(t, field.ErrorTypeInternal, errs[0].Type,
		"a read failure must not be reported as a policy violation")
	assert.Contains(t, errs[0].Detail, "informer not synced")
	assert.Contains(t, errs[0].Detail, "ComponentDefinition", "the message has to name the kind")
	assert.NotContains(t, errs[0].Detail, "restricted to")
}
