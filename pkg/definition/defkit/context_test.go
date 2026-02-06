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

	Context("VelaCtx", func() {
		It("should create a context instance", func() {
			vela := defkit.VelaCtx()
			Expect(vela).NotTo(BeNil())
		})
	})

	Context("Basic Context References", func() {
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

	Context("ClusterVersion", func() {
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

	Context("Additional VelaContext Methods", func() {
		var vela *defkit.VelaContext

		BeforeEach(func() {
			vela = defkit.VelaCtx()
		})

		It("should return revision reference", func() {
			ref := vela.Revision()
			Expect(ref.Path()).To(Equal("context.revision"))
		})

		It("should return output reference", func() {
			ref := vela.Output()
			Expect(ref.Path()).To(Equal("context.output"))
		})

		It("should return outputs reference with name", func() {
			ref := vela.Outputs("service")
			Expect(ref.Path()).To(Equal("context.outputs.service"))
		})

		It("should return outputs reference for configmap", func() {
			ref := vela.Outputs("configmap")
			Expect(ref.Path()).To(Equal("context.outputs.configmap"))
		})
	})

	Context("ContextRef String Method", func() {
		It("should return string representation for name", func() {
			vela := defkit.VelaCtx()
			ref := vela.Name()
			Expect(ref.String()).To(Equal("$(context.name)"))
		})

		It("should return string representation for namespace", func() {
			vela := defkit.VelaCtx()
			ref := vela.Namespace()
			Expect(ref.String()).To(Equal("$(context.namespace)"))
		})

		It("should return string representation for appName", func() {
			vela := defkit.VelaCtx()
			ref := vela.AppName()
			Expect(ref.String()).To(Equal("$(context.appName)"))
		})

		It("should return string representation for revision", func() {
			vela := defkit.VelaCtx()
			ref := vela.Revision()
			Expect(ref.String()).To(Equal("$(context.revision)"))
		})

		It("should return string representation for output", func() {
			vela := defkit.VelaCtx()
			ref := vela.Output()
			Expect(ref.String()).To(Equal("$(context.output)"))
		})

		It("should return string representation for outputs", func() {
			vela := defkit.VelaCtx()
			ref := vela.Outputs("service")
			Expect(ref.String()).To(Equal("$(context.outputs.service)"))
		})
	})

	Context("Context in Expressions", func() {
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

	Context("TestContextBuilder", func() {
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

	Context("Render", func() {
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

	Context("SetIf with IsSet", func() {
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

	Context("Literal values", func() {
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

	Context("Comparison conditions in Render", func() {
		It("should evaluate greater-than condition", func() {
			replicas := defkit.Int("replicas")

			comp := defkit.NewComponent("test").
				Params(replicas).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							SetIf(defkit.Gt(replicas, defkit.Lit(1)), "spec.strategy.type", defkit.Lit("RollingUpdate")),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("replicas", 3),
			)
			Expect(rendered.Get("spec.strategy.type")).To(Equal("RollingUpdate"))
		})

		It("should evaluate less-than condition", func() {
			replicas := defkit.Int("replicas")

			comp := defkit.NewComponent("test").
				Params(replicas).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							SetIf(defkit.Lt(replicas, defkit.Lit(5)), "spec.minReplicas", replicas),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("replicas", 3),
			)
			Expect(rendered.Get("spec.minReplicas")).To(Equal(3))
		})

		It("should evaluate greater-than-or-equal condition", func() {
			port := defkit.Int("port")

			comp := defkit.NewComponent("test").
				Params(port).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "Service").
							SetIf(defkit.Ge(port, defkit.Lit(8080)), "spec.ports[0].port", port),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("port", 8080),
			)
			Expect(rendered.Get("spec.ports[0].port")).To(Equal(8080))
		})

		It("should evaluate less-than-or-equal condition", func() {
			replicas := defkit.Int("replicas")

			comp := defkit.NewComponent("test").
				Params(replicas).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							SetIf(defkit.Le(replicas, defkit.Lit(10)), "spec.replicas", replicas),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("replicas", 10),
			)
			Expect(rendered.Get("spec.replicas")).To(Equal(10))
		})

		It("should evaluate not-equal condition", func() {
			env := defkit.String("env")

			comp := defkit.NewComponent("test").
				Params(env).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							SetIf(defkit.Ne(env, defkit.Lit("prod")), "spec.debug", defkit.Lit(true)),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("env", "dev"),
			)
			Expect(rendered.Get("spec.debug")).To(Equal(true))
		})
	})

	Context("Numeric value comparisons", func() {
		It("should compare float values", func() {
			threshold := defkit.Float("threshold")

			comp := defkit.NewComponent("test").
				Params(threshold).
				Template(func(tpl *defkit.Template) {
					cond := defkit.Gt(threshold, defkit.Lit(0.5))
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							SetIf(cond, "data.highThreshold", defkit.Lit("true")),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("threshold", 0.8),
			)
			Expect(rendered.Get("data.highThreshold")).To(Equal("true"))
		})

		It("should compare int values with different operators", func() {
			priority := defkit.Int("priority")

			comp := defkit.NewComponent("test").
				Params(priority).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							SetIf(defkit.Ge(priority, defkit.Lit(1)), "data.low", defkit.Lit("yes")).
							SetIf(defkit.Ge(priority, defkit.Lit(5)), "data.medium", defkit.Lit("yes")).
							SetIf(defkit.Ge(priority, defkit.Lit(8)), "data.high", defkit.Lit("yes")),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("priority", 6),
			)
			Expect(rendered.Get("data.low")).To(Equal("yes"))
			Expect(rendered.Get("data.medium")).To(Equal("yes"))
			Expect(rendered.Get("data.high")).To(BeNil())
		})
	})

	Context("Logical conditions in Render", func() {
		It("should evaluate And condition", func() {
			replicas := defkit.Int("replicas")
			cpu := defkit.String("cpu")

			comp := defkit.NewComponent("test").
				Params(replicas, cpu).
				Template(func(tpl *defkit.Template) {
					cond := defkit.And(
						defkit.Gt(replicas, defkit.Lit(1)),
						cpu.IsSet(),
					)
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							SetIf(cond, "spec.resources.limits.cpu", cpu),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().
					WithParam("replicas", 3).
					WithParam("cpu", "500m"),
			)
			Expect(rendered.Get("spec.resources.limits.cpu")).To(Equal("500m"))
		})

		It("should evaluate Or condition", func() {
			debug := defkit.Bool("debug")
			env := defkit.String("env")

			comp := defkit.NewComponent("test").
				Params(debug, env).
				Template(func(tpl *defkit.Template) {
					cond := defkit.Or(
						defkit.Eq(debug, defkit.Lit(true)),
						defkit.Eq(env, defkit.Lit("dev")),
					)
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							SetIf(cond, "spec.logging", defkit.Lit("verbose")),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().
					WithParam("debug", false).
					WithParam("env", "dev"),
			)
			Expect(rendered.Get("spec.logging")).To(Equal("verbose"))
		})

		It("should evaluate Not condition", func() {
			enabled := defkit.Bool("enabled")

			comp := defkit.NewComponent("test").
				Params(enabled).
				Template(func(tpl *defkit.Template) {
					cond := defkit.Not(defkit.Eq(enabled, defkit.Lit(true)))
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							SetIf(cond, "spec.paused", defkit.Lit(true)),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("enabled", false),
			)
			Expect(rendered.Get("spec.paused")).To(Equal(true))
		})

		It("should evaluate nested And/Or conditions", func() {
			replicas := defkit.Int("replicas")
			cpu := defkit.String("cpu")
			memory := defkit.String("memory")

			comp := defkit.NewComponent("test").
				Params(replicas, cpu, memory).
				Template(func(tpl *defkit.Template) {
					// (replicas > 1) AND (cpu set OR memory set)
					cond := defkit.And(
						defkit.Gt(replicas, defkit.Lit(1)),
						defkit.Or(cpu.IsSet(), memory.IsSet()),
					)
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							SetIf(cond, "spec.autoscaling", defkit.Lit(true)),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().
					WithParam("replicas", 3).
					WithParam("memory", "512Mi"),
			)
			Expect(rendered.Get("spec.autoscaling")).To(Equal(true))
		})
	})

	Context("If/EndIf blocks in Render", func() {
		It("should evaluate If block when condition is true", func() {
			enabled := defkit.Bool("enabled")
			port := defkit.Int("port")

			comp := defkit.NewComponent("test").
				Params(enabled, port).
				Template(func(tpl *defkit.Template) {
					cond := defkit.Eq(enabled, defkit.Lit(true))
					tpl.Output(
						defkit.NewResource("v1", "Service").
							Set("metadata.name", defkit.VelaCtx().Name()).
							If(cond).
							Set("spec.ports[0].port", port).
							Set("spec.ports[0].protocol", defkit.Lit("TCP")).
							EndIf(),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().
					WithName("my-svc").
					WithParam("enabled", true).
					WithParam("port", 8080),
			)

			Expect(rendered.Get("metadata.name")).To(Equal("my-svc"))
			Expect(rendered.Get("spec.ports[0].port")).To(Equal(8080))
			Expect(rendered.Get("spec.ports[0].protocol")).To(Equal("TCP"))
		})

		It("should skip If block when condition is false", func() {
			enabled := defkit.Bool("enabled")
			port := defkit.Int("port")

			comp := defkit.NewComponent("test").
				Params(enabled, port).
				Template(func(tpl *defkit.Template) {
					cond := defkit.Eq(enabled, defkit.Lit(true))
					tpl.Output(
						defkit.NewResource("v1", "Service").
							Set("metadata.name", defkit.VelaCtx().Name()).
							If(cond).
							Set("spec.ports[0].port", port).
							EndIf(),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().
					WithName("my-svc").
					WithParam("enabled", false).
					WithParam("port", 8080),
			)

			Expect(rendered.Get("metadata.name")).To(Equal("my-svc"))
			Expect(rendered.Get("spec.ports[0].port")).To(BeNil())
		})
	})

	Context("Array indexing in Render", func() {
		It("should set values at array indices", func() {
			ports := defkit.Array("ports")

			comp := defkit.NewComponent("test").
				Params(ports).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "Service").
							Set("spec.ports[0].port", defkit.Lit(80)).
							Set("spec.ports[0].name", defkit.Lit("http")).
							Set("spec.ports[1].port", defkit.Lit(443)).
							Set("spec.ports[1].name", defkit.Lit("https")),
					)
				})

			rendered := comp.Render(defkit.TestContext())

			Expect(rendered.Get("spec.ports[0].port")).To(Equal(80))
			Expect(rendered.Get("spec.ports[0].name")).To(Equal("http"))
			Expect(rendered.Get("spec.ports[1].port")).To(Equal(443))
			Expect(rendered.Get("spec.ports[1].name")).To(Equal("https"))
		})
	})

	Context("Context references in Render", func() {
		It("should resolve appRevision context ref", func() {
			comp := defkit.NewComponent("test").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("metadata.name", defkit.VelaCtx().Name()).
							Set("metadata.labels.app", defkit.VelaCtx().AppName()).
							Set("metadata.labels.revision", defkit.VelaCtx().AppRevision()),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().
					WithName("my-config").
					WithAppName("my-app"),
			)

			Expect(rendered.Get("metadata.name")).To(Equal("my-config"))
			Expect(rendered.Get("metadata.labels.app")).To(Equal("my-app"))
		})

		It("should resolve namespace context ref", func() {
			comp := defkit.NewComponent("test").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("metadata.namespace", defkit.VelaCtx().Namespace()),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithNamespace("prod"),
			)

			Expect(rendered.Get("metadata.namespace")).To(Equal("prod"))
		})
	})

	Context("Different numeric types in comparison", func() {
		It("should compare int64 values", func() {
			count := defkit.Int("count")

			comp := defkit.NewComponent("test").
				Params(count).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							SetIf(defkit.Gt(count, defkit.Lit(int64(100))), "data.large", defkit.Lit("true")),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("count", int64(150)),
			)

			Expect(rendered.Get("data.large")).To(Equal("true"))
		})

		It("should compare float32 values", func() {
			ratio := defkit.Float("ratio")

			comp := defkit.NewComponent("test").
				Params(ratio).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							SetIf(defkit.Lt(ratio, defkit.Lit(float32(0.5))), "data.low", defkit.Lit("true")),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("ratio", float32(0.3)),
			)

			Expect(rendered.Get("data.low")).To(Equal("true"))
		})
	})

	Context("Boolean parameter conditions", func() {
		It("should evaluate boolean parameter directly", func() {
			debug := defkit.Bool("debug")

			comp := defkit.NewComponent("test").
				Params(debug).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							SetIf(debug.IsTrue(), "data.debug", defkit.Lit("enabled")),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("debug", true),
			)

			Expect(rendered.Get("data.debug")).To(Equal("enabled"))
		})

		It("should evaluate boolean false condition", func() {
			enabled := defkit.Bool("enabled")

			comp := defkit.NewComponent("test").
				Params(enabled).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							SetIf(enabled.IsFalse(), "data.disabled", defkit.Lit("true")),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("enabled", false),
			)

			Expect(rendered.Get("data.disabled")).To(Equal("true"))
		})
	})
})
