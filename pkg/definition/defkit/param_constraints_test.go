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

package defkit

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Parameter Constraints", func() {
	var gen *CUEGenerator

	ginkgo.BeforeEach(func() {
		gen = NewCUEGenerator()
	})

	// --- Schema Constraint Tests ---

	ginkgo.Context("Schema Constraints", func() {
		ginkgo.It("should set and generate string pattern constraint", func() {
			p := String("name").Pattern("^[a-z][a-z0-9-]*$")
			gomega.Expect(p.GetPattern()).To(gomega.Equal("^[a-z][a-z0-9-]*$"))

			comp := NewComponent("test").Params(p)
			cue := gen.GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring(`=~"^[a-z][a-z0-9-]*$"`))
		})

		ginkgo.It("should set and generate string min/max length constraints", func() {
			p := String("name").MinLen(3).MaxLen(63)
			gomega.Expect(p.GetMinLen()).ToNot(gomega.BeNil())
			gomega.Expect(*p.GetMinLen()).To(gomega.Equal(3))
			gomega.Expect(p.GetMaxLen()).ToNot(gomega.BeNil())
			gomega.Expect(*p.GetMaxLen()).To(gomega.Equal(63))

			comp := NewComponent("test").Params(p)
			cue := gen.GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring("strings.MinRunes(3)"))
			gomega.Expect(cue).To(gomega.ContainSubstring("strings.MaxRunes(63)"))
		})

		ginkgo.It("should set and generate int min/max constraints", func() {
			p := Int("replicas").Min(1).Max(100)
			gomega.Expect(p.GetMin()).ToNot(gomega.BeNil())
			gomega.Expect(*p.GetMin()).To(gomega.Equal(1))
			gomega.Expect(p.GetMax()).ToNot(gomega.BeNil())
			gomega.Expect(*p.GetMax()).To(gomega.Equal(100))

			comp := NewComponent("test").Params(p)
			cue := gen.GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring(">=1"))
			gomega.Expect(cue).To(gomega.ContainSubstring("<=100"))
		})

		ginkgo.It("should set and generate float min/max constraints", func() {
			p := Float("ratio").Min(0.0).Max(1.0)
			gomega.Expect(p.GetMin()).ToNot(gomega.BeNil())
			gomega.Expect(*p.GetMin()).To(gomega.Equal(0.0))
			gomega.Expect(p.GetMax()).ToNot(gomega.BeNil())
			gomega.Expect(*p.GetMax()).To(gomega.Equal(1.0))

			comp := NewComponent("test").Params(p)
			cue := gen.GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring(">=0"))
			gomega.Expect(cue).To(gomega.ContainSubstring("<=1"))
		})

		ginkgo.It("should set and generate array min/max items constraints", func() {
			p := Array("tags").Of(ParamTypeString).MinItems(1).MaxItems(10)
			gomega.Expect(p.GetMinItems()).ToNot(gomega.BeNil())
			gomega.Expect(*p.GetMinItems()).To(gomega.Equal(1))
			gomega.Expect(p.GetMaxItems()).ToNot(gomega.BeNil())
			gomega.Expect(*p.GetMaxItems()).To(gomega.Equal(10))

			comp := NewComponent("test").Params(p)
			cue := gen.GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring("list.MinItems(1)"))
			gomega.Expect(cue).To(gomega.ContainSubstring("list.MaxItems(10)"))
		})

		ginkgo.It("should generate string NotEmpty constraint", func() {
			p := String("name").NotEmpty()
			gomega.Expect(p.GetNotEmpty()).To(gomega.BeTrue())

			comp := NewComponent("test").Params(p)
			cue := gen.GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring(`!=""`))
		})

		ginkgo.It("should generate string NotEmpty with pattern combined", func() {
			p := String("name").NotEmpty().Pattern(`^[a-z0-9.-]{3,63}$`)

			comp := NewComponent("test").Params(p)
			cue := gen.GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring(`!=""`))
			gomega.Expect(cue).To(gomega.ContainSubstring(`=~"^[a-z0-9.-]{3,63}$"`))
		})

		ginkgo.It("should generate MapParam closed struct", func() {
			p := Object("governance").Closed().WithFields(
				String("tenantName"),
				String("departmentCode"),
			)
			gomega.Expect(p.IsClosed()).To(gomega.BeTrue())

			comp := NewComponent("test").Params(p)
			cue := gen.GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring("close({"))
			gomega.Expect(cue).To(gomega.ContainSubstring("})"))
		})

		ginkgo.It("should generate ArrayParam OfEnum schema", func() {
			p := Array("allowedMethods").OfEnum("GET", "PUT", "HEAD", "POST", "DELETE")

			comp := NewComponent("test").Params(p)
			cue := gen.GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring(`[...("GET" | "PUT" | "HEAD" | "POST" | "DELETE")]`))
		})
	})

	// --- Runtime Condition Tests ---

	ginkgo.Context("Runtime Conditions", func() {
		ginkgo.It("should generate string Contains condition", func() {
			p := String("name")
			cueStr := gen.conditionToCUE(p.Contains("prod"))
			gomega.Expect(cueStr).To(gomega.Equal(`strings.Contains(parameter.name, "prod")`))
		})

		ginkgo.It("should generate string Matches condition", func() {
			p := String("name")
			cueStr := gen.conditionToCUE(p.Matches("^prod-"))
			gomega.Expect(cueStr).To(gomega.Equal(`parameter.name =~ "^prod-"`))
		})

		ginkgo.It("should generate string StartsWith condition", func() {
			p := String("name")
			cueStr := gen.conditionToCUE(p.StartsWith("prod-"))
			gomega.Expect(cueStr).To(gomega.Equal(`strings.HasPrefix(parameter.name, "prod-")`))
		})

		ginkgo.It("should generate string EndsWith condition", func() {
			p := String("name")
			cueStr := gen.conditionToCUE(p.EndsWith("-prod"))
			gomega.Expect(cueStr).To(gomega.Equal(`strings.HasSuffix(parameter.name, "-prod")`))
		})

		ginkgo.It("should generate string In condition", func() {
			p := String("name")
			cueStr := gen.conditionToCUE(p.In("api", "web", "worker"))
			gomega.Expect(cueStr).To(gomega.ContainSubstring(`parameter.name == "api"`))
			gomega.Expect(cueStr).To(gomega.ContainSubstring(`parameter.name == "web"`))
			gomega.Expect(cueStr).To(gomega.ContainSubstring(" || "))
		})

		ginkgo.It("should generate int In condition", func() {
			p := Int("port")
			cueStr := gen.conditionToCUE(p.In(80, 443, 8080))
			gomega.Expect(cueStr).To(gomega.ContainSubstring("parameter.port == 80"))
			gomega.Expect(cueStr).To(gomega.ContainSubstring("parameter.port == 443"))
		})

		ginkgo.It("should generate bool IsFalse condition", func() {
			p := Bool("enabled")
			cueStr := gen.conditionToCUE(p.IsFalse())
			gomega.Expect(cueStr).To(gomega.Equal("!parameter.enabled"))
		})

		ginkgo.It("should generate float In condition", func() {
			p := Float("ratio")
			cueStr := gen.conditionToCUE(p.In(0.5, 1.0, 2.0))
			gomega.Expect(cueStr).To(gomega.ContainSubstring("parameter.ratio == 0.5"))
			gomega.Expect(cueStr).To(gomega.ContainSubstring("parameter.ratio == 1"))
			gomega.Expect(cueStr).To(gomega.ContainSubstring(" || "))
		})
	})

	ginkgo.Context("String Length Conditions", func() {
		ginkgo.DescribeTable("should generate correct CUE for length conditions",
			func(condFn func(*StringParam) Condition, expected string) {
				p := String("name")
				cueStr := gen.conditionToCUE(condFn(p))
				gomega.Expect(cueStr).To(gomega.Equal(expected))
			},
			ginkgo.Entry("LenEq", func(p *StringParam) Condition { return p.LenEq(5) }, `parameter["name"] != _|_ if len(parameter["name"]) == 5`),
			ginkgo.Entry("LenGt", func(p *StringParam) Condition { return p.LenGt(5) }, `parameter["name"] != _|_ if len(parameter["name"]) > 5`),
			ginkgo.Entry("LenGte", func(p *StringParam) Condition { return p.LenGte(5) }, `parameter["name"] != _|_ if len(parameter["name"]) >= 5`),
			ginkgo.Entry("LenLt", func(p *StringParam) Condition { return p.LenLt(5) }, `parameter["name"] != _|_ if len(parameter["name"]) < 5`),
			ginkgo.Entry("LenLte", func(p *StringParam) Condition { return p.LenLte(5) }, `parameter["name"] != _|_ if len(parameter["name"]) <= 5`),
		)
	})

	ginkgo.Context("Array Conditions", func() {
		ginkgo.DescribeTable("should generate correct CUE for array conditions",
			func(condFn func(*ArrayParam) Condition, expected string) {
				p := Array("tags").Of(ParamTypeString)
				cueStr := gen.conditionToCUE(condFn(p))
				gomega.Expect(cueStr).To(gomega.Equal(expected))
			},
			// All length predicates use CUE chained-if guard syntax. See
			// `lenConditionToCUE` in cuegen.go.
			//
			// IsEmpty() (and LenEq(0)) returns *AbsentOrEmptyCondition, which
			// expands into TWO if blocks at SetIf rendering. The string here
			// is the fallback (set-and-empty branch only) used when the
			// condition appears in non-expanding contexts.
			ginkgo.Entry("LenEq", func(p *ArrayParam) Condition { return p.LenEq(5) }, `parameter["tags"] != _|_ if len(parameter["tags"]) == 5`),
			ginkgo.Entry("LenGt", func(p *ArrayParam) Condition { return p.LenGt(0) }, `parameter["tags"] != _|_ if len(parameter["tags"]) > 0`),
			ginkgo.Entry("IsEmpty", func(p *ArrayParam) Condition { return p.IsEmpty() }, `parameter["tags"] != _|_ if len(parameter["tags"]) == 0`),
			ginkgo.Entry("LenEq(0)", func(p *ArrayParam) Condition { return p.LenEq(0) }, `parameter["tags"] != _|_ if len(parameter["tags"]) == 0`),
			ginkgo.Entry("IsNotEmpty", func(p *ArrayParam) Condition { return p.IsNotEmpty() }, `parameter["tags"] != _|_ if len(parameter["tags"]) > 0`),
			ginkgo.Entry("Contains", func(p *ArrayParam) Condition { return p.Contains("gpu") }, `parameter["tags"] != _|_ if list.Contains(parameter["tags"], "gpu")`),
		)

		ginkgo.It("should generate array Contains with different element types", func() {
			intArray := Array("ports").Of(ParamTypeInt)
			gomega.Expect(gen.conditionToCUE(intArray.Contains(8080))).To(gomega.Equal(`parameter["ports"] != _|_ if list.Contains(parameter["ports"], 8080)`))

			boolArray := Array("flags").Of(ParamTypeBool)
			gomega.Expect(gen.conditionToCUE(boolArray.Contains(true))).To(gomega.Equal(`parameter["flags"] != _|_ if list.Contains(parameter["flags"], true)`))
		})
	})

	ginkgo.Context("Map Conditions", func() {
		ginkgo.DescribeTable("should generate correct CUE for map conditions",
			func(condFn func(*MapParam) Condition, expected string) {
				p := Map("config")
				cueStr := gen.conditionToCUE(condFn(p))
				gomega.Expect(cueStr).To(gomega.Equal(expected))
			},
			ginkgo.Entry("HasKey", func(p *MapParam) Condition { return p.HasKey("debug") }, `parameter["config"] != _|_ && parameter["config"].debug != _|_`),
			ginkgo.Entry("LenEq", func(p *MapParam) Condition { return p.LenEq(5) }, `parameter["config"] != _|_ if len(parameter["config"]) == 5`),
			ginkgo.Entry("LenGt", func(p *MapParam) Condition { return p.LenGt(0) }, `parameter["config"] != _|_ if len(parameter["config"]) > 0`),
			ginkgo.Entry("IsEmpty", func(p *MapParam) Condition { return p.IsEmpty() }, `parameter["config"] != _|_ if len(parameter["config"]) == 0`),
			ginkgo.Entry("IsNotEmpty", func(p *MapParam) Condition { return p.IsNotEmpty() }, `parameter["config"] != _|_ if len(parameter["config"]) > 0`),
		)
	})

	// --- Chaining Tests ---

	ginkgo.Context("Constraint Chaining", func() {
		ginkgo.It("should preserve all chained string constraints", func() {
			p := String("name").
				Pattern("^[a-z]+$").
				MinLen(3).
				MaxLen(63).
				Description("The name")

			gomega.Expect(p.GetPattern()).To(gomega.Equal("^[a-z]+$"))
			gomega.Expect(*p.GetMinLen()).To(gomega.Equal(3))
			gomega.Expect(*p.GetMaxLen()).To(gomega.Equal(63))
			gomega.Expect(p.GetDescription()).To(gomega.Equal("The name"))
		})

		ginkgo.It("should preserve all chained int constraints", func() {
			p := Int("replicas").Min(1).Max(100).Default(3)

			gomega.Expect(*p.GetMin()).To(gomega.Equal(1))
			gomega.Expect(*p.GetMax()).To(gomega.Equal(100))
			gomega.Expect(p.HasDefault()).To(gomega.BeTrue())
			gomega.Expect(p.GetDefault()).To(gomega.Equal(3))
		})
	})

	// --- Combined Schema + Runtime ---

	ginkgo.Context("Combined Schema and Runtime", func() {
		ginkgo.It("should generate both schema constraints and runtime conditions", func() {
			replicas := Int("replicas").Min(1).Max(100).Default(3)

			comp := NewComponent("test").Params(replicas)
			schema := gen.GenerateParameterSchema(comp)
			gomega.Expect(schema).To(gomega.ContainSubstring(">=1"))
			gomega.Expect(schema).To(gomega.ContainSubstring("<=100"))
			gomega.Expect(schema).To(gomega.ContainSubstring("*3"))

			condStr := gen.conditionToCUE(replicas.Gt(5))
			gomega.Expect(condStr).To(gomega.Equal("parameter.replicas > 5"))
		})

		ginkgo.It("should generate combined string constraints", func() {
			p := String("hostname").
				Pattern("^[a-z][a-z0-9-]*$").
				MinLen(3).
				MaxLen(63)

			comp := NewComponent("test").Params(p)
			cue := gen.GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring(`=~"^[a-z][a-z0-9-]*$"`))
			gomega.Expect(cue).To(gomega.ContainSubstring("strings.MinRunes(3)"))
			gomega.Expect(cue).To(gomega.ContainSubstring("strings.MaxRunes(63)"))
			gomega.Expect(cue).To(gomega.ContainSubstring(" & "))
		})

		ginkgo.It("should generate string constraints with default", func() {
			p := String("env").
				Pattern("^(dev|staging|prod)$").
				Default("dev")

			comp := NewComponent("test").Params(p)
			cue := gen.GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring(`*"dev"`))
			gomega.Expect(cue).To(gomega.ContainSubstring(`=~"^(dev|staging|prod)$"`))
		})

		ginkgo.It("should generate int constraints with default", func() {
			p := Int("port").Min(1).Max(65535).Default(8080)

			comp := NewComponent("test").Params(p)
			cue := gen.GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring("*8080"))
			gomega.Expect(cue).To(gomega.ContainSubstring(">=1"))
			gomega.Expect(cue).To(gomega.ContainSubstring("<=65535"))
		})
	})

	// --- Integration with SetIf ---

	ginkgo.Context("SetIf Integration", func() {
		ginkgo.It("should generate CUE with various SetIf conditions", func() {
			name := String("name")
			replicas := Int("replicas").Min(1).Max(100)
			tags := Array("tags").Of(ParamTypeString)

			comp := NewComponent("test-app").
				Params(name, replicas, tags).
				Template(func(t *Template) {
					deployment := NewResource("apps/v1", "Deployment").
						Set("metadata.name", name).
						Set("spec.replicas", replicas).
						SetIf(name.StartsWith("prod-"), "metadata.labels.env", Lit("production")).
						SetIf(name.Contains("canary"), "metadata.labels.deployment", Lit("canary")).
						SetIf(replicas.Gt(5), "spec.strategy.type", Lit("RollingUpdate")).
						SetIf(tags.IsNotEmpty(), "metadata.annotations.has-tags", Lit("true")).
						SetIf(tags.Contains("gpu"), "spec.template.spec.nodeSelector.accelerator", Lit("nvidia"))
					t.Output(deployment)
				})

			cue := comp.ToCue()
			gomega.Expect(cue).To(gomega.ContainSubstring(`strings.HasPrefix(parameter.name, "prod-")`))
			gomega.Expect(cue).To(gomega.ContainSubstring(`strings.Contains(parameter.name, "canary")`))
			gomega.Expect(cue).To(gomega.ContainSubstring(`parameter.replicas > 5`))
			gomega.Expect(cue).To(gomega.ContainSubstring(`parameter["tags"] != _|_`))
			gomega.Expect(cue).To(gomega.ContainSubstring(`list.Contains(parameter["tags"], "gpu")`))
		})
	})

	// --- Edge Cases ---

	ginkgo.Context("Edge Cases", func() {
		ginkgo.It("should handle zero as min value", func() {
			p := Int("count").Min(0).Max(10)

			comp := NewComponent("test").Params(p)
			cue := gen.GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring(">=0"))
		})

		ginkgo.It("should handle empty string conditions", func() {
			p := String("name")
			gomega.Expect(gen.conditionToCUE(p.Contains(""))).To(gomega.Equal(`strings.Contains(parameter.name, "")`))
			gomega.Expect(gen.conditionToCUE(p.StartsWith(""))).To(gomega.Equal(`strings.HasPrefix(parameter.name, "")`))
		})

		ginkgo.It("should handle single value In condition", func() {
			p := String("status")
			cueStr := gen.conditionToCUE(p.In("active"))
			gomega.Expect(cueStr).To(gomega.Equal(`parameter.status == "active"`))
		})

		ginkgo.It("should escape special regex characters in pattern", func() {
			p := String("email").Pattern(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

			comp := NewComponent("test").Params(p)
			cue := gen.GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring(`=~"^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$"`))
		})
	})
})
