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

var _ = Describe("VelaContext", func() {

	Describe("VelaCtx", func() {
		It("should create a context instance", func() {
			vela := defkit.VelaCtx()
			Expect(vela).NotTo(BeNil())
		})
	})

	Describe("Basic Context References", func() {
		var vela *defkit.VelaContext

		BeforeEach(func() {
			vela = defkit.VelaCtx()
		})

		It("should return name reference", func() {
			ref := vela.Name()
			Expect(ref.Path()).To(Equal("context.name"))
		})

		It("should return namespace reference", func() {
			ref := vela.Namespace()
			Expect(ref.Path()).To(Equal("context.namespace"))
		})

		It("should return appName reference", func() {
			ref := vela.AppName()
			Expect(ref.Path()).To(Equal("context.appName"))
		})

		It("should return appRevision reference", func() {
			ref := vela.AppRevision()
			Expect(ref.Path()).To(Equal("context.appRevision"))
		})

		It("should return appRevisionNum reference", func() {
			ref := vela.AppRevisionNum()
			Expect(ref.Path()).To(Equal("context.appRevisionNum"))
		})
	})

	Describe("ClusterVersion", func() {
		var vela *defkit.VelaContext

		BeforeEach(func() {
			vela = defkit.VelaCtx()
		})

		It("should return cluster version reference", func() {
			ref := vela.ClusterVersion()
			Expect(ref.Path()).To(Equal("context.clusterVersion"))
		})

		It("should return major version reference", func() {
			ref := vela.ClusterVersion().Major()
			Expect(ref.Path()).To(Equal("context.clusterVersion.major"))
		})

		It("should return minor version reference", func() {
			ref := vela.ClusterVersion().Minor()
			Expect(ref.Path()).To(Equal("context.clusterVersion.minor"))
		})

		It("should return patch version reference", func() {
			ref := vela.ClusterVersion().Patch()
			Expect(ref.Path()).To(Equal("context.clusterVersion.patch"))
		})

		It("should return gitVersion reference", func() {
			ref := vela.ClusterVersion().GitVersion()
			Expect(ref.Path()).To(Equal("context.clusterVersion.gitVersion"))
		})
	})

	Describe("Context in Expressions", func() {
		It("should use context ref in comparison", func() {
			vela := defkit.VelaCtx()
			cond := defkit.Ge(vela.ClusterVersion().Minor(), defkit.Lit(21))
			Expect(cond.Left()).To(Equal(vela.ClusterVersion().Minor()))
		})

		It("should use context ref in resource Set", func() {
			vela := defkit.VelaCtx()
			r := defkit.NewResource("v1", "ConfigMap").
				Set("metadata.name", vela.Name()).
				Set("metadata.namespace", vela.Namespace())
			Expect(r.Ops()).To(HaveLen(2))
		})
	})
})

