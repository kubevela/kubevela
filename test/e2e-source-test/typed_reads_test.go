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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("Typed component and loop reads", Ordered, ContinueOnFailure, func() {
	ctx := context.Background()
	var namespaceName string
	var ns corev1.Namespace

	// typed-cm applies a ConfigMap named after the component, holding data, and
	// takes an int it does nothing with, for a read to feed.
	const typedConfigMap = `
parameter: {
	count: *1 | int
	data:  [string]: string
}
output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: context.name
	data: parameter.data
}
`
	// typed-note writes the text it is given to a ConfigMap named after it.
	const textStep = `
import (
	"strings"
	"vela/kube"
)
parameter: {text: string}
apply: kube.#Apply & {$params: value: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: {
		name:      strings.Replace(strings.ToLower(parameter.text), " ", "-", -1)
		namespace: context.namespace
	}
	data: text: parameter.text
}}
`
	component := func(name, props string) oamcomm.ApplicationComponent {
		return oamcomm.ApplicationComponent{Name: name, Type: "typed-cm",
			Properties: &runtime.RawExtension{Raw: []byte(props)}}
	}
	app := func(name string, comps []oamcomm.ApplicationComponent, steps ...wfTypesv1alpha1.WorkflowStep) *v1beta1.Application {
		a := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespaceName},
			Spec:       v1beta1.ApplicationSpec{Components: comps},
		}
		if len(steps) > 0 {
			a.Spec.Workflow = &v1beta1.Workflow{Steps: steps}
		}
		return optIn(a)
	}
	loop := func(items, props string) wfTypesv1alpha1.WorkflowStep {
		return wfTypesv1alpha1.WorkflowStep{
			WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "note", Type: "typed-note",
				Properties: &runtime.RawExtension{Raw: []byte(props)}},
			ForEach: &wfTypesv1alpha1.ForEach{Items: &apiextensionsv1.JSON{Raw: []byte(items)}},
		}
	}

	BeforeEach(func() {
		namespaceName = randomNamespaceName("typed-reads-e2e")
		ns = createNamespace(ctx, namespaceName)
		applyDefinition(ctx, exprComponentDefinition(namespaceName, "typed-cm", typedConfigMap))
		applyDefinition(ctx, &v1beta1.WorkflowStepDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "typed-note", Namespace: namespaceName},
			Spec:       v1beta1.WorkflowStepDefinitionSpec{Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: textStep}}},
		})
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
	})

	It("refuses a component read whose type does not fit the parameter it feeds", func() {
		err := k8sClient.Create(ctx, app("mistyped", []oamcomm.ApplicationComponent{
			component("db", `{"data":{"host":"db.local"}}`),
			component("api", `{"count":"$(component.db.output.metadata.name)"}`),
		}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("spec.components[1].properties.count"))
		Expect(err.Error()).To(ContainSubstring("$(component.db.output.metadata.name) is string"))
		Expect(err.Error()).To(ContainSubstring("expects int"))
	})

	It("admits a read of a live field the template does not write, and resolves it", func() {
		Expect(k8sClient.Create(ctx, app("live-field", []oamcomm.ApplicationComponent{
			component("db", `{"data":{"host":"db.local"}}`),
			component("api", `{"data":{"uid":"$(component.db.output.metadata.uid)","host":"$(component.db.output.data.host)"}}`),
		}))).To(Succeed())
		db := &corev1.ConfigMap{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "db"}, db)
		}, 90*time.Second, time.Second).Should(Succeed())
		Eventually(func() map[string]string {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "api"}, cm); err != nil {
				return nil
			}
			return cm.Data
		}, 90*time.Second, time.Second).Should(Equal(map[string]string{"uid": string(db.UID), "host": "db.local"}))
	})

	It("refuses a loop item whose type does not fit the parameter it feeds", func() {
		err := k8sClient.Create(ctx, app("mistyped-loop",
			[]oamcomm.ApplicationComponent{component("db", `{"data":{}}`)},
			loop(`[1, 2]`, `{"text":"$(each.item)"}`)))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("spec.workflow.steps[0].properties.text"))
		Expect(err.Error()).To(ContainSubstring("$(each.item) is int"))
		Expect(err.Error()).To(ContainSubstring("expects string"))
	})

	It("admits loop items that fit, and text built from any item, and renders them", func() {
		Expect(k8sClient.Create(ctx, app("typed-loop",
			[]oamcomm.ApplicationComponent{component("typed-db", `{"data":{}}`)},
			loop(`[1, 2]`, `{"text":"wave $(each.item) of $(each.index)"}`)))).To(Succeed())
		Expect(k8sClient.Create(ctx, app("string-loop",
			[]oamcomm.ApplicationComponent{component("string-db", `{"data":{}}`)},
			loop(`["eu", "us"]`, `{"text":"$(each.item)"}`)))).To(Succeed())
		verifyApplicationPhase(ctx, namespaceName, "typed-loop", oamcomm.ApplicationRunning)
		verifyApplicationPhase(ctx, namespaceName, "string-loop", oamcomm.ApplicationRunning)
		for name, text := range map[string]string{
			"wave-1-of-0": "wave 1 of 0", "wave-2-of-1": "wave 2 of 1", "eu": "eu", "us": "us",
		} {
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: name}, cm)).To(Succeed(), name)
			Expect(cm.Data).To(Equal(map[string]string{"text": text}), name)
		}
	})
})
