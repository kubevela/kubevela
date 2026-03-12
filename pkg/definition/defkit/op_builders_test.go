/*
Copyright 2025 The KubeVela Authors.

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

// simpleRenderValue is a test helper that renders Values the same way cuegen does for Ref types.
func simpleRenderValue(v defkit.Value) string {
	if cr, ok := v.(defkit.CUERenderer); ok {
		return cr.RenderCUE(simpleRenderValue)
	}
	// For *Ref, use the Path() method via the interface trick
	type pather interface{ Path() string }
	if p, ok := v.(pather); ok {
		return p.Path()
	}
	return "_"
}

var _ = Describe("Operation Builders", func() {

	Describe("KubeRead", func() {
		It("should render a basic kube.#Read with name and namespace", func() {
			b := defkit.KubeRead("v1", "Secret").
				Name(defkit.Reference("parameter.name")).
				Namespace(defkit.Reference("context.namespace"))

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).To(ContainSubstring("kube.#Read & {"))
			Expect(cue).To(ContainSubstring(`apiVersion: "v1"`))
			Expect(cue).To(ContainSubstring(`kind:       "Secret"`))
			Expect(cue).To(ContainSubstring("name:      parameter.name"))
			Expect(cue).To(ContainSubstring("namespace: context.namespace"))
			Expect(cue).To(ContainSubstring("$params: value: {"))
		})

		It("should render kube.#Read with cluster parameter", func() {
			b := defkit.KubeRead("v1", "ConfigMap").
				Name(defkit.Reference("parameter.name")).
				Namespace(defkit.Reference("context.namespace")).
				Cluster(defkit.Reference("parameter.cluster"))

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).To(ContainSubstring("kube.#Read & {"))
			Expect(cue).To(ContainSubstring("cluster: parameter.cluster"))
			Expect(cue).To(ContainSubstring(`apiVersion: "v1"`))
			Expect(cue).To(ContainSubstring(`kind:       "ConfigMap"`))
			// When cluster is set, $params uses expanded form
			Expect(cue).NotTo(ContainSubstring("$params: value: {"))
			Expect(cue).To(ContainSubstring("$params: {"))
		})

		It("should render without namespace when not set", func() {
			b := defkit.KubeRead("apps/v1", "Deployment").
				Name(defkit.Reference("parameter.name"))

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).To(ContainSubstring("name:      parameter.name"))
			Expect(cue).NotTo(ContainSubstring("namespace:"))
		})

		It("should render with different apiVersion and kind", func() {
			b := defkit.KubeRead("core.oam.dev/v1beta1", "Application").
				Name(defkit.Reference("context.name")).
				Namespace(defkit.Reference("context.namespace"))

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).To(ContainSubstring(`apiVersion: "core.oam.dev/v1beta1"`))
			Expect(cue).To(ContainSubstring(`kind:       "Application"`))
		})

		It("should implement Value interface", func() {
			var v defkit.Value = defkit.KubeRead("v1", "Pod").
				Name(defkit.Reference("name"))
			Expect(v).NotTo(BeNil())
		})
	})

	Describe("KubeApply", func() {
		It("should render a basic kube.#Apply", func() {
			b := defkit.KubeApply(defkit.Reference("deployment"))

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).To(ContainSubstring("kube.#Apply & {"))
			Expect(cue).To(ContainSubstring("value:   deployment"))
			Expect(cue).To(ContainSubstring("$params: {"))
		})

		It("should render kube.#Apply with cluster", func() {
			b := defkit.KubeApply(defkit.Reference("configMap")).
				Cluster(defkit.Reference("parameter.cluster"))

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).To(ContainSubstring("value:   configMap"))
			Expect(cue).To(ContainSubstring("cluster: parameter.cluster"))
		})

		It("should render kube.#Apply without cluster", func() {
			b := defkit.KubeApply(defkit.Reference("jobValue"))

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).NotTo(ContainSubstring("cluster:"))
		})

		It("should implement Value interface", func() {
			var v defkit.Value = defkit.KubeApply(defkit.Reference("obj"))
			Expect(v).NotTo(BeNil())
		})
	})

	Describe("HTTPPost", func() {
		It("should render a basic POST request", func() {
			b := defkit.HTTPPost(defkit.Reference("parameter.url")).
				Body(defkit.Reference("data.value")).
				Header("Content-Type", "application/json")

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).To(ContainSubstring("http.#HTTPDo & {"))
			Expect(cue).To(ContainSubstring(`method: "POST"`))
			Expect(cue).To(ContainSubstring("url:    parameter.url"))
			Expect(cue).To(ContainSubstring("body: data.value"))
			Expect(cue).To(ContainSubstring(`header: "Content-Type": "application/json"`))
		})

		It("should render without body when not set", func() {
			b := defkit.HTTPPost(defkit.Reference("parameter.url")).
				Header("Content-Type", "application/json")

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).NotTo(ContainSubstring("body:"))
			Expect(cue).To(ContainSubstring("request: {"))
		})

		It("should render with computed URL", func() {
			b := defkit.HTTPPost(defkit.Reference("stringValue.$returns.str")).
				Body(defkit.Reference("data.value")).
				Header("Content-Type", "application/json")

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).To(ContainSubstring("url:    stringValue.$returns.str"))
		})

		It("should implement Value interface", func() {
			var v defkit.Value = defkit.HTTPPost(defkit.Reference("url"))
			Expect(v).NotTo(BeNil())
		})
	})

	Describe("ConvertString", func() {
		It("should render util.#ConvertString", func() {
			b := defkit.ConvertString(
				defkit.Reference("base64.Decode(null, read.$returns.value.data[key])"),
			)

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).To(ContainSubstring("util.#ConvertString & {"))
			Expect(cue).To(ContainSubstring("$params: bt: base64.Decode(null, read.$returns.value.data[key])"))
		})

		It("should implement Value interface", func() {
			var v defkit.Value = defkit.ConvertString(defkit.Reference("input"))
			Expect(v).NotTo(BeNil())
		})
	})

	Describe("WaitUntil", func() {
		It("should render simple conditional wait", func() {
			b := defkit.WaitUntil(
				defkit.Reference("output.$returns.value.status.readyReplicas == parameter.replicas"),
			)

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).To(ContainSubstring("builtin.#ConditionalWait & {"))
			Expect(cue).To(ContainSubstring("$params: continue: output.$returns.value.status.readyReplicas == parameter.replicas"))
			Expect(cue).NotTo(ContainSubstring("if "))
		})

		It("should render with single guard", func() {
			b := defkit.WaitUntil(
				defkit.Reference(`apply.$returns.value.status.apply.state == "Available"`),
			).Guard(
				defkit.Reference("apply.$returns.value.status"),
			)

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).To(ContainSubstring("if apply.$returns.value.status != _|_"))
			Expect(cue).To(ContainSubstring(`$params: continue: apply.$returns.value.status.apply.state == "Available"`))
		})

		It("should render with chained guards", func() {
			b := defkit.WaitUntil(
				defkit.Reference(`apply.$returns.value.status.apply.state == "Available"`),
			).Guard(
				defkit.Reference("apply.$returns.value.status"),
				defkit.Reference("apply.$returns.value.status.apply"),
			)

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).To(ContainSubstring("if apply.$returns.value.status != _|_ if apply.$returns.value.status.apply != _|_"))
		})

		It("should implement Value interface", func() {
			var v defkit.Value = defkit.WaitUntil(defkit.Reference("true"))
			Expect(v).NotTo(BeNil())
		})
	})

	Describe("Fail", func() {
		It("should render builtin.#Fail with message", func() {
			b := defkit.Fail(defkit.Reference("check.$returns.message"))

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).To(ContainSubstring("builtin.#Fail & {"))
			Expect(cue).To(ContainSubstring("$params: message: check.$returns.message"))
		})

		It("should render with literal message", func() {
			b := defkit.Fail(defkit.Reference(`"failed to execute command"`))

			cue := b.RenderCUE(simpleRenderValue)

			Expect(cue).To(ContainSubstring(`$params: message: "failed to execute command"`))
		})

		It("should implement Value interface", func() {
			var v defkit.Value = defkit.Fail(defkit.Reference("msg"))
			Expect(v).NotTo(BeNil())
		})
	})

	Describe("CUERenderer integration via valueToCUE", func() {
		It("should render KubeRead through CUE generator", func() {
			ws := defkit.NewWorkflowStep("test").
				Params(defkit.String("name").Required()).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.Set("output", defkit.KubeRead("v1", "Pod").
						Name(defkit.Reference("parameter.name")).
						Namespace(defkit.Reference("context.namespace")),
					)
				})

			cue := ws.ToCue()

			Expect(cue).To(ContainSubstring("kube.#Read & {"))
			Expect(cue).To(ContainSubstring(`apiVersion: "v1"`))
			Expect(cue).To(ContainSubstring(`kind:       "Pod"`))
			Expect(cue).To(ContainSubstring("name:      parameter.name"))
			Expect(cue).To(ContainSubstring("namespace: context.namespace"))
		})

		It("should render HTTPPost through CUE generator", func() {
			ws := defkit.NewWorkflowStep("test").
				Params(defkit.String("url").Required()).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.Set("req", defkit.HTTPPost(defkit.Reference("parameter.url")).
						Body(defkit.Reference("data.value")).
						Header("Content-Type", "application/json"),
					)
				})

			cue := ws.ToCue()

			Expect(cue).To(ContainSubstring("http.#HTTPDo & {"))
			Expect(cue).To(ContainSubstring(`method: "POST"`))
			Expect(cue).To(ContainSubstring("url:    parameter.url"))
		})

		It("should render ConvertString through CUE generator", func() {
			ws := defkit.NewWorkflowStep("test").
				Params(defkit.String("name").Required()).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.Set("convert", defkit.ConvertString(
						defkit.Reference("base64.Decode(null, data)"),
					))
				})

			cue := ws.ToCue()

			Expect(cue).To(ContainSubstring("util.#ConvertString & {"))
			Expect(cue).To(ContainSubstring("$params: bt: base64.Decode(null, data)"))
		})
	})
})