var _ = Describe("TestContext", func() {

	Describe("TestContextBuilder", func() {
		It("should create test context with defaults", func() {
			ctx := defkit.TestContext()
			Expect(ctx.Name()).To(Equal("test-component"))
			Expect(ctx.Namespace()).To(Equal("default"))
			Expect(ctx.AppName()).To(Equal("test-app"))
		})

		It("should set custom name", func() {
			ctx := defkit.TestContext().WithName("my-component")
			Expect(ctx.Name()).To(Equal("my-component"))
		})

		It("should set custom namespace", func() {
			ctx := defkit.TestContext().WithNamespace("production")
			Expect(ctx.Namespace()).To(Equal("production"))
		})

		It("should set app name", func() {
			ctx := defkit.TestContext().WithAppName("my-app")
			Expect(ctx.AppName()).To(Equal("my-app"))
		})

		It("should set single parameter", func() {
			ctx := defkit.TestContext().WithParam("replicas", 3)
			Expect(ctx.Params()["replicas"]).To(Equal(3))
		})

		It("should set multiple parameters", func() {
			ctx := defkit.TestContext().WithParams(map[string]any{
				"image":    "nginx:latest",
				"replicas": 5,
				"port":     8080,
			})
			Expect(ctx.Params()["image"]).To(Equal("nginx:latest"))
			Expect(ctx.Params()["replicas"]).To(Equal(5))
			Expect(ctx.Params()["port"]).To(Equal(8080))
		})

		It("should set cluster version", func() {
			ctx := defkit.TestContext().WithClusterVersion(1, 30)
			major, minor := ctx.ClusterVersion()
			Expect(major).To(Equal(1))
			Expect(minor).To(Equal(30))
		})

		It("should chain configuration methods", func() {
			ctx := defkit.TestContext().
				WithName("web").
				WithNamespace("prod").
				WithParam("image", "nginx").
				WithParam("replicas", 3).
				WithClusterVersion(1, 28)

			Expect(ctx.Name()).To(Equal("web"))
			Expect(ctx.Namespace()).To(Equal("prod"))
			Expect(ctx.Params()["image"]).To(Equal("nginx"))
			Expect(ctx.Params()["replicas"]).To(Equal(3))
			major, minor := ctx.ClusterVersion()
			Expect(major).To(Equal(1))
			Expect(minor).To(Equal(28))
		})
	})

	Describe("Render", func() {
		var (
			image    *defkit.StringParam
			replicas *defkit.IntParam
			port     *defkit.IntParam
			comp     *defkit.ComponentDefinition
		)

		BeforeEach(func() {
			image = defkit.String("image").Required()
			replicas = defkit.Int("replicas").Default(1)
			port = defkit.Int("port").Default(80)

			comp = defkit.NewComponent("webservice").
				Description("Web service component").
				Workload("apps/v1", "Deployment").
				Params(image, replicas, port).
				Template(func(tpl *defkit.Template) {
					vela := defkit.VelaCtx()
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", vela.Name()).
							Set("spec.replicas", replicas).
							Set("spec.template.spec.containers[0].image", image).
							Set("spec.template.spec.containers[0].ports[0].containerPort", port),
					)
					tpl.Outputs("service",
						defkit.NewResource("v1", "Service").
							Set("metadata.name", vela.Name()).
							Set("spec.ports[0].port", port),
					)
				})
		})

		It("should render component with parameter values", func() {
			rendered := comp.Render(
				defkit.TestContext().
					WithName("my-web").
					WithParam("image", "nginx:1.21").
					WithParam("replicas", 3).
					WithParam("port", 8080),
			)

			Expect(rendered.APIVersion()).To(Equal("apps/v1"))
			Expect(rendered.Kind()).To(Equal("Deployment"))
			Expect(rendered.Get("metadata.name")).To(Equal("my-web"))
			Expect(rendered.Get("spec.replicas")).To(Equal(3))
			Expect(rendered.Get("spec.template.spec.containers[0].image")).To(Equal("nginx:1.21"))
			Expect(rendered.Get("spec.template.spec.containers[0].ports[0].containerPort")).To(Equal(8080))
		})

		It("should use default values when parameters not set", func() {
			rendered := comp.Render(
				defkit.TestContext().
					WithName("default-web").
					WithParam("image", "nginx:latest"),
			)

			Expect(rendered.Get("spec.replicas")).To(Equal(1))
			Expect(rendered.Get("spec.template.spec.containers[0].ports[0].containerPort")).To(Equal(80))
		})

		It("should render all outputs", func() {
			outputs := comp.RenderAll(
				defkit.TestContext().
					WithName("my-web").
					WithParam("image", "nginx:latest").
					WithParam("port", 9000),
			)

			Expect(outputs.Primary.Kind()).To(Equal("Deployment"))
			Expect(outputs.Auxiliary).To(HaveKey("service"))
			Expect(outputs.Auxiliary["service"].Kind()).To(Equal("Service"))
			Expect(outputs.Auxiliary["service"].Get("spec.ports[0].port")).To(Equal(9000))
		})

		It("should resolve context references", func() {
			rendered := comp.Render(
				defkit.TestContext().
					WithName("context-test").
					WithNamespace("test-ns").
					WithParam("image", "test"),
			)

			Expect(rendered.Get("metadata.name")).To(Equal("context-test"))
		})
	})

	Describe("SetIf with IsSet", func() {
		It("should set field when parameter is set", func() {
			cpu := defkit.String("cpu").Optional()

			comp := defkit.NewComponent("test").
				Params(cpu).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "Pod").
							SetIf(cpu.IsSet(), "spec.resources.limits.cpu", cpu),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("cpu", "500m"),
			)

			Expect(rendered.Get("spec.resources.limits.cpu")).To(Equal("500m"))
		})

		It("should not set field when parameter is not set", func() {
			cpu := defkit.String("cpu").Optional()

			comp := defkit.NewComponent("test").
				Params(cpu).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "Pod").
							SetIf(cpu.IsSet(), "spec.resources.limits.cpu", cpu),
					)
				})

			rendered := comp.Render(defkit.TestContext())

			Expect(rendered.Get("spec.resources.limits.cpu")).To(BeNil())
		})
	})

	Describe("Literal values", func() {
		It("should set literal string values", func() {
			comp := defkit.NewComponent("test").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("data.key", defkit.Lit("literal-value")),
					)
				})

			rendered := comp.Render(defkit.TestContext())
			Expect(rendered.Get("data.key")).To(Equal("literal-value"))
		})

		It("should set literal int values", func() {
			comp := defkit.NewComponent("test").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.replicas", defkit.Lit(5)),
					)
				})

			rendered := comp.Render(defkit.TestContext())
			Expect(rendered.Get("spec.replicas")).To(Equal(5))
		})
	})
})
