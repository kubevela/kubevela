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
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestAppliesToRuleMatches(t *testing.T) {
	webservice := appliesToTarget{definition: "webservice", workload: "deployments.apps"}
	ingress := appliesToTarget{definition: "gateway", workload: "ingresses.networking.k8s.io"}
	cronTask := appliesToTarget{definition: "cron-task"}

	cases := []struct {
		name   string
		rules  []string
		target appliesToTarget
		want   bool
	}{
		{"empty list applies to anything", nil, webservice, true},
		{"wildcard", []string{"*"}, webservice, true},
		{"workload match", []string{"deployments.apps"}, webservice, true},
		{"workload miss", []string{"statefulsets.apps"}, webservice, false},
		{"any entry may match", []string{"statefulsets.apps", "deployments.apps"}, webservice, true},
		{"group wildcard match", []string{"*.apps"}, webservice, true},
		{"group wildcard miss", []string{"*.batch"}, webservice, false},
		{"group wildcard does not match a nested group suffix", []string{"*.k8s.io"}, ingress, false},
		{"group wildcard matches a nested group exactly", []string{"*.networking.k8s.io"}, ingress, true},
		{"definition name match", []string{"webservice"}, webservice, true},
		{"definition name miss", []string{"worker"}, webservice, false},
		{"version suffix is ignored", []string{"deployments.apps/v1"}, webservice, true},
		{"version suffix on a miss", []string{"statefulsets.apps/v1"}, webservice, false},
		{"no workload: workload form is not checked", []string{"cronjobs.batch"}, cronTask, true},
		{"no workload: group wildcard is not checked", []string{"*.apps"}, cronTask, true},
		{"no workload: definition name match", []string{"cron-task"}, cronTask, true},
		{"no workload: definition name miss", []string{"webservice"}, cronTask, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, traitAppliesTo(tc.rules, tc.target))
		})
	}
}

func appliesToComponentDef(name string, workload common.WorkloadTypeDescriptor) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: oam.SystemDefinitionNamespace},
		Spec: v1beta1.ComponentDefinitionSpec{
			Workload:  workload,
			Schematic: &common.Schematic{CUE: &common.CUE{Template: "output: {}"}},
		},
	}
}

func appliesToTraitDef(name string, appliesTo ...string) *v1beta1.TraitDefinition {
	return &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: oam.SystemDefinitionNamespace},
		Spec: v1beta1.TraitDefinitionSpec{
			AppliesToWorkloads: appliesTo,
			Schematic:          &common.Schematic{CUE: &common.CUE{Template: "patch: {}"}},
		},
	}
}

func appliesToApp(components ...common.ApplicationComponent) *v1beta1.Application {
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: "default"},
		Spec:       v1beta1.ApplicationSpec{Components: components},
	}
}

func component(name, compType string, traitTypes ...string) common.ApplicationComponent {
	comp := common.ApplicationComponent{Name: name, Type: compType}
	for _, tt := range traitTypes {
		comp.Traits = append(comp.Traits, common.ApplicationTrait{Type: tt})
	}
	return comp
}

// appliesToRESTMapper maps the GVKs these tests use.
func appliesToRESTMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, meta.RESTScopeNamespace)
	return mapper
}

// unavailableRESTMapper fails like a mapper whose discovery is down, which is
// not a NoKindMatch.
type unavailableRESTMapper struct{ meta.RESTMapper }

func (unavailableRESTMapper) RESTMapping(schema.GroupKind, ...string) (*meta.RESTMapping, error) {
	return nil, fmt.Errorf("discovery unavailable")
}

// appliesToClient returns a fake client serving objects, intercepted by funcs if set.
func appliesToClient(funcs *interceptor.Funcs, objects ...client.Object) client.Client {
	b := fake.NewClientBuilder().WithScheme(readKindsScheme()).WithRESTMapper(appliesToRESTMapper()).WithObjects(objects...)
	if funcs != nil {
		b = b.WithInterceptorFuncs(*funcs)
	}
	return b.Build()
}

func appliesToHandler(objects ...client.Object) *ValidatingHandler {
	return &ValidatingHandler{Client: appliesToClient(nil, objects...)}
}

