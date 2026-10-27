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

// appliesToRESTMapper knows the workloads used below, for definitions that set
// only the GVK.
func appliesToRESTMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, meta.RESTScopeNamespace)
	return mapper
}

func appliesToHandler(objects ...client.Object) *ValidatingHandler {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	cli := fake.NewClientBuilder().WithScheme(scheme).WithRESTMapper(appliesToRESTMapper()).WithObjects(objects...).Build()
	return &ValidatingHandler{Client: cli}
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

	t.Run("fails closed when a definition cannot be read", func(t *testing.T) {
		withAppliesToValidation(t)
		scheme := runtime.NewScheme()
		_ = v1beta1.AddToScheme(scheme)
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(definitions...).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*v1beta1.DefinitionRevision); ok {
						return fmt.Errorf("apiserver unavailable")
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}).Build()
		app := appliesToApp(component("web", "webservice", "anywhere@v1"))

		errs := appliesToErrs((&ValidatingHandler{Client: cli}), app)

		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeInternal, errs[0].Type)
		assert.Equal(t, "spec.components[0].traits[0].type", errs[0].Field)
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
	childComp := appliesToComponentDef("tenant-web", common.WorkloadTypeDescriptor{})
	childComp.Spec.Extends = "base-web"

	definitions := []client.Object{parentTrait, childTrait, parentComp, childComp}

	t.Run("a child trait inherits its parent's appliesToWorkloads", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.EnableDefinitionInheritance, true)
		app := appliesToApp(component("web", "base-web", "tenant-scaler"))

		errs := appliesToErrs(appliesToHandler(definitions...), app)

		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, `trait "tenant-scaler"`)
	})

	t.Run("a child component inherits its parent's workload", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.EnableDefinitionInheritance, true)
		app := appliesToApp(component("web", "tenant-web", "sts-scaler"))

		errs := appliesToErrs(appliesToHandler(definitions...), app)

		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, `(workload "deployments.apps")`)
	})

	t.Run("an unresolvable chain is left to rendering", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.EnableDefinitionInheritance, false)
		app := appliesToApp(component("web", "base-web", "tenant-scaler"))
		assert.Empty(t, appliesToErrs(appliesToHandler(definitions...), app))
	})
}

// Through the admission handler against a real apiserver, so the check is
// wired into the soundness phase and the GVK-only "worker" fixture's workload
// is resolved by the apiserver's RESTMapper.
var _ = Describe("Trait appliesToWorkloads admission", func() {
	var stsOnly *v1beta1.TraitDefinition

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

		enabled := utilfeature.DefaultMutableFeatureGate.Enabled(features.ValidateTraitAppliesToWorkloads)
		DeferCleanup(func() {
			Expect(utilfeature.DefaultMutableFeatureGate.Set(
				fmt.Sprintf("%s=%t", features.ValidateTraitAppliesToWorkloads, enabled))).Should(Succeed())
		})
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

	setGate := func(on bool) {
		Expect(utilfeature.DefaultMutableFeatureGate.Set(
			fmt.Sprintf("%s=%t", features.ValidateTraitAppliesToWorkloads, on))).Should(Succeed())
	}

	It("rejects a trait that does not apply to the component's workload", func() {
		setGate(true)
		resp := handler.Handle(ctx, request("sts-only-scaler"))
		Expect(resp.Allowed).Should(BeFalse())
		Expect(resp.Result.Message).Should(ContainSubstring(
			`trait "sts-only-scaler" cannot apply to component "web" of type "worker" (workload "deployments.apps")`))
	})

	It("admits a trait that applies", func() {
		setGate(true)
		resp := handler.Handle(ctx, request("scaler"))
		Expect(resp.Allowed).Should(BeTrue(), resp.Result.Message)
	})

	It("admits an update that keeps a trait the Application already had, with a warning", func() {
		setGate(true)
		resp := handler.Handle(ctx, updateRequest("sts-only-scaler", "sts-only-scaler"))
		Expect(resp.Allowed).Should(BeTrue(), resp.Result.Message)
		Expect(resp.Warnings).Should(ContainElement(ContainSubstring("allowed because the Application already had it")))
	})

	It("rejects an update that attaches the trait", func() {
		setGate(true)
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
