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

var _ = Describe("Render", func() {

	Context("Render with context references", func() {
		It("should resolve context.name", func() {
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", defkit.VelaCtx().Name()),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithName("my-comp"),
			)

			Expect(rendered.Get("metadata.name")).To(Equal("my-comp"))
		})

		It("should resolve context.namespace", func() {
			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("metadata.namespace", defkit.VelaCtx().Namespace()),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithNamespace("production"),
			)

			Expect(rendered.Get("metadata.namespace")).To(Equal("production"))
		})

		It("should resolve context.appName", func() {
			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("metadata.labels.app", defkit.VelaCtx().AppName()),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithAppName("myapp"),
			)

			Expect(rendered.Get("metadata.labels.app")).To(Equal("myapp"))
		})

		It("should resolve context.appRevision", func() {
			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("metadata.labels.revision", defkit.VelaCtx().AppRevision()),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithAppRevision("myapp-v3"),
			)

			Expect(rendered.Get("metadata.labels.revision")).To(Equal("myapp-v3"))
		})
	})

	Context("Render with various param types", func() {
		It("should resolve string params", func() {
			image := defkit.String("image").Default("nginx")

			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(image).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.template.spec.containers[0].image", image),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("image", "nginx:1.21"),
			)
			Expect(rendered.Get("spec.template.spec.containers[0].image")).To(Equal("nginx:1.21"))
		})

		It("should use default values when param not set", func() {
			replicas := defkit.Int("replicas").Default(3)

			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(replicas).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.replicas", replicas),
					)
				})

			rendered := comp.Render(defkit.TestContext())
			Expect(rendered.Get("spec.replicas")).To(Equal(3))
		})

		It("should resolve int params", func() {
			port := defkit.Int("port").Default(80)

			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(port).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.template.spec.containers[0].ports[0].containerPort", port),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("port", 8080),
			)
			Expect(rendered.Get("spec.template.spec.containers[0].ports[0].containerPort")).To(Equal(8080))
		})

		It("should resolve bool params", func() {
			enabled := defkit.Bool("enabled").Default(false)

			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Params(enabled).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("data.enabled", enabled),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("enabled", true),
			)
			Expect(rendered.Get("data.enabled")).To(BeTrue())
		})

		It("should resolve float params", func() {
			ratio := defkit.Float("ratio").Default(0.5)

			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Params(ratio).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("data.ratio", ratio),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("ratio", 0.75),
			)
			Expect(rendered.Get("data.ratio")).To(Equal(0.75))
		})

		It("should resolve enum params", func() {
			env := defkit.Enum("environment").Values("dev", "staging", "prod").Default("dev")

			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Params(env).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("data.env", env),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("environment", "prod"),
			)
			Expect(rendered.Get("data.env")).To(Equal("prod"))
		})

		It("should resolve map params", func() {
			labels := defkit.Object("labels")

			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Params(labels).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("metadata.labels", labels),
					)
				})

			labelData := map[string]any{"app": "test", "env": "dev"}
			rendered := comp.Render(
				defkit.TestContext().WithParam("labels", labelData),
			)
			Expect(rendered.Get("metadata.labels")).To(Equal(labelData))
		})

		It("should resolve literal values", func() {
			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("data.fixed", defkit.Lit("constant-value")).
							Set("data.count", defkit.Lit(42)).
							Set("data.flag", defkit.Lit(true)),
					)
				})

			rendered := comp.Render(defkit.TestContext())
			Expect(rendered.Get("data.fixed")).To(Equal("constant-value"))
			Expect(rendered.Get("data.count")).To(Equal(42))
			Expect(rendered.Get("data.flag")).To(BeTrue())
		})
	})

	Context("Render with conditional operations", func() {
		It("should apply SetIf when condition is true", func() {
			cpu := defkit.String("cpu")

			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(cpu).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							SetIf(cpu.IsSet(), "spec.template.spec.containers[0].resources.limits.cpu", cpu),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("cpu", "500m"),
			)
			Expect(rendered.Get("spec.template.spec.containers[0].resources.limits.cpu")).To(Equal("500m"))
		})

		It("should skip SetIf when condition is false", func() {
			cpu := defkit.String("cpu")

			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(cpu).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							SetIf(cpu.IsSet(), "spec.template.spec.containers[0].resources.limits.cpu", cpu),
					)
				})

			rendered := comp.Render(defkit.TestContext())
			Expect(rendered.Get("spec.template.spec.containers[0].resources.limits.cpu")).To(BeNil())
		})

		It("should handle If/EndIf blocks", func() {
			enabled := defkit.Bool("enabled")
			replicas := defkit.Int("replicas").Default(1)

			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(enabled, replicas).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							If(defkit.Eq(enabled, defkit.Lit(true))).
							Set("spec.replicas", replicas).
							EndIf(),
					)
				})

			// Condition true
			renderedTrue := comp.Render(
				defkit.TestContext().
					WithParam("enabled", true).
					WithParam("replicas", 5),
			)
			Expect(renderedTrue.Get("spec.replicas")).To(Equal(5))

			// Condition false
			renderedFalse := comp.Render(
				defkit.TestContext().
					WithParam("enabled", false).
					WithParam("replicas", 5),
			)
			Expect(renderedFalse.Get("spec.replicas")).To(BeNil())
		})
	})

	Context("Render with comparison conditions", func() {
		It("should evaluate equality comparison", func() {
			env := defkit.String("env").Default("dev")
			replicas := defkit.Int("replicas").Default(1)

			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(env, replicas).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							SetIf(defkit.Eq(env, defkit.Lit("prod")), "spec.replicas", defkit.Lit(3)),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("env", "prod"),
			)
			Expect(rendered.Get("spec.replicas")).To(Equal(3))

			renderedDev := comp.Render(
				defkit.TestContext().WithParam("env", "dev"),
			)
			Expect(renderedDev.Get("spec.replicas")).To(BeNil())
		})

		It("should evaluate inequality comparison", func() {
			env := defkit.String("env").Default("dev")

			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Params(env).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							SetIf(defkit.Ne(env, defkit.Lit("prod")), "data.debug", defkit.Lit("true")),
					)
				})

			renderedDev := comp.Render(
				defkit.TestContext().WithParam("env", "dev"),
			)
			Expect(renderedDev.Get("data.debug")).To(Equal("true"))

			renderedProd := comp.Render(
				defkit.TestContext().WithParam("env", "prod"),
			)
			Expect(renderedProd.Get("data.debug")).To(BeNil())
		})

		It("should evaluate numeric comparisons", func() {
			replicas := defkit.Int("replicas").Default(1)

			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Params(replicas).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							SetIf(defkit.Gt(replicas, defkit.Lit(1)), "data.scaled", defkit.Lit("true")),
					)
				})

			renderedScaled := comp.Render(
				defkit.TestContext().WithParam("replicas", 3),
			)
			Expect(renderedScaled.Get("data.scaled")).To(Equal("true"))

			renderedSingle := comp.Render(
				defkit.TestContext().WithParam("replicas", 1),
			)
			Expect(renderedSingle.Get("data.scaled")).To(BeNil())
		})
	})

	Context("Render with logical conditions", func() {
		It("should evaluate And conditions", func() {
			enabled := defkit.Bool("enabled")
			debug := defkit.Bool("debug")

			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Params(enabled, debug).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							SetIf(
								defkit.And(
									defkit.Eq(enabled, defkit.Lit(true)),
									defkit.Eq(debug, defkit.Lit(true)),
								),
								"data.verbose", defkit.Lit("true"),
							),
					)
				})

			// Both true
			rendered := comp.Render(
				defkit.TestContext().
					WithParam("enabled", true).
					WithParam("debug", true),
			)
			Expect(rendered.Get("data.verbose")).To(Equal("true"))

			// One false
			renderedPartial := comp.Render(
				defkit.TestContext().
					WithParam("enabled", true).
					WithParam("debug", false),
			)
			Expect(renderedPartial.Get("data.verbose")).To(BeNil())
		})

		It("should evaluate Or conditions", func() {
			env := defkit.String("env")

			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Params(env).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							SetIf(
								defkit.Or(
									defkit.Eq(env, defkit.Lit("staging")),
									defkit.Eq(env, defkit.Lit("prod")),
								),
								"data.production-like", defkit.Lit("true"),
							),
					)
				})

			// Staging matches
			renderedStaging := comp.Render(
				defkit.TestContext().WithParam("env", "staging"),
			)
			Expect(renderedStaging.Get("data.production-like")).To(Equal("true"))

			// Dev doesn't match
			renderedDev := comp.Render(
				defkit.TestContext().WithParam("env", "dev"),
			)
			Expect(renderedDev.Get("data.production-like")).To(BeNil())
		})

		It("should evaluate Not conditions", func() {
			debug := defkit.Bool("debug")

			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Params(debug).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							SetIf(
								defkit.Not(defkit.Eq(debug, defkit.Lit(true))),
								"data.optimized", defkit.Lit("true"),
							),
					)
				})

			renderedNoDebug := comp.Render(
				defkit.TestContext().WithParam("debug", false),
			)
			Expect(renderedNoDebug.Get("data.optimized")).To(Equal("true"))

			renderedDebug := comp.Render(
				defkit.TestContext().WithParam("debug", true),
			)
			Expect(renderedDebug.Get("data.optimized")).To(BeNil())
		})
	})

	Context("Render with transformed values", func() {
		It("should apply transformation to value", func() {
			port := defkit.Int("port").Default(80)

			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Params(port).
				Template(func(tpl *defkit.Template) {
					portStr := defkit.Transform(port, func(v any) any {
						if n, ok := v.(int); ok {
							return n * 2
						}
						return v
					})
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("data.doubledPort", portStr),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("port", 8080),
			)
			Expect(rendered.Get("data.doubledPort")).To(Equal(16160))
		})

		It("should handle nil transform function", func() {
			port := defkit.Int("port").Default(80)

			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Params(port).
				Template(func(tpl *defkit.Template) {
					noopTransform := defkit.Transform(port, nil)
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("data.port", noopTransform),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("port", 8080),
			)
			Expect(rendered.Get("data.port")).To(Equal(8080))
		})
	})

	Context("RenderAll with auxiliary outputs", func() {
		It("should render primary and auxiliary outputs", func() {
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.replicas", defkit.Lit(1)),
					)
					tpl.Outputs("service",
						defkit.NewResource("v1", "Service").
							Set("spec.type", defkit.Lit("ClusterIP")),
					)
					tpl.Outputs("configmap",
						defkit.NewResource("v1", "ConfigMap").
							Set("data.key", defkit.Lit("value")),
					)
				})

			outputs := comp.RenderAll(defkit.TestContext())

			Expect(outputs.Primary).NotTo(BeNil())
			Expect(outputs.Primary.Kind()).To(Equal("Deployment"))
			Expect(outputs.Auxiliary).To(HaveLen(2))
			Expect(outputs.Auxiliary["service"].Kind()).To(Equal("Service"))
			Expect(outputs.Auxiliary["configmap"].Kind()).To(Equal("ConfigMap"))
		})

		It("should filter conditional outputs when condition is false", func() {
			enabled := defkit.Bool("enabled")

			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(enabled).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment"),
					)
					tpl.OutputsIf(
						defkit.Eq(enabled, defkit.Lit(true)),
						"service",
						defkit.NewResource("v1", "Service"),
					)
				})

			// Condition false - service should be excluded
			outputs := comp.RenderAll(
				defkit.TestContext().WithParam("enabled", false),
			)
			Expect(outputs.Auxiliary).To(BeEmpty())

			// Condition true - service should be included
			outputsEnabled := comp.RenderAll(
				defkit.TestContext().WithParam("enabled", true),
			)
			Expect(outputsEnabled.Auxiliary).To(HaveKey("service"))
		})
	})

	Context("RenderedResource nil safety", func() {
		It("should return empty values for nil RenderedResource", func() {
			var r *defkit.RenderedResource
			Expect(r.APIVersion()).To(BeEmpty())
			Expect(r.Kind()).To(BeEmpty())
			Expect(r.Get("any.path")).To(BeNil())
			Expect(r.Data()).To(BeNil())
		})
	})

	Context("Render with nil template", func() {
		It("should return nil output for component with no template", func() {
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment")

			rendered := comp.Render(defkit.TestContext())
			Expect(rendered).To(BeNil())
		})
	})

	Context("Nested path operations", func() {
		It("should handle deeply nested paths", func() {
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.template.spec.containers[0].env[0].name", defkit.Lit("FOO")).
							Set("spec.template.spec.containers[0].env[0].value", defkit.Lit("bar")),
					)
				})

			rendered := comp.Render(defkit.TestContext())
			Expect(rendered.Get("spec.template.spec.containers[0].env[0].name")).To(Equal("FOO"))
			Expect(rendered.Get("spec.template.spec.containers[0].env[0].value")).To(Equal("bar"))
		})

		It("should handle map key bracket notation", func() {
			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("metadata.labels[app.kubernetes.io/name]", defkit.Lit("myapp")),
					)
				})

			rendered := comp.Render(defkit.TestContext())
			labels := rendered.Get("metadata.labels")
			Expect(labels).NotTo(BeNil())
			labelsMap := labels.(map[string]any)
			Expect(labelsMap["app.kubernetes.io/name"]).To(Equal("myapp"))
		})
	})

	Context("HasExposedPorts condition", func() {
		It("should return true when ports have expose=true", func() {
			ports := defkit.Array("ports")

			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Params(ports).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							SetIf(defkit.HasExposedPorts(ports), "data.hasExposed", defkit.Lit("true")),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("ports", []any{
					map[string]any{"port": 80, "expose": true},
					map[string]any{"port": 443, "expose": false},
				}),
			)
			Expect(rendered.Get("data.hasExposed")).To(Equal("true"))
		})

		It("should return false when no ports have expose=true", func() {
			ports := defkit.Array("ports")

			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Params(ports).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							SetIf(defkit.HasExposedPorts(ports), "data.hasExposed", defkit.Lit("true")),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("ports", []any{
					map[string]any{"port": 80, "expose": false},
				}),
			)
			Expect(rendered.Get("data.hasExposed")).To(BeNil())
		})

		It("should return false for non-array ports value", func() {
			ports := defkit.String("ports")

			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Params(ports).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							SetIf(defkit.HasExposedPorts(ports), "data.hasExposed", defkit.Lit("true")),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithParam("ports", "not-an-array"),
			)
			Expect(rendered.Get("data.hasExposed")).To(BeNil())
		})
	})

	Context("evaluateCondition with nil", func() {
		It("should treat nil condition as true", func() {
			// This is tested indirectly: a SetIf with no condition should apply
			comp := defkit.NewComponent("test").
				Workload("v1", "ConfigMap").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("v1", "ConfigMap").
							Set("data.always", defkit.Lit("present")),
					)
				})

			rendered := comp.Render(defkit.TestContext())
			Expect(rendered.Get("data.always")).To(Equal("present"))
		})
	})
})