// appliesToErrs validates app as a create.
func appliesToErrs(h *ValidatingHandler, app *v1beta1.Application) field.ErrorList {
	errs, _ := h.ValidateTraitAppliesTo(context.Background(), app, nil)
	return errs
}

func withAppliesToValidation(t *testing.T) {
	t.Helper()
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ValidateTraitAppliesToWorkloads, true)
}

func TestValidateTraitAppliesTo(t *testing.T) {
	deployment := common.WorkloadTypeDescriptor{Type: "deployments.apps"}
	autodetect := common.WorkloadTypeDescriptor{Type: types.AutoDetectWorkloadDefinition}
	definitions := []client.Object{
		appliesToComponentDef("webservice", deployment),
		appliesToComponentDef("cron-task", autodetect),
		appliesToComponentDef("gvk-only", common.WorkloadTypeDescriptor{
			Definition: common.WorkloadGVK{APIVersion: "apps/v1", Kind: "StatefulSet"},
		}),
		appliesToTraitDef("sts-only", "statefulsets.apps"),
		appliesToTraitDef("apps-group", "*.apps"),
		appliesToTraitDef("webservice-only", "webservice"),
		appliesToTraitDef("cronjob-scaler", "cronjobs.batch"),
		appliesToTraitDef("anywhere"),
		appliesToComponentDef("wd-backed", common.WorkloadTypeDescriptor{Type: "my-deploy"}),
		appliesToComponentDef("wd-missing", common.WorkloadTypeDescriptor{Type: "statefulsets.apps"}),
		appliesToComponentDef("wd-noref", common.WorkloadTypeDescriptor{Type: "bare-wd"}),
		&v1beta1.WorkloadDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "bare-wd", Namespace: oam.SystemDefinitionNamespace},
		},
		// What the ComponentDefinition mutating webhook stores for a GVK.
		appliesToComponentDef("gvk-and-type", common.WorkloadTypeDescriptor{
			Definition: common.WorkloadGVK{APIVersion: "apps/v1", Kind: "StatefulSet"},
			Type:       "statefulsets.apps",
		}),
		appliesToComponentDef("no-workload", common.WorkloadTypeDescriptor{}),
		&v1beta1.WorkloadDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "my-deploy", Namespace: oam.SystemDefinitionNamespace},
			Spec:       v1beta1.WorkloadDefinitionSpec{Reference: common.DefinitionReference{Name: "deployments.apps"}},
		},
	}

	t.Run("does nothing while the gate is off", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ValidateTraitAppliesToWorkloads, false)
		app := appliesToApp(component("web", "webservice", "sts-only"))
		assert.Empty(t, appliesToErrs(appliesToHandler(definitions...), app))
	})

	t.Run("rejects a trait whose appliesToWorkloads does not list the workload", func(t *testing.T) {
		withAppliesToValidation(t)
		app := appliesToApp(component("web", "webservice", "anywhere", "sts-only"))

		errs := appliesToErrs(appliesToHandler(definitions...), app)

		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInvalid, errs[0].Type)
		assert.Equal(t, "spec.components[0].traits[1].type", errs[0].Field)
		assert.Equal(t, fmt.Sprintf(errFmtTraitNotAppliable, "sts-only", "web", "webservice", "deployments.apps", []string{"statefulsets.apps"}), errs[0].Detail)
	})

	t.Run("allows traits that apply", func(t *testing.T) {
		withAppliesToValidation(t)
		app := appliesToApp(
			component("web", "webservice", "anywhere", "apps-group", "webservice-only"),
			component("db", "gvk-only", "sts-only", "apps-group"),
			component("no-traits", "webservice"),
		)
		assert.Empty(t, appliesToErrs(appliesToHandler(definitions...), app))
	})

	t.Run("reports every component that breaks the rule", func(t *testing.T) {
		withAppliesToValidation(t)
		app := appliesToApp(
			component("a", "webservice", "sts-only"),
			component("b", "webservice", "sts-only"),
			component("c", "gvk-only", "webservice-only"),
		)

		errs := appliesToErrs(appliesToHandler(definitions...), app)

		require.Len(t, errs, 3)
		assert.Equal(t, "spec.components[2].traits[0].type", errs[2].Field)
	})

	t.Run("maps a GVK-only workload through the RESTMapper", func(t *testing.T) {
		withAppliesToValidation(t)
		app := appliesToApp(component("db", "gvk-only", "webservice-only"))

		errs := appliesToErrs(appliesToHandler(definitions...), app)

		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, `(workload "statefulsets.apps")`)
	})

	t.Run("treats a kind the cluster does not serve as having no workload type", func(t *testing.T) {
		withAppliesToValidation(t)
		unserved := appliesToComponentDef("widget", common.WorkloadTypeDescriptor{
			Definition: common.WorkloadGVK{APIVersion: "example.com/v1", Kind: "Widget"},
		})
		app := appliesToApp(component("w", "widget", "sts-only"))

		assert.Empty(t, appliesToErrs(appliesToHandler(append(definitions, unserved)...), app))
	})

	t.Run("fails closed when the RESTMapper cannot answer", func(t *testing.T) {
		withAppliesToValidation(t)
		cli := fake.NewClientBuilder().WithScheme(readKindsScheme()).
			WithRESTMapper(unavailableRESTMapper{appliesToRESTMapper()}).WithObjects(definitions...).Build()
		app := appliesToApp(component("db", "gvk-only", "sts-only"))

		errs := appliesToErrs(&ValidatingHandler{Client: cli}, app)

		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInternal, errs[0].Type)
		assert.Equal(t, "spec.components[0].type", errs[0].Field)
	})

	t.Run("reads the workload from the WorkloadDefinition spec.workload.type names", func(t *testing.T) {
		withAppliesToValidation(t)
		app := appliesToApp(
			component("web", "wd-backed", "apps-group", "sts-only"),
			component("db", "wd-missing", "sts-only"),
		)

		errs := appliesToErrs(appliesToHandler(definitions...), app)

		// my-deploy refers to deployments.apps; with no WorkloadDefinition the type itself is used.
		require.Len(t, errs, 1)
		assert.Equal(t, "spec.components[0].traits[1].type", errs[0].Field)
		assert.Contains(t, errs[0].Detail, `(workload "deployments.apps")`)
	})

	t.Run("does not check workload forms on a component with no workload type", func(t *testing.T) {
		withAppliesToValidation(t)
		app := appliesToApp(component("job", "cron-task", "cronjob-scaler", "sts-only", "apps-group"))
		assert.Empty(t, appliesToErrs(appliesToHandler(definitions...), app))
	})

	t.Run("still checks a definition name on a component with no workload type", func(t *testing.T) {
		withAppliesToValidation(t)
		app := appliesToApp(component("job", "cron-task", "webservice-only"))

		errs := appliesToErrs(appliesToHandler(definitions...), app)

		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, `(workload "")`)
	})

	t.Run("skips definitions that do not exist", func(t *testing.T) {
		withAppliesToValidation(t)
		app := appliesToApp(
			component("web", "webservice", "missing-trait"),
			component("other", "missing-component", "sts-only"),
		)
		assert.Empty(t, appliesToErrs(appliesToHandler(definitions...), app))
	})

	t.Run("prefers a namespaced TraitDefinition over the system one", func(t *testing.T) {
		withAppliesToValidation(t)
		override := appliesToTraitDef("sts-only", "deployments.apps")
		override.Namespace = "default"
		app := appliesToApp(component("web", "webservice", "sts-only"))

		assert.Empty(t, appliesToErrs(appliesToHandler(append(definitions, override)...), app))
	})

	t.Run("reads a pinned definition revision", func(t *testing.T) {
		withAppliesToValidation(t)
		rev := &v1beta1.DefinitionRevision{
			ObjectMeta: metav1.ObjectMeta{Name: "anywhere-v1", Namespace: oam.SystemDefinitionNamespace},
			Spec: v1beta1.DefinitionRevisionSpec{
				Revision:        1,
				DefinitionType:  common.TraitType,
				TraitDefinition: *appliesToTraitDef("anywhere", "statefulsets.apps"),
			},
		}
		app := appliesToApp(component("web", "webservice", "anywhere@v1", "anywhere"))

		errs := appliesToErrs(appliesToHandler(append(definitions, rev)...), app)

		// v1 was restricted; the current definition is not.
		require.Len(t, errs, 1)
		assert.Equal(t, "spec.components[0].traits[0].type", errs[0].Field)
	})

	failingClient := func(failOn func(client.Object) bool) client.Client {
		return appliesToClient(&interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if failOn(obj) {
					return apierrors.NewServiceUnavailable("apiserver unavailable")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}, definitions...)
	}

	t.Run("leaves a type that cannot be resolved to the abstract-type check", func(t *testing.T) {
		withAppliesToValidation(t)
		cli := failingClient(func(obj client.Object) bool {
			_, ok := obj.(*v1beta1.TraitDefinition)
			return ok
		})
		h := &ValidatingHandler{Client: cli}
		app := appliesToApp(component("web", "webservice", "anywhere"))

		// ValidateAbstractTypes refuses it, so this check doesn't repeat it.
		require.Len(t, h.ValidateAbstractTypes(context.Background(), app), 1)
		assert.Empty(t, appliesToErrs(h, app))
	})

	t.Run("reports a failing type once", func(t *testing.T) {
		withAppliesToValidation(t)
		calls := 0
		cli := failingClient(func(obj client.Object) bool {
			_, ok := obj.(*v1beta1.DefinitionRevision)
			if ok {
				calls++
			}
			return ok
		})
		app := appliesToApp(
			component("a", "webservice@v1", "anywhere"),
			component("b", "webservice@v1", "anywhere"),
		)

		errs := appliesToErrs(&ValidatingHandler{Client: cli}, app)

		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInternal, errs[0].Type)
		assert.Equal(t, "spec.components[0].type", errs[0].Field)
		assert.Equal(t, 1, calls)
	})

	t.Run("does not repeat a malformed type", func(t *testing.T) {
		withAppliesToValidation(t)
		app := appliesToApp(component("web", "webservice", "foo/bar"))
		assert.Empty(t, appliesToErrs(appliesToHandler(definitions...), app))
	})

	t.Run("reads a WorkloadDefinition from the ComponentDefinition's namespace", func(t *testing.T) {
		withAppliesToValidation(t)
		// Same name as vela-system/my-deploy, in the Application's namespace.
		shadow := &v1beta1.WorkloadDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "my-deploy", Namespace: "default"},
			Spec:       v1beta1.WorkloadDefinitionSpec{Reference: common.DefinitionReference{Name: "statefulsets.apps"}},
		}
		app := appliesToApp(component("web", "wd-backed", "sts-only"))

		errs := appliesToErrs(appliesToHandler(append(definitions, shadow)...), app)

		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, `(workload "deployments.apps")`)
	})

	t.Run("reads a cluster-scoped ComponentDefinition's WorkloadDefinition from vela-system", func(t *testing.T) {
		withAppliesToValidation(t)
		legacy := appliesToComponentDef("legacy-wd", common.WorkloadTypeDescriptor{Type: "my-deploy"})
		legacy.Namespace = ""
		app := appliesToApp(component("web", "legacy-wd", "sts-only"))

		errs := appliesToErrs(appliesToHandler(append(definitions, legacy)...), app)

		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, `(workload "deployments.apps")`)
	})

	t.Run("uses spec.workload.type when a GVK is set as well", func(t *testing.T) {
		withAppliesToValidation(t)
		app := appliesToApp(component("db", "gvk-and-type", "sts-only", "webservice-only"))

		errs := appliesToErrs(appliesToHandler(definitions...), app)

		require.Len(t, errs, 1)
		assert.Equal(t, "spec.components[0].traits[1].type", errs[0].Field)
		assert.Contains(t, errs[0].Detail, `(workload "statefulsets.apps")`)
	})

	t.Run("a WorkloadDefinition without a definitionRef stands for its own name", func(t *testing.T) {
		withAppliesToValidation(t)
		app := appliesToApp(component("web", "wd-noref", "webservice-only"))

		errs := appliesToErrs(appliesToHandler(definitions...), app)

		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, `(workload "bare-wd")`)
	})

	t.Run("a component that declares no workload has no workload type", func(t *testing.T) {
		withAppliesToValidation(t)
		app := appliesToApp(component("x", "no-workload", "sts-only", "webservice-only"))

		errs := appliesToErrs(appliesToHandler(definitions...), app)

		require.Len(t, errs, 1)
		assert.Equal(t, "spec.components[0].traits[1].type", errs[0].Field)
		assert.Contains(t, errs[0].Detail, `(workload "")`)
	})

	t.Run("fails closed when the WorkloadDefinition cannot be read", func(t *testing.T) {
		withAppliesToValidation(t)
		cli := failingClient(func(obj client.Object) bool {
			_, ok := obj.(*v1beta1.WorkloadDefinition)
			return ok
		})
		app := appliesToApp(component("web", "wd-backed", "sts-only"))

		errs := appliesToErrs(&ValidatingHandler{Client: cli}, app)

		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInternal, errs[0].Type)
		assert.Equal(t, "spec.components[0].type", errs[0].Field)
	})

	t.Run("fails closed when a definition cannot be read", func(t *testing.T) {
		withAppliesToValidation(t)
		cli := failingClient(func(obj client.Object) bool {
			_, ok := obj.(*v1beta1.DefinitionRevision)
			return ok
		})
		app := appliesToApp(component("web", "webservice", "anywhere@v1"))

		errs := appliesToErrs(&ValidatingHandler{Client: cli}, app)

		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInternal, errs[0].Type)
		assert.Equal(t, "spec.components[0].traits[0].type", errs[0].Field)
	})

	t.Run("reads a definition the cache has not caught up with from the API server", func(t *testing.T) {
		withAppliesToValidation(t)
		late := appliesToTraitDef("late-sts-only", "statefulsets.apps")
		h := &ValidatingHandler{
			Client: appliesToClient(nil, definitions...),
			Live:   appliesToClient(nil, append(definitions, late)...),
		}
		app := appliesToApp(component("web", "webservice", "late-sts-only"))

		errs := appliesToErrs(h, app)

		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, `trait "late-sts-only"`)
	})
}

