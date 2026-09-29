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

package controllers_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// These specs prove the wiring of a forEach step end to end: that a list from a source
// or an earlier step reaches the workflow engine, that each item reaches its step
// through inputs, and that a new revision iterates its own list. The engine's ordering
// and failure rules have their own tests in kubevela/workflow.
var _ = Describe("forEach workflow steps", func() {
	ctx := context.Background()

	var namespaceName string
	var ns corev1.Namespace

	const inventorySource = `
schema: {
  clusters: [...{name: string}]
  region:   string
}
$internal: {key: "cluster-inventory", keyInputs: []}
storage: {storageTTL: "1h"}
output: {
  clusters: [{name: "c1"}, {name: "c2"}]
  region:   "eu-west"
}
parameter: {}
`

	// A step whose only job is to produce a list for a later forEach.from to read.
	const listStep = `
parameter: {}
names: ["from-a", "from-b"]
`

	// applyObject is an apply-object step creating a ConfigMap whose name comes from the
	// current item, with inputs doing the passing.
	applyObject := func(name string, forEach *wfTypesv1alpha1.ForEach, from string) wfTypesv1alpha1.WorkflowStep {
		return wfTypesv1alpha1.WorkflowStep{
			WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
				Name: name,
				Type: "apply-object",
				Properties: &runtime.RawExtension{Raw: []byte(fmt.Sprintf(
					`{"value":{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":%q},"data":{"loop":"yes"}}}`, namespaceName))},
				Inputs: wfTypesv1alpha1.StepInputs{{From: from, ParameterKey: "value.metadata.name"}},
			},
			ForEach: forEach,
		}
	}
	items := func(raw string) *wfTypesv1alpha1.ForEach {
		return &wfTypesv1alpha1.ForEach{Items: &apiextensionsv1.JSON{Raw: []byte(raw)}}
	}

	// createApp retries only while a definition the Application names has not reached
	// the webhook's cache yet; any other denial stops at once with its own message.
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

	configMapExists := func(name string) func() error {
		return func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: name}, &corev1.ConfigMap{})
		}
	}

	subStepNames := func(appName, step string) func() []string {
		return func() []string {
			app := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: appName}, app); err != nil || app.Status.Workflow == nil {
				return nil
			}
			var names []string
			for _, s := range app.Status.Workflow.Steps {
				if s.Name != step {
					continue
				}
				for _, sub := range s.SubStepsStatus {
					names = append(names, sub.Name+"="+string(sub.Phase))
				}
			}
			return names
		}
	}

	BeforeEach(func() {
		namespaceName = randomNamespaceName("foreach-e2e")
		ns = createNamespace(ctx, namespaceName)
		applyDefinition(ctx, &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-inventory", Namespace: namespaceName},
			Spec: v1beta1.SourceDefinitionSpec{
				Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: inventorySource}},
			},
		})
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
		nsLabel := client.MatchingLabels{"sourcedefinition.oam.dev/namespace": namespaceName}
		Eventually(func() error {
			if err := k8sClient.DeleteAllOf(ctx, &corev1.ConfigMap{}, client.InNamespace("vela-system"), nsLabel); err != nil {
				return err
			}
			return k8sClient.DeleteAllOf(ctx, &corev1.Secret{}, client.InNamespace("vela-system"), nsLabel)
		}, 30*time.Second, time.Second).Should(Succeed())
	})

	It("repeats a step once per item a source returns", func() {
		createApp(&v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "from-source", Namespace: namespaceName},
			Spec: v1beta1.ApplicationSpec{
				Components: []oamcomm.ApplicationComponent{},
				Sources:    []v1beta1.ApplicationSource{{Name: "inv", Type: "cluster-inventory"}},
				Workflow: &v1beta1.Workflow{Steps: []wfTypesv1alpha1.WorkflowStep{
					applyObject("apply", items(`"$(source.inv.clusters)"`), "loop.item.name"),
				}},
			},
		})
		Eventually(configMapExists("c1"), 60*time.Second, time.Second).Should(Succeed())
		Eventually(configMapExists("c2"), 60*time.Second, time.Second).Should(Succeed())
		Eventually(subStepNames("from-source", "apply"), 60*time.Second, time.Second).
			Should(ConsistOf("apply-0=succeeded", "apply-1=succeeded"))
	})

	It("reads the list from an earlier step's output", func() {
		applyDefinition(ctx, &v1beta1.WorkflowStepDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "make-names", Namespace: namespaceName},
			Spec: v1beta1.WorkflowStepDefinitionSpec{
				Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: listStep}},
			},
		})
		createApp(&v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "from-output", Namespace: namespaceName},
			Spec: v1beta1.ApplicationSpec{
				Components: []oamcomm.ApplicationComponent{},
				Workflow: &v1beta1.Workflow{Steps: []wfTypesv1alpha1.WorkflowStep{
					{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
						Name:    "names",
						Type:    "make-names",
						Outputs: wfTypesv1alpha1.StepOutputs{{Name: "names", ValueFrom: "names"}},
					}},
					applyObject("apply", &wfTypesv1alpha1.ForEach{From: "names"}, "loop.item"),
				}},
			},
		})
		Eventually(configMapExists("from-a"), 60*time.Second, time.Second).Should(Succeed())
		Eventually(configMapExists("from-b"), 60*time.Second, time.Second).Should(Succeed())
	})

	It("iterates the new list after the Application is updated", func() {
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "updated", Namespace: namespaceName},
			Spec: v1beta1.ApplicationSpec{
				Components: []oamcomm.ApplicationComponent{},
				Workflow: &v1beta1.Workflow{Steps: []wfTypesv1alpha1.WorkflowStep{
					applyObject("apply", items(`["first-a","first-b"]`), "loop.item"),
				}},
			},
		}
		createApp(app)
		Eventually(subStepNames("updated", "apply"), 60*time.Second, time.Second).
			Should(ConsistOf("apply-0=succeeded", "apply-1=succeeded"))

		Eventually(func() error {
			latest := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), latest); err != nil {
				return err
			}
			latest.Spec.Workflow.Steps[0].ForEach.Items = &apiextensionsv1.JSON{Raw: []byte(`["second"]`)}
			return k8sClient.Update(ctx, latest)
		}, 30*time.Second, time.Second).Should(Succeed())

		Eventually(configMapExists("second"), 60*time.Second, time.Second).Should(Succeed())
		Eventually(subStepNames("updated", "apply"), 60*time.Second, time.Second).
			Should(ConsistOf("apply-0=succeeded"), "the new run iterates its own list, not the one pinned before")
	})

	It("refuses an expression that does not type as a list", func() {
		err := k8sClient.Create(ctx, optIn(&v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "not-a-list", Namespace: namespaceName},
			Spec: v1beta1.ApplicationSpec{
				Components: []oamcomm.ApplicationComponent{},
				Sources:    []v1beta1.ApplicationSource{{Name: "inv", Type: "cluster-inventory"}},
				Workflow: &v1beta1.Workflow{Steps: []wfTypesv1alpha1.WorkflowStep{
					applyObject("apply", items(`"$(source.inv.region)"`), "loop.item"),
				}},
			},
		}))
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("forEach.items expects list"))
	})
})
