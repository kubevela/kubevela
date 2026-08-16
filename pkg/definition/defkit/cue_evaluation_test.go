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

package defkit_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("Generated CUE helpers", func() {
	It("validates a generated component definition", func() {
		component := defkit.NewComponent("web").
			Workload("apps/v1", "Deployment").
			Params(defkit.String("image").Default("nginx")).
			Template(func(tpl *defkit.Template) {
				tpl.Output(defkit.NewResource("apps/v1", "Deployment").
					Set("spec.template.spec.containers", defkit.InlineArray(map[string]defkit.Value{
						"image": defkit.String("image"),
					})))
			})

		Expect(defkit.ValidateGeneratedCUE(component)).To(Succeed())
	})

	It("validates generated CUE with runtime context references", func() {
		component := defkit.NewComponent("contextual").
			Workload("v1", "ConfigMap").
			Template(func(tpl *defkit.Template) {
				tpl.Output(defkit.NewResource("v1", "ConfigMap").
					Set("metadata.namespace", defkit.VelaCtx().Namespace()))
			})

		Expect(defkit.ValidateGeneratedCUE(component)).To(Succeed())
	})

	It("reports malformed raw CUE with CUE diagnostics", func() {
		component := defkit.NewComponent("broken").RawCUE("broken: { value: ")

		err := defkit.ValidateGeneratedCUE(component)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("generated CUE for component \"broken\""))
		Expect(err.Error()).To(ContainSubstring("expected"))
	})

	It("validates and evaluates a dotted ItemBuilder path through to a concrete value", func() {
		// ItemBuilder.Set previously emitted a dotted path verbatim as a
		// single field label (e.g. "metadata.name: v"), which is invalid CUE.
		// The generator now expands it into nested fields; ValidateGeneratedCUE
		// and EvaluateCUE must both accept the result and evaluate it.
		items := defkit.Array("items").Of(defkit.ParamTypeString)
		component := defkit.NewComponent("named-items").
			Workload("v1", "ConfigMap").
			Params(items).
			Template(func(tpl *defkit.Template) {
				tpl.Output(defkit.NewResource("v1", "ConfigMap").
					Set("data.items", defkit.NewArray().ForEachWith(items, func(item *defkit.ItemBuilder) {
						item.Set("metadata.name", item.Var().Ref())
					})))
			})

		Expect(defkit.ValidateGeneratedCUE(component)).To(Succeed())

		outputs, err := component.EvaluateCUE(defkit.TestContext().WithParam("items", []any{"a", "b"}))
		Expect(err).NotTo(HaveOccurred())
		Expect(outputs.Primary["data"]).To(Equal(map[string]any{
			"items": []any{
				map[string]any{"metadata": map[string]any{"name": "a"}},
				map[string]any{"metadata": map[string]any{"name": "b"}},
			},
		}))
	})

	It("evaluates CUE defaults, fixtures, and context references", func() {
		replicas := defkit.Int("replicas").Default(2)
		component := defkit.NewComponent("web").
			Workload("apps/v1", "Deployment").
			Params(replicas).
			Template(func(tpl *defkit.Template) {
				tpl.Output(defkit.NewResource("apps/v1", "Deployment").
					Set("metadata.name", defkit.VelaCtx().Name()).
					Set("metadata.namespace", defkit.VelaCtx().Namespace()).
					Set("spec.replicas", replicas))
			})

		outputs, err := component.EvaluateCUE(defkit.TestContext().WithName("example").WithNamespace("production"))
		Expect(err).NotTo(HaveOccurred())
		Expect(outputs.Primary).To(Equal(map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":      "example",
				"namespace": "production",
			},
			"spec": map[string]any{"replicas": int64(2)},
		}))
	})

	It("evaluates auxiliary outputs and omits CUE-disabled outputs", func() {
		expose := defkit.Bool("expose").Default(false)
		component := defkit.NewComponent("web").
			Workload("apps/v1", "Deployment").
			Params(expose).
			Template(func(tpl *defkit.Template) {
				tpl.Output(defkit.NewResource("apps/v1", "Deployment"))
				tpl.OutputsIf(expose.IsTrue(), "service", defkit.NewResource("v1", "Service").Set("metadata.name", defkit.VelaCtx().Name()))
			})

		disabled, err := component.EvaluateCUE(defkit.TestContext())
		Expect(err).NotTo(HaveOccurred())
		Expect(disabled.Auxiliary).To(BeEmpty())

		enabled, err := component.EvaluateCUE(defkit.TestContext().WithName("example").WithParam("expose", true))
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled.Auxiliary).To(Equal(map[string]map[string]any{
			"service": {
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata":   map[string]any{"name": "example"},
			},
		}))
	})

	It("returns CUE validation errors for invalid fixtures", func() {
		replicas := defkit.Int("replicas").Min(1)
		component := defkit.NewComponent("web").
			Workload("apps/v1", "Deployment").
			Params(replicas).
			Template(func(tpl *defkit.Template) {
				tpl.Output(defkit.NewResource("apps/v1", "Deployment").Set("spec.replicas", replicas))
			})

		_, err := component.EvaluateCUE(defkit.TestContext().WithParam("replicas", 0))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("evaluate component \"web\" with the supplied fixtures"))
	})

	It("fills list fixtures before evaluating list validators", func() {
		items := defkit.Array("items").Of(defkit.ParamTypeString)
		component := defkit.NewComponent("validated-list").
			Workload("v1", "ConfigMap").
			Params(items).
			Validators(defkit.Validate("items must not be empty").
				FailWhen(defkit.LocalField("items").LenEq(0))).
			Template(func(tpl *defkit.Template) {
				tpl.Output(defkit.NewResource("v1", "ConfigMap").Set("data.items", items))
			})

		outputs, err := component.EvaluateCUE(defkit.TestContext().WithParam("items", []any{"item"}))
		Expect(err).NotTo(HaveOccurred())
		Expect(outputs.Primary["data"]).To(Equal(map[string]any{"items": []any{"item"}}))

		_, err = component.EvaluateCUE(defkit.TestContext().WithParam("items", []any{}))
		Expect(err).To(MatchError(ContainSubstring("items must not be empty")))
	})

	It("rejects a nil definition and a nil or receiverless evaluation request", func() {
		Expect(defkit.ValidateGeneratedCUE(nil)).To(MatchError(ContainSubstring("cannot validate a nil definition")))

		var nilComponent *defkit.ComponentDefinition
		_, err := nilComponent.EvaluateCUE(defkit.TestContext())
		Expect(err).To(MatchError(ContainSubstring("cannot evaluate a nil component")))

		component := defkit.NewComponent("web").Workload("apps/v1", "Deployment")
		_, err = component.EvaluateCUE(nil)
		Expect(err).To(MatchError(ContainSubstring(`cannot evaluate component "web" with a nil test context`)))
	})

	It("reports an error when the primary output is not concrete", func() {
		component := defkit.NewComponent("web").RawCUE(`template: {
	parameter: {}
	output: {
		apiVersion: "v1"
		kind: "ConfigMap"
		spec: replicas: int
	}
}`)

		_, err := component.EvaluateCUE(defkit.TestContext())
		Expect(err).To(MatchError(ContainSubstring(`evaluate primary output for component "web"`)))
	})

	It("reports an error when an auxiliary output cannot be decoded", func() {
		component := defkit.NewComponent("web").RawCUE(`template: {
	parameter: {}
	output: {
		apiVersion: "v1"
		kind: "ConfigMap"
	}
	outputs: {
		svc: [1, 2, 3]
	}
}`)

		_, err := component.EvaluateCUE(defkit.TestContext())
		Expect(err).To(MatchError(ContainSubstring(`evaluate auxiliary output "svc" for component "web"`)))
	})

	It("reports an error when auxiliary outputs is not a struct of resources", func() {
		component := defkit.NewComponent("web").RawCUE(`template: {
	parameter: {}
	output: {
		apiVersion: "v1"
		kind: "ConfigMap"
	}
	outputs: [1, 2, 3]
}`)

		_, err := component.EvaluateCUE(defkit.TestContext())
		Expect(err).To(MatchError(ContainSubstring(`enumerate auxiliary outputs for component "web"`)))
	})

	It("exposes simulated output and auxiliary output status through the context fixture", func() {
		component := defkit.NewComponent("web").RawCUE(`template: {
	parameter: {}
	output: {
		apiVersion: "v1"
		kind: "ConfigMap"
		status: context.output.status
	}
	outputs: {
		svc: {
			apiVersion: "v1"
			kind: "Service"
			status: context.outputs.svc.status
		}
	}
}`)

		outputs, err := component.EvaluateCUE(defkit.TestContext().
			WithOutputStatus(map[string]any{"ready": true}).
			WithOutputsStatus("svc", map[string]any{"ready": false}))
		Expect(err).NotTo(HaveOccurred())
		Expect(outputs.Primary["status"]).To(Equal(map[string]any{"ready": true}))
		Expect(outputs.Auxiliary["svc"]["status"]).To(Equal(map[string]any{"ready": false}))
	})
})