func TestValidateTraitAppliesToOnUpdate(t *testing.T) {
	withAppliesToValidation(t)
	h := appliesToHandler(
		appliesToComponentDef("webservice", common.WorkloadTypeDescriptor{Type: "deployments.apps"}),
		appliesToComponentDef("worker", common.WorkloadTypeDescriptor{Type: "deployments.apps"}),
		appliesToTraitDef("sts-only", "statefulsets.apps"),
		appliesToTraitDef("anywhere"),
	)
	oldApp := appliesToApp(component("web", "webservice", "sts-only"))

	t.Run("a trait the Application already had is admitted with a warning", func(t *testing.T) {
		newApp := appliesToApp(component("web", "webservice", "anywhere", "sts-only"))

		errs, warnings := h.ValidateTraitAppliesTo(context.Background(), newApp, oldApp)

		assert.Empty(t, errs)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], `trait "sts-only" cannot apply to component "web"`)
		assert.Contains(t, warnings[0], "allowed because the Application already had it")
	})

	t.Run("the same trait on a new component is refused", func(t *testing.T) {
		newApp := appliesToApp(
			component("web", "webservice", "sts-only"),
			component("api", "webservice", "sts-only"),
		)

		errs, warnings := h.ValidateTraitAppliesTo(context.Background(), newApp, oldApp)

		require.Len(t, errs, 1)
		assert.Equal(t, "spec.components[1].traits[0].type", errs[0].Field)
		assert.Len(t, warnings, 1)
	})

	t.Run("changing the component type makes it a new attachment", func(t *testing.T) {
		newApp := appliesToApp(component("web", "worker", "sts-only"))

		errs, warnings := h.ValidateTraitAppliesTo(context.Background(), newApp, oldApp)

		require.Len(t, errs, 1)
		assert.Empty(t, warnings)
	})
}

