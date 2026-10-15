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

package source_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// These specs prove that a step's expressions are resolved when the step runs:
// that it reads an earlier step's output and the current forEach item inline,
// and that a source holds for the whole run but not into the next. How the
// workflow orders and validates reads has its own tests in kubevela/workflow.
var _ = Describe("Workflow step expressions", Ordered, ContinueOnFailure, func() {
	ctx := context.Background()

	var namespaceName string
	var ns corev1.Namespace

	// A source reading the settings ConfigMap, kept for a second only: what holds
	// its value for the rest of a run is the run, not the cache.
	const settingsSource = `
import "vela/kube"

schema: data: [string]: string
$internal: {
  key: "step-settings-\(context.namespace)"
  keyInputs: ["namespace"]
}
storage: storageTTL: "1s"
parameter: name: string
_cm: kube.#Get & {
  $params: resource: {
    apiVersion: "v1"
    kind:       "ConfigMap"
    metadata: {name: parameter.name, namespace: context.namespace}
  }
}
output: data: _cm.$returns.data
`

	const listStep = `
parameter: {}
names: ["from-a", "from-b"]
`

	// configMapStep applies a ConfigMap named name holding data.
	configMapStep := func(step, name string, data map[string]string) wfTypesv1alpha1.WorkflowStep {
		var pairs []string
		for k, v := range data {
			pairs = append(pairs, fmt.Sprintf("%q:%q", k, v))
		}
		return wfTypesv1alpha1.WorkflowStep{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
			Name: step,
			Type: "apply-object",
			Properties: &runtime.RawExtension{Raw: []byte(fmt.Sprintf(
				`{"value":{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":%q,"namespace":%q},"data":{%s}}}`,
				name, namespaceName, strings.Join(pairs, ",")))},
		}}
	}

	createApp := func(app *v1beta1.Application) {
		GinkgoHelper()
		Eventually(func() error {
			err := k8sClient.Create(ctx, optIn(app))
			if err != nil && strings.Contains(err.Error(), "not found") {
				return err
			}
			if err != nil {
				StopTrying("Application refused").Wrap(err).Now()
			}
			return nil
		}, 30*time.Second, time.Second).Should(Succeed())
	}

	// configMapData is the data of the ConfigMap named name, nil when there is
	// none; any other error fails the poll, so an absence check cannot pass on it.
	configMapData := func(name string) func() (map[string]string, error) {
		return func() (map[string]string, error) {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: name}, cm)
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			return cm.Data, nil
		}
	}

	BeforeEach(func() {
		namespaceName = randomNamespaceName("step-expr-e2e")
		ns = createNamespace(ctx, namespaceName)
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
		// A namespaced SourceDefinition writes a ConfigTemplate, and its cache a
		// Config and a Secret, in vela-system, labelled with the namespace.
		nsLabel := client.MatchingLabels{"sourcedefinition.oam.dev/namespace": namespaceName}
		Eventually(func() error {
			for _, obj := range []client.Object{&configv1alpha1.ConfigTemplate{}, &configv1alpha1.Config{},
				&corev1.ConfigMap{}, &corev1.Secret{}} {
				if err := k8sClient.DeleteAllOf(ctx, obj, client.InNamespace("vela-system"), nsLabel); err != nil {
					return err
				}
			}
			var templates configv1alpha1.ConfigTemplateList
			if err := k8sClient.List(ctx, &templates, client.InNamespace("vela-system"), nsLabel); err != nil {
				return err
			}
			var configs configv1alpha1.ConfigList
			if err := k8sClient.List(ctx, &configs, client.InNamespace("vela-system"), nsLabel); err != nil {
				return err
			}
			var configMaps corev1.ConfigMapList
			if err := k8sClient.List(ctx, &configMaps, client.InNamespace("vela-system"), nsLabel); err != nil {
				return err
			}
			var secrets corev1.SecretList
			if err := k8sClient.List(ctx, &secrets, client.InNamespace("vela-system"), nsLabel); err != nil {
				return err
			}
			if n := len(templates.Items) + len(configs.Items) + len(configMaps.Items) + len(secrets.Items); n > 0 {
				return fmt.Errorf("%d ConfigTemplates, Configs, ConfigMaps and Secrets for %s still in vela-system", n, namespaceName)
			}
			return nil
		}, 30*time.Second, time.Second).Should(Succeed())
	})

	It("reads an earlier step's output and the current item inline", func() {
		applyDefinition(ctx, &v1beta1.WorkflowStepDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "make-names", Namespace: namespaceName},
			Spec: v1beta1.WorkflowStepDefinitionSpec{
				Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: listStep}},
			},
		})
		loop := configMapStep("apply", "$(each.item)", map[string]string{
			"index": "#$(each.index)",
			"app":   "$(context.appName)",
		})
		loop.ForEach = &wfTypesv1alpha1.ForEach{Items: &apiextensionsv1.JSON{Raw: []byte(`"$(steps.names.outputs.names)"`)}}
		createApp(&v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "inline-reads", Namespace: namespaceName},
			Spec: v1beta1.ApplicationSpec{
				Components: []oamcomm.ApplicationComponent{},
				Workflow: &v1beta1.Workflow{Steps: []wfTypesv1alpha1.WorkflowStep{
					{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
						Name:    "names",
						Type:    "make-names",
						Outputs: wfTypesv1alpha1.StepOutputs{{Name: "names", ValueFrom: "names"}},
					}},
					loop,
				}},
			},
		})
		Eventually(configMapData("from-a"), 60*time.Second, time.Second).
			Should(Equal(map[string]string{"index": "#0", "app": "inline-reads"}))
		Eventually(configMapData("from-b"), 60*time.Second, time.Second).
			Should(Equal(map[string]string{"index": "#1", "app": "inline-reads"}))

		By("the read is recorded as what the loop waits on")
		Eventually(func() []oamcomm.WorkflowStepDependency {
			app := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "inline-reads"}, app); err != nil ||
				app.Status.Workflow == nil {
				return nil
			}
			return app.Status.Workflow.Dependencies
		}, 30*time.Second, time.Second).Should(ContainElement(oamcomm.WorkflowStepDependency{
			Step: "apply", DependsOn: "names", Source: "expression"}))
	})

	// configMapComponent applies a ConfigMap named after the component, holding data.
	const configMapComponent = `
parameter: {data: [string]: string}
output: {
  apiVersion: "v1"
  kind:       "ConfigMap"
  metadata: name: context.name
  data: parameter.data
}
`
	const failStep = `
import "vela/builtin"
parameter: {}
fail: builtin.#Fail & {$params: message: "migrations failed"}
`
	component := func(name, data string) oamcomm.ApplicationComponent {
		return oamcomm.ApplicationComponent{Name: name, Type: "step-cm",
			Properties: &runtime.RawExtension{Raw: []byte(`{"data":` + data + `}`)}}
	}
	applyStep := func(c string) wfTypesv1alpha1.WorkflowStep {
		return wfTypesv1alpha1.WorkflowStep{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
			Name: c, Type: "apply-component", Properties: &runtime.RawExtension{Raw: []byte(`{"component":"` + c + `"}`)}}}
	}
	appStatus := func(name string) func() *v1beta1.Application {
		return func() *v1beta1.Application {
			app := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: name}, app); err != nil ||
				app.Status.Workflow == nil {
				return nil
			}
			return app
		}
	}
	readDependency := oamcomm.WorkflowStepDependency{Step: "api", DependsOn: "db", Source: "expression"}

	It("in a workflow run in order, a component read does not run its step after an earlier failure", func() {
		applyDefinition(ctx, exprComponentDefinition(namespaceName, "step-cm", configMapComponent))
		applyDefinition(ctx, &v1beta1.WorkflowStepDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "fail-migrations", Namespace: namespaceName},
			Spec:       v1beta1.WorkflowStepDefinitionSpec{Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: failStep}}},
		})
		createApp(&v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "in-order", Namespace: namespaceName},
			Spec: v1beta1.ApplicationSpec{
				Components: []oamcomm.ApplicationComponent{
					component("db", `{"host":"db.local"}`),
					component("api", `{"db":"$(component.db.output.data.host)"}`),
				},
				Workflow: &v1beta1.Workflow{Steps: []wfTypesv1alpha1.WorkflowStep{
					applyStep("db"),
					{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "migrate", Type: "fail-migrations"}},
					applyStep("api"),
				}},
			},
		})
		Eventually(func(g Gomega) {
			app := appStatus("in-order")()
			g.Expect(app).ShouldNot(BeNil())
			phases := map[string]string{}
			for _, s := range app.Status.Workflow.Steps {
				phases[s.Name] = string(s.Phase)
			}
			g.Expect(phases).Should(HaveKeyWithValue("migrate", "failed"))
			g.Expect(phases).Should(HaveKeyWithValue("api", "skipped"), "as it would be with no read")
			g.Expect(app.Status.Workflow.Dependencies).Should(ContainElement(readDependency),
				"the read is still recorded, with no dependsOn added for it")
		}, 90*time.Second, time.Second).Should(Succeed())
		Expect(configMapData("db")()).ShouldNot(BeNil())
		Consistently(configMapData("api"), 5*time.Second, time.Second).Should(BeNil(), "api is never applied")
	})

	It("in a generated workflow, a component read is recorded as what its step waits on", func() {
		applyDefinition(ctx, exprComponentDefinition(namespaceName, "step-cm", configMapComponent))
		createApp(&v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "generated", Namespace: namespaceName},
			Spec: v1beta1.ApplicationSpec{Components: []oamcomm.ApplicationComponent{
				component("db", `{"host":"db.local"}`),
				component("api", `{"db":"$(component.db.output.data.host)"}`),
			}},
		})
		Eventually(configMapData("api"), 90*time.Second, time.Second).Should(Equal(map[string]string{"db": "db.local"}))
		Eventually(func() []oamcomm.WorkflowStepDependency {
			if app := appStatus("generated")(); app != nil {
				return app.Status.Workflow.Dependencies
			}
			return nil
		}, 30*time.Second, time.Second).Should(Equal([]oamcomm.WorkflowStepDependency{readDependency}),
			"an edge KubeVela adds for a read is the read's, not the spec's")
	})

	It("decides steps by $( ) conditions reading context, a source, a step and the loop", func() {
		applyDefinition(ctx, &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "step-settings", Namespace: namespaceName},
			Spec:       v1beta1.SourceDefinitionSpec{Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: settingsSource}}},
		})
		applyDefinition(ctx, &v1beta1.WorkflowStepDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "make-names", Namespace: namespaceName},
			Spec:       v1beta1.WorkflowStepDefinitionSpec{Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: listStep}}},
		})
		Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "flags", Namespace: namespaceName},
			Data:       map[string]string{"mode": "on"},
		})).Should(Succeed())

		when := func(step, cond string, dependsOn ...string) wfTypesv1alpha1.WorkflowStep {
			s := configMapStep(step, "cond-"+step, map[string]string{"ran": "yes"})
			s.If, s.DependsOn = cond, dependsOn
			return s
		}
		save := configMapStep("save", "cond-$(each.item)", map[string]string{"pass": "#$(each.index)"})
		perName := wfTypesv1alpha1.WorkflowStep{
			WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "per-name", Type: "step-group"},
			ForEach:          &wfTypesv1alpha1.ForEach{Items: &apiextensionsv1.JSON{Raw: []byte(`"$(steps.names.outputs.names)"`)}},
			SubSteps: []wfTypesv1alpha1.WorkflowStepBase{func() wfTypesv1alpha1.WorkflowStepBase {
				b := save.WorkflowStepBase
				b.If = `$(each.index == 1 && has(source.cfg.data.mode) && source.cfg.data.mode == "on")`
				return b
			}()},
		}
		createApp(&v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "conditions", Namespace: namespaceName},
			Spec: v1beta1.ApplicationSpec{
				Components: []oamcomm.ApplicationComponent{},
				Sources: []v1beta1.ApplicationSource{{Name: "cfg", Type: "step-settings",
					Properties: &runtime.RawExtension{Raw: []byte(`{"name":"flags"}`)}}},
				Workflow: &v1beta1.Workflow{
					Mode: &wfTypesv1alpha1.WorkflowExecuteMode{Steps: "DAG", SubSteps: "DAG"},
					Steps: []wfTypesv1alpha1.WorkflowStep{
						{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "names", Type: "make-names",
							Outputs: wfTypesv1alpha1.StepOutputs{{Name: "names", ValueFrom: "names"}}}},
						when("all-true", `$(context.appName == "conditions" && has(source.cfg.data.mode) && source.cfg.data.mode == "on" && `+
							`steps.names.succeeded && steps.names.outputs.names.size() == 2)`),
						when("source-false", `$(has(source.cfg.data.mode) && source.cfg.data.mode == "off")`),
						when("cue-if", "status.names.succeeded", "names"),
						perName,
					},
				},
			},
		})

		exists := func(name string) func() (bool, error) {
			return func() (bool, error) {
				data, err := configMapData(name)()
				return data != nil, err
			}
		}
		Eventually(exists("cond-all-true"), 90*time.Second, time.Second).Should(BeTrue())
		Eventually(exists("cond-cue-if"), 30*time.Second, time.Second).Should(BeTrue(), "a CUE if beside them is unchanged")
		Eventually(configMapData("cond-from-b"), 30*time.Second, time.Second).Should(Equal(map[string]string{"pass": "#1"}),
			"a sub-step's condition is decided each pass")

		var app *v1beta1.Application
		Eventually(func(g Gomega) {
			app = &v1beta1.Application{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "conditions"}, app)).Should(Succeed())
			g.Expect(app.Status.Workflow).ShouldNot(BeNil())
			g.Expect(app.Status.Workflow.Finished).Should(BeTrue())
		}, 60*time.Second, time.Second).Should(Succeed())
		phases := map[string]string{}
		for _, s := range app.Status.Workflow.Steps {
			phases[s.Name] = string(s.Phase)
			for _, sub := range s.SubStepsStatus {
				phases[sub.Name] = string(sub.Phase)
			}
		}
		Expect(phases).Should(HaveKeyWithValue("all-true", "succeeded"))
		Expect(phases).Should(HaveKeyWithValue("source-false", "skipped"))
		Expect(phases).Should(HaveKeyWithValue("cue-if", "succeeded"))
		Expect(configMapData("cond-source-false")()).Should(BeNil())
		Expect(configMapData("cond-from-a")()).Should(BeNil(), "the first pass's condition is false")
		Expect(app.Status.Workflow.Dependencies).Should(ContainElement(oamcomm.WorkflowStepDependency{
			Step: "all-true", DependsOn: "names", Source: "expression"}), "a condition's reads are what its step waits on")
	})

	It("refuses a condition that is not a bool at admission", func() {
		applyDefinition(ctx, &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "step-settings", Namespace: namespaceName},
			Spec:       v1beta1.SourceDefinitionSpec{Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: settingsSource}}},
		})
		step := configMapStep("notify", "cond-notify", map[string]string{"ran": "yes"})
		step.If = "$(source.cfg.data.mode)"
		err := k8sClient.Create(ctx, optIn(&v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "not-a-bool", Namespace: namespaceName},
			Spec: v1beta1.ApplicationSpec{
				Components: []oamcomm.ApplicationComponent{},
				Sources: []v1beta1.ApplicationSource{{Name: "cfg", Type: "step-settings",
					Properties: &runtime.RawExtension{Raw: []byte(`{"name":"flags"}`)}}},
				Workflow: &v1beta1.Workflow{Steps: []wfTypesv1alpha1.WorkflowStep{step}},
			},
		}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("spec.workflow.steps[0].if"))
		Expect(err.Error()).To(ContainSubstring("is string but if expects bool"))
	})

	It("holds a source for the whole run, and reads it afresh in the next", func() {
		applyDefinition(ctx, &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "step-settings", Namespace: namespaceName},
			Spec: v1beta1.SourceDefinitionSpec{
				Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: settingsSource}},
			},
		})
		settings := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "settings", Namespace: namespaceName},
			Data:       map[string]string{"color": "blue"},
		}
		Expect(k8sClient.Create(ctx, settings)).Should(Succeed())

		pause := wfTypesv1alpha1.WorkflowStep{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
			Name: "pause", Type: "suspend", Properties: &runtime.RawExtension{Raw: []byte(`{"duration":"30s"}`)}}}
		steps := func(run string) []wfTypesv1alpha1.WorkflowStep {
			return []wfTypesv1alpha1.WorkflowStep{
				configMapStep("first", "first", map[string]string{"color": "$(source.cfg.data.color)", "run": run}),
				pause,
				configMapStep("second", "second", map[string]string{"color": "$(source.cfg.data.color)", "run": run}),
			}
		}
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "pinned", Namespace: namespaceName},
			Spec: v1beta1.ApplicationSpec{
				Components: []oamcomm.ApplicationComponent{},
				Sources: []v1beta1.ApplicationSource{{Name: "cfg", Type: "step-settings",
					Properties: &runtime.RawExtension{Raw: []byte(`{"name":"settings"}`)}}},
				Workflow: &v1beta1.Workflow{Steps: steps("1")},
			},
		}
		createApp(app)
		Eventually(configMapData("first"), 60*time.Second, time.Second).
			Should(Equal(map[string]string{"color": "blue", "run": "1"}))

		By("the source changes while the run waits")
		settings.Data["color"] = "red"
		Expect(k8sClient.Update(ctx, settings)).Should(Succeed())

		Eventually(configMapData("second"), 90*time.Second, time.Second).
			Should(Equal(map[string]string{"color": "blue", "run": "1"}), "a later step reads what the first did")

		By("the source stays reported as read by both steps, after they have run")
		Eventually(func() []string {
			got := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), got); err != nil || len(got.Status.Sources) == 0 {
				return nil
			}
			readers := []string{got.Status.Sources[0].Phase}
			for _, c := range got.Status.Sources[0].ConsumedBy {
				readers = append(readers, c.DefinitionKind+"/"+c.Name)
			}
			return readers
		}, 30*time.Second, time.Second).Should(ConsistOf("Resolved", "workflowstep/first", "workflowstep/second"))

		By("a new revision is a new run")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
		app.Spec.Workflow.Steps = steps("2")
		Expect(k8sClient.Update(ctx, app)).Should(Succeed())
		Eventually(configMapData("first"), 60*time.Second, time.Second).
			Should(Equal(map[string]string{"color": "red", "run": "2"}))
		Eventually(configMapData("second"), 90*time.Second, time.Second).
			Should(Equal(map[string]string{"color": "red", "run": "2"}))
	})
})