func TestValidateTraitAppliesToWithInheritance(t *testing.T) {
	withAppliesToValidation(t)

	parentTrait := appliesToTraitDef("sts-scaler", "statefulsets.apps")
	childTrait := appliesToTraitDef("tenant-scaler")
	childTrait.Spec.Extends = "sts-scaler"

	parentComp := appliesToComponentDef("base-web", common.WorkloadTypeDescriptor{Type: "deployments.apps"})
	// What the mutating webhook stores for a child without a workload.
	childComp := appliesToComponentDef("tenant-web", common.WorkloadTypeDescriptor{Type: types.AutoDetectWorkloadDefinition})
	childComp.Spec.Extends = "base-web"
	orphanComp := appliesToComponentDef("orphan-web", common.WorkloadTypeDescriptor{Type: types.AutoDetectWorkloadDefinition})
	orphanComp.Spec.Extends = "gone"

	definitions := []client.Object{parentTrait, childTrait, parentComp, childComp, orphanComp}

	t.Run("a child trait inherits its parent's appliesToWorkloads", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.EnableDefinitionInheritance, true)
		app := appliesToApp(component("web", "base-web", "tenant-scaler"))

		errs := appliesToErrs(appliesToHandler(definitions...), app)

		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, `trait "tenant-scaler"`)
	})

	// Child components don't inherit their parent's workload yet
	// (kubevela/kubevela#7508), so workload entries aren't checked on them. Once
	// that's fixed, sts-scaler is refused here.
	t.Run("a child component has no workload type of its own yet", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.EnableDefinitionInheritance, true)
		app := appliesToApp(component("web", "tenant-web", "sts-scaler"))

		assert.Empty(t, appliesToErrs(appliesToHandler(definitions...), app))
	})

	t.Run("an extending component is left to rendering while inheritance is off", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.EnableDefinitionInheritance, false)
		app := appliesToApp(component("web", "tenant-web", "sts-scaler"))
		assert.Empty(t, appliesToErrs(appliesToHandler(definitions...), app))
	})

	t.Run("a component whose parent is missing is left to rendering", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.EnableDefinitionInheritance, true)
		app := appliesToApp(component("web", "orphan-web", "sts-scaler"))
		assert.Empty(t, appliesToErrs(appliesToHandler(definitions...), app))
	})

	t.Run("a chain that breaks the definition rules is left to rendering", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.EnableDefinitionInheritance, true)
		// A child may only extend a parent in its own namespace; sts-scaler is in vela-system.
		crossNS := appliesToTraitDef("cross-ns-scaler")
		crossNS.Namespace = "default"
		crossNS.Spec.Extends = "sts-scaler"
		app := appliesToApp(component("web", "base-web", "cross-ns-scaler"))

		assert.Empty(t, appliesToErrs(appliesToHandler(append(definitions, crossNS)...), app))
	})

	t.Run("fails closed when a parent cannot be read", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.EnableDefinitionInheritance, true)
		cli := appliesToClient(&interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*v1beta1.TraitDefinition); ok && key.Name == "sts-scaler" {
					return apierrors.NewServiceUnavailable("apiserver unavailable")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}, definitions...)
		app := appliesToApp(component("web", "base-web", "tenant-scaler"))

		errs := appliesToErrs(&ValidatingHandler{Client: cli}, app)

		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInternal, errs[0].Type)
		assert.Equal(t, "spec.components[0].traits[0].type", errs[0].Field)
	})

	t.Run("an unresolvable chain is left to rendering", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.EnableDefinitionInheritance, false)
		app := appliesToApp(component("web", "base-web", "tenant-scaler"))
		assert.Empty(t, appliesToErrs(appliesToHandler(definitions...), app))
	})

	// A "@v1" parent resolves with the Application's annotations, as at render:
	// auto-update reads the latest v1.x.
	t.Run("a pinned parent is resolved with the Application's annotations", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.EnableDefinitionInheritance, true)
		revision := func(name string, appliesTo ...string) *v1beta1.DefinitionRevision {
			return &v1beta1.DefinitionRevision{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: oam.SystemDefinitionNamespace,
					Labels:    map[string]string{oam.LabelTraitDefinitionName: "sts-scaler"},
				},
				Spec: v1beta1.DefinitionRevisionSpec{
					DefinitionType:  common.TraitType,
					TraitDefinition: *appliesToTraitDef("sts-scaler", appliesTo...),
				},
			}
		}
		pinned := appliesToTraitDef("pinned-scaler")
		pinned.Spec.Extends = "sts-scaler@v1"
		h := appliesToHandler(append(definitions, pinned,
			revision("sts-scaler-v1.0", "*"),
			revision("sts-scaler-v1.1", "statefulsets.apps"))...)

		app := appliesToApp(component("web", "base-web", "pinned-scaler"))
		app.Annotations = map[string]string{oam.AnnotationAutoUpdate: "true"}
		errs := appliesToErrs(h, app)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, `appliesToWorkloads: ["statefulsets.apps"]`)

		// Without auto-update, "@v1" doesn't exist, so the chain is left to rendering.
		app.Annotations = nil
		assert.Empty(t, appliesToErrs(h, app))
	})
}

// Through the admission handler against a real apiserver: this checks the
// wiring, and that the GVK-only "worker" fixture gets its workload from the
// apiserver's RESTMapper.
var _ = Describe("Trait appliesToWorkloads admission", func() {
	var stsOnly *v1beta1.TraitDefinition

	setGate := func(on bool) {
		Expect(utilfeature.DefaultMutableFeatureGate.Set(
			fmt.Sprintf("%s=%t", features.ValidateTraitAppliesToWorkloads, on))).Should(Succeed())
	}

	BeforeEach(func() {
		handler = &ValidatingHandler{Client: k8sClient, Decoder: decoder}
		stsOnly = &v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "sts-only-scaler", Namespace: oam.SystemDefinitionNamespace},
			Spec: v1beta1.TraitDefinitionSpec{
				AppliesToWorkloads: []string{"statefulsets.apps"},
				Schematic: &common.Schematic{CUE: &common.CUE{
					Template: "patch: spec: replicas: parameter.replicas\nparameter: replicas: *1 | int\n",
				}},
			},
		}
		Expect(k8sClient.Create(ctx, stsOnly)).Should(Succeed())
		DeferCleanup(func() {
			Expect(k8sClient.Delete(ctx, stsOnly)).Should(Succeed())
		})

		DeferCleanup(setGate, utilfeature.DefaultMutableFeatureGate.Enabled(features.ValidateTraitAppliesToWorkloads))
		setGate(true)
	})

	appJSON := func(traitType string) []byte {
		return []byte(fmt.Sprintf(`
{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Application",
"metadata":{"name":"applies-to-app"},
"spec":{"components":[{"name":"web","type":"worker","properties":{"image":"busybox"},
"traits":[{"type":%q,"properties":{"replicas":3}}]}]}}
`, traitType))
	}

	request := func(traitType string) admission.Request {
		return admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applications"},
				Object:    runtime.RawExtension{Raw: appJSON(traitType)},
			},
		}
	}

	updateRequest := func(oldTraitType, newTraitType string) admission.Request {
		req := request(newTraitType)
		req.Operation = admissionv1.Update
		req.OldObject = runtime.RawExtension{Raw: appJSON(oldTraitType)}
		return req
	}

	It("rejects a trait that does not apply to the component's workload", func() {
		resp := handler.Handle(ctx, request("sts-only-scaler"))
		Expect(resp.Allowed).Should(BeFalse())
		Expect(resp.Result.Message).Should(ContainSubstring(
			`trait "sts-only-scaler" cannot apply to component "web" of type "worker" (workload "deployments.apps")`))
	})

	It("admits a trait that applies", func() {
		resp := handler.Handle(ctx, request("scaler"))
		Expect(resp.Allowed).Should(BeTrue(), resp.Result.Message)
	})

	It("admits an update that keeps a trait the Application already had, with a warning", func() {
		resp := handler.Handle(ctx, updateRequest("sts-only-scaler", "sts-only-scaler"))
		Expect(resp.Allowed).Should(BeTrue(), resp.Result.Message)
		Expect(resp.Warnings).Should(ContainElement(ContainSubstring("allowed because the Application already had it")))
	})

	It("rejects an update that attaches the trait", func() {
		resp := handler.Handle(ctx, updateRequest("scaler", "sts-only-scaler"))
		Expect(resp.Allowed).Should(BeFalse())
		Expect(resp.Result.Message).Should(ContainSubstring(`trait "sts-only-scaler" cannot apply`))
	})

	It("admits anything while the gate is off", func() {
		setGate(false)
		resp := handler.Handle(ctx, request("sts-only-scaler"))
		Expect(resp.Allowed).Should(BeTrue(), resp.Result.Message)
	})
})
