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

var _ = Describe("ArrayBuilder", func() {

	Describe("NewArray", func() {
		It("should create an empty array builder", func() {
			ab := defkit.NewArray()
			Expect(ab).NotTo(BeNil())
			Expect(ab.Entries()).To(BeEmpty())
		})

		It("should implement the Value interface and retain builder behavior", func() {
			ab := defkit.NewArray()
			var v defkit.Value = ab // compile-time interface check
			Expect(v).NotTo(BeNil())
			// Verify the builder still works through the interface
			ab.Item(defkit.NewArrayElement().Set("name", defkit.Lit("test")))
			Expect(ab.Entries()).To(HaveLen(1))
		})
	})

	Describe("Item", func() {
		It("should add a static entry", func() {
			elem := defkit.NewArrayElement().Set("name", defkit.Lit("cpu"))
			ab := defkit.NewArray().Item(elem)
			Expect(ab.Entries()).To(HaveLen(1))
		})

		It("should chain multiple static items", func() {
			elem1 := defkit.NewArrayElement().Set("name", defkit.Lit("cpu"))
			elem2 := defkit.NewArrayElement().Set("name", defkit.Lit("memory"))
			ab := defkit.NewArray().Item(elem1).Item(elem2)
			Expect(ab.Entries()).To(HaveLen(2))
		})
	})

	Describe("ItemIf", func() {
		It("should add a conditional entry", func() {
			mem := defkit.String("memory")
			elem := defkit.NewArrayElement().Set("name", defkit.Lit("memory"))
			ab := defkit.NewArray().ItemIf(mem.IsSet(), elem)
			Expect(ab.Entries()).To(HaveLen(1))
		})
	})

	Describe("ForEach", func() {
		It("should add a forEach entry", func() {
			ports := defkit.List("ports")
			elem := defkit.NewArrayElement().Set("port", defkit.Reference("m.port"))
			ab := defkit.NewArray().ForEach(ports, elem)
			Expect(ab.Entries()).To(HaveLen(1))
		})
	})

	Describe("ForEachGuarded", func() {
		It("should add a guarded forEach entry", func() {
			ports := defkit.List("ports")
			elem := defkit.NewArrayElement().Set("port", defkit.Reference("m.port"))
			ab := defkit.NewArray().ForEachGuarded(ports.IsSet(), ports, elem)
			Expect(ab.Entries()).To(HaveLen(1))
		})
	})

	Describe("ForEachWith", func() {
		It("should add a forEachWith entry with ItemBuilder", func() {
			ports := defkit.List("ports")
			ab := defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
				v := item.Var()
				item.Set("port", v.Field("port"))
			})
			entries := ab.Entries()
			Expect(entries).To(HaveLen(1))
		})

		It("should use 'v' as default variable name", func() {
			ports := defkit.List("ports")
			ab := defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
				Expect(item.VarName()).To(Equal("v"))
			})
			Expect(ab.Entries()).To(HaveLen(1))
		})
	})

	Describe("ForEachWithVar", func() {
		It("should use custom variable name", func() {
			ports := defkit.List("ports")
			ab := defkit.NewArray().ForEachWithVar("p", ports, func(item *defkit.ItemBuilder) {
				Expect(item.VarName()).To(Equal("p"))
				v := item.Var()
				item.Set("port", v.Field("port"))
			})
			Expect(ab.Entries()).To(HaveLen(1))
		})
	})

	Describe("ForEachWithGuardedFiltered", func() {
		It("should add entry with guard and filter", func() {
			ports := defkit.List("ports")
			ab := defkit.NewArray().ForEachWithGuardedFiltered(
				ports.IsSet(),
				defkit.FieldEquals("expose", true),
				ports,
				func(item *defkit.ItemBuilder) {
					v := item.Var()
					item.Set("port", v.Field("port"))
				},
			)
			Expect(ab.Entries()).To(HaveLen(1))
		})

		It("should use 'v' as default variable name", func() {
			ports := defkit.List("ports")
			defkit.NewArray().ForEachWithGuardedFiltered(
				ports.IsSet(),
				defkit.FieldEquals("expose", true),
				ports,
				func(item *defkit.ItemBuilder) {
					Expect(item.VarName()).To(Equal("v"))
				},
			)
		})
	})

	Describe("ForEachWithGuardedFilteredVar", func() {
		It("should use custom variable name with guard and filter", func() {
			ports := defkit.List("ports")
			ab := defkit.NewArray().ForEachWithGuardedFilteredVar(
				"p",
				ports.IsSet(),
				defkit.FieldEquals("expose", true),
				ports,
				func(item *defkit.ItemBuilder) {
					Expect(item.VarName()).To(Equal("p"))
					v := item.Var()
					item.Set("port", v.Field("port"))
				},
			)
			Expect(ab.Entries()).To(HaveLen(1))
		})
	})

	Describe("mixed entries", func() {
		It("should support mixing static, conditional, forEach, and forEachWith entries", func() {
			mem := defkit.String("memory")
			ports := defkit.List("ports")
			metrics := defkit.List("metrics")

			staticElem := defkit.NewArrayElement().Set("name", defkit.Lit("cpu"))
			condElem := defkit.NewArrayElement().Set("name", defkit.Lit("memory"))
			forEachElem := defkit.NewArrayElement().Set("name", defkit.Reference("m.name"))

			ab := defkit.NewArray().
				Item(staticElem).
				ItemIf(mem.IsSet(), condElem).
				ForEach(metrics, forEachElem).
				ForEachWith(ports, func(item *defkit.ItemBuilder) {
					v := item.Var()
					item.Set("port", v.Field("port"))
				})

			Expect(ab.Entries()).To(HaveLen(4))
		})
	})

	Describe("fluent chaining", func() {
		It("should return the same builder for all methods", func() {
			ab := defkit.NewArray()
			elem := defkit.NewArrayElement().Set("name", defkit.Lit("test"))
			ports := defkit.List("ports")

			result := ab.Item(elem)
			Expect(result).To(BeIdenticalTo(ab))

			result = ab.ItemIf(ports.IsSet(), elem)
			Expect(result).To(BeIdenticalTo(ab))

			result = ab.ForEach(ports, elem)
			Expect(result).To(BeIdenticalTo(ab))

			result = ab.ForEachGuarded(ports.IsSet(), ports, elem)
			Expect(result).To(BeIdenticalTo(ab))

			result = ab.ForEachWith(ports, func(item *defkit.ItemBuilder) {})
			Expect(result).To(BeIdenticalTo(ab))

			result = ab.ForEachWithGuardedFiltered(
				ports.IsSet(), defkit.FieldEquals("expose", true), ports,
				func(item *defkit.ItemBuilder) {},
			)
			Expect(result).To(BeIdenticalTo(ab))
		})
	})
})

var _ = Describe("ItemBuilder", func() {

	Describe("Var", func() {
		It("should return an IterVarBuilder with the correct variable name", func() {
			ports := defkit.List("ports")
			defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
				v := item.Var()
				Expect(v).NotTo(BeNil())

				// v.Field returns an IterFieldRef which implements Value
				fieldRef := v.Field("port")
				Expect(fieldRef).NotTo(BeNil())
				Expect(fieldRef.VarName()).To(Equal("v"))
				Expect(fieldRef.FieldName()).To(Equal("port"))

				// v.Ref returns an IterVarRef for the whole variable
				varRef := v.Ref()
				Expect(varRef).NotTo(BeNil())
				Expect(varRef.VarName()).To(Equal("v"))
			})
		})
	})

	Describe("Set", func() {
		It("should record a setOp", func() {
			ports := defkit.List("ports")
			defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
				v := item.Var()
				item.Set("port", v.Field("port"))
				item.Set("name", v.Field("name"))
				Expect(item.Ops()).To(HaveLen(2))
			})
		})
	})

	Describe("If", func() {
		It("should record an ifBlockOp with nested operations", func() {
			ports := defkit.List("ports")
			exposeType := defkit.String("exposeType")
			defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
				v := item.Var()
				item.If(defkit.Eq(exposeType, defkit.Lit("NodePort")), func() {
					item.Set("nodePort", v.Field("nodePort"))
				})
				Expect(item.Ops()).To(HaveLen(1))
			})
		})
	})

	Describe("IfSet", func() {
		It("should record a conditional for field existence", func() {
			ports := defkit.List("ports")
			defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
				v := item.Var()
				item.IfSet("name", func() {
					item.Set("name", v.Field("name"))
				})
				Expect(item.Ops()).To(HaveLen(1))
			})
		})
	})

	Describe("IfNotSet", func() {
		It("should record a conditional for field non-existence", func() {
			ports := defkit.List("ports")
			defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
				_ = item.Var()
				item.IfNotSet("name", func() {
					item.Set("name", defkit.Lit("default"))
				})
				Expect(item.Ops()).To(HaveLen(1))
			})
		})
	})

	Describe("IfSet and IfNotSet together", func() {
		It("should form an if/else pattern", func() {
			ports := defkit.List("ports")
			defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
				v := item.Var()
				item.IfSet("containerPort", func() {
					item.Set("containerPort", v.Field("containerPort"))
				})
				item.IfNotSet("containerPort", func() {
					item.Set("containerPort", v.Field("port"))
				})
				Expect(item.Ops()).To(HaveLen(2))
			})
		})
	})

	Describe("Let", func() {
		It("should record a letOp and return a reference with correct name", func() {
			ports := defkit.List("ports")
			defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
				v := item.Var()
				ref := item.Let("_name",
					defkit.Plus(defkit.Lit("port-"), defkit.StrconvFormatInt(v.Field("port"), 10)))
				letRef, ok := ref.(*defkit.IterLetRef)
				Expect(ok).To(BeTrue(), "expected *IterLetRef")
				Expect(letRef.RefName()).To(Equal("_name"))
				Expect(item.Ops()).To(HaveLen(1))
			})
		})

		It("should allow using the let reference in subsequent operations", func() {
			ports := defkit.List("ports")
			defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
				v := item.Var()
				nameRef := item.Let("_name",
					defkit.Plus(defkit.Lit("port-"), defkit.StrconvFormatInt(v.Field("port"), 10)))
				item.SetDefault("name", nameRef, "string")
				Expect(item.Ops()).To(HaveLen(2))
			})
		})
	})

	Describe("SetDefault", func() {
		It("should record a setDefaultOp", func() {
			ports := defkit.List("ports")
			defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
				item.SetDefault("name", defkit.Lit("default"), "string")
				Expect(item.Ops()).To(HaveLen(1))
			})
		})
	})

	Describe("FieldExists", func() {
		It("should return a Condition for field existence with correct var and field name", func() {
			ports := defkit.List("ports")
			defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
				cond := item.FieldExists("name")
				Expect(cond).NotTo(BeNil())
				iterCond, ok := cond.(*defkit.IterFieldExistsCondition)
				Expect(ok).To(BeTrue(), "expected *IterFieldExistsCondition")
				Expect(iterCond.VarName()).To(Equal("v"))
				Expect(iterCond.FieldName()).To(Equal("name"))
				Expect(iterCond.IsNegated()).To(BeFalse())
			})
		})
	})

	Describe("FieldNotExists", func() {
		It("should return a negated Condition for field non-existence", func() {
			ports := defkit.List("ports")
			defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
				cond := item.FieldNotExists("name")
				Expect(cond).NotTo(BeNil())
				iterCond, ok := cond.(*defkit.IterFieldExistsCondition)
				Expect(ok).To(BeTrue(), "expected *IterFieldExistsCondition")
				Expect(iterCond.VarName()).To(Equal("v"))
				Expect(iterCond.FieldName()).To(Equal("name"))
				Expect(iterCond.IsNegated()).To(BeTrue())
			})
		})
	})

	Describe("complex ItemBuilder pattern", func() {
		It("should support nested conditionals with let bindings and defaults", func() {
			ports := defkit.List("ports")
			defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
				v := item.Var()

				// Unconditional field
				item.Set("port", v.Field("port"))

				// If/else for containerPort
				item.IfSet("containerPort", func() {
					item.Set("containerPort", v.Field("containerPort"))
				})
				item.IfNotSet("containerPort", func() {
					item.Set("containerPort", v.Field("port"))
				})

				// Nested: name with let binding, default, and protocol suffix
				item.IfSet("name", func() {
					item.Set("name", v.Field("name"))
				})
				item.IfNotSet("name", func() {
					nameRef := item.Let("_name",
						defkit.Plus(defkit.Lit("port-"), defkit.StrconvFormatInt(v.Field("port"), 10)))
					item.SetDefault("name", nameRef, "string")
					item.If(defkit.Ne(v.Field("protocol"), defkit.Lit("TCP")), func() {
						item.Set("name", defkit.Plus(nameRef, defkit.Lit("-"), defkit.StringsToLower(v.Field("protocol"))))
					})
				})

				// set=1, ifSet(containerPort)=1, ifNotSet(containerPort)=1, ifSet(name)=1, ifNotSet(name)=1
				Expect(item.Ops()).To(HaveLen(5))
			})
		})
	})
})

var _ = Describe("IterVarBuilder", func() {
	It("should return field references with correct variable and field names", func() {
		ports := defkit.List("ports")
		defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
			v := item.Var()
			portRef := v.Field("port")
			Expect(portRef.VarName()).To(Equal("v"))
			Expect(portRef.FieldName()).To(Equal("port"))

			nameRef := v.Field("name")
			Expect(nameRef.VarName()).To(Equal("v"))
			Expect(nameRef.FieldName()).To(Equal("name"))
		})
	})

	It("should return a whole-variable reference with correct variable name", func() {
		items := defkit.StringList("items")
		defkit.NewArray().ForEachWith(items, func(item *defkit.ItemBuilder) {
			v := item.Var()
			ref := v.Ref()
			Expect(ref.VarName()).To(Equal("v"))
		})
	})

	It("should use custom variable name for field references", func() {
		ports := defkit.List("ports")
		defkit.NewArray().ForEachWithVar("p", ports, func(item *defkit.ItemBuilder) {
			v := item.Var()
			fieldRef := v.Field("port")
			Expect(fieldRef.VarName()).To(Equal("p"))
			Expect(fieldRef.FieldName()).To(Equal("port"))

			ref := v.Ref()
			Expect(ref.VarName()).To(Equal("p"))
		})
	})
})

var _ = Describe("ArrayConcat", func() {
	It("should create an array concatenation value", func() {
		left := defkit.NewArray()
		right := defkit.List("extra")
		concat := defkit.ArrayConcat(left, right)
		Expect(concat).NotTo(BeNil())
		Expect(concat.Left()).To(BeIdenticalTo(left))
		Expect(concat.Right()).To(BeIdenticalTo(right))
	})

	It("should implement the Value interface and preserve operands", func() {
		left := defkit.NewArray()
		right := defkit.List("extra")
		var v defkit.Value = defkit.ArrayConcat(left, right) // compile-time interface check
		Expect(v).NotTo(BeNil())
		concat := v.(*defkit.ArrayConcatValue)
		Expect(concat.Left()).To(BeIdenticalTo(left))
		Expect(concat.Right()).To(BeIdenticalTo(right))
	})
})

var _ = Describe("ArrayBuilder CUE Generation", func() {
	var gen *defkit.CUEGenerator

	BeforeEach(func() {
		gen = defkit.NewCUEGenerator()
	})

	It("should generate CUE for ForEachWith with simple field assignments", func() {
		ports := defkit.List("ports").WithFields(
			defkit.Int("port").Required(),
			defkit.String("name"),
		)
		comp := defkit.NewComponent("test").
			Workload("apps/v1", "Deployment").
			Params(ports).
			Template(func(tpl *defkit.Template) {
				containerPorts := defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
					v := item.Var()
					item.Set("containerPort", v.Field("port"))
					item.Set("name", v.Field("name"))
				})
				tpl.Output(
					defkit.NewResource("apps/v1", "Deployment").
						SetIf(ports.IsSet(), "spec.template.spec.containers[0].ports", containerPorts),
				)
			})

		cue := gen.GenerateFullDefinition(comp)

		Expect(cue).To(ContainSubstring("for v in parameter.ports"))
		Expect(cue).To(ContainSubstring("containerPort: v.port"))
		Expect(cue).To(ContainSubstring("name: v.name"))
	})

	It("should generate CUE for ForEachWith with IfSet/IfNotSet conditionals", func() {
		ports := defkit.List("ports").WithFields(
			defkit.Int("port").Required(),
			defkit.Int("containerPort"),
		)
		comp := defkit.NewComponent("test").
			Workload("apps/v1", "Deployment").
			Params(ports).
			Template(func(tpl *defkit.Template) {
				containerPorts := defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
					v := item.Var()
					item.IfSet("containerPort", func() {
						item.Set("containerPort", v.Field("containerPort"))
					})
					item.IfNotSet("containerPort", func() {
						item.Set("containerPort", v.Field("port"))
					})
				})
				tpl.Output(
					defkit.NewResource("apps/v1", "Deployment").
						SetIf(ports.IsSet(), "spec.template.spec.containers[0].ports", containerPorts),
				)
			})

		cue := gen.GenerateFullDefinition(comp)

		Expect(cue).To(ContainSubstring("if v.containerPort != _|_"))
		Expect(cue).To(ContainSubstring("containerPort: v.containerPort"))
		Expect(cue).To(ContainSubstring("if v.containerPort == _|_"))
		Expect(cue).To(ContainSubstring("containerPort: v.port"))
	})

	It("should generate CUE for ForEachWith with let bindings and defaults", func() {
		ports := defkit.List("ports").WithFields(
			defkit.Int("port").Required(),
		)
		comp := defkit.NewComponent("test").
			Workload("apps/v1", "Deployment").
			Params(ports).
			Template(func(tpl *defkit.Template) {
				containerPorts := defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
					v := item.Var()
					nameRef := item.Let("_name",
						defkit.Plus(defkit.Lit("port-"), defkit.StrconvFormatInt(v.Field("port"), 10)))
					item.SetDefault("name", nameRef, "string")
				})
				tpl.Output(
					defkit.NewResource("apps/v1", "Deployment").
						SetIf(ports.IsSet(), "spec.template.spec.containers[0].ports", containerPorts),
				)
			})

		cue := gen.GenerateFullDefinition(comp)

		Expect(cue).To(ContainSubstring(`_name: "port-" + strconv.FormatInt(v.port, 10)`))
		Expect(cue).To(ContainSubstring("name: *_name | string"))
	})

	It("should generate CUE for ForEachWithVar with custom variable name", func() {
		ports := defkit.List("ports").WithFields(
			defkit.Int("port").Required(),
		)
		comp := defkit.NewComponent("test").
			Workload("apps/v1", "Deployment").
			Params(ports).
			Template(func(tpl *defkit.Template) {
				containerPorts := defkit.NewArray().ForEachWithVar("p", ports, func(item *defkit.ItemBuilder) {
					v := item.Var()
					item.Set("containerPort", v.Field("port"))
				})
				tpl.Output(
					defkit.NewResource("apps/v1", "Deployment").
						SetIf(ports.IsSet(), "spec.template.spec.containers[0].ports", containerPorts),
				)
			})

		cue := gen.GenerateFullDefinition(comp)

		Expect(cue).To(ContainSubstring("for p in parameter.ports"))
		Expect(cue).To(ContainSubstring("containerPort: p.port"))
	})

	It("should generate CUE for ForEachWithGuardedFiltered with guard and filter", func() {
		ports := defkit.List("ports").WithFields(
			defkit.Int("port").Required(),
			defkit.Bool("expose").Default(false),
		)
		comp := defkit.NewComponent("test").
			Workload("apps/v1", "Deployment").
			Params(ports).
			Template(func(tpl *defkit.Template) {
				exposePorts := defkit.NewArray().ForEachWithGuardedFiltered(
					ports.IsSet(),
					defkit.FieldEquals("expose", true),
					ports,
					func(item *defkit.ItemBuilder) {
						v := item.Var()
						item.Set("port", v.Field("port"))
					},
				)
				tpl.Output(
					defkit.NewResource("apps/v1", "Deployment").
						Set("spec.ports", exposePorts),
				)
			})

		cue := gen.GenerateFullDefinition(comp)

		// Guard condition
		Expect(cue).To(ContainSubstring(`parameter["ports"] != _|_`))
		// Filter predicate
		Expect(cue).To(ContainSubstring("v.expose == true"))
		// Field assignment
		Expect(cue).To(ContainSubstring("port: v.port"))
	})

	It("should generate CUE for ForEachWith with nested If conditions", func() {
		ports := defkit.List("ports").WithFields(
			defkit.Int("port").Required(),
			defkit.String("protocol"),
		)
		exposeType := defkit.String("exposeType")
		comp := defkit.NewComponent("test").
			Workload("apps/v1", "Deployment").
			Params(ports, exposeType).
			Template(func(tpl *defkit.Template) {
				result := defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
					v := item.Var()
					item.Set("port", v.Field("port"))
					item.If(defkit.Ne(v.Field("protocol"), defkit.Lit("TCP")), func() {
						item.Set("protocol", v.Field("protocol"))
					})
				})
				tpl.Output(
					defkit.NewResource("apps/v1", "Deployment").
						SetIf(ports.IsSet(), "spec.ports", result),
				)
			})

		cue := gen.GenerateFullDefinition(comp)

		Expect(cue).To(ContainSubstring("port: v.port"))
		Expect(cue).To(ContainSubstring(`v.protocol != "TCP"`))
		Expect(cue).To(ContainSubstring("protocol: v.protocol"))
	})

	It("should auto-detect strconv import from ForEachWith ItemBuilder ops", func() {
		ports := defkit.List("ports").WithFields(
			defkit.Int("port").Required(),
		)
		comp := defkit.NewComponent("test").
			Workload("apps/v1", "Deployment").
			Params(ports).
			Template(func(tpl *defkit.Template) {
				result := defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
					v := item.Var()
					item.Let("_name",
						defkit.Plus(defkit.Lit("port-"), defkit.StrconvFormatInt(v.Field("port"), 10)))
				})
				tpl.Output(
					defkit.NewResource("apps/v1", "Deployment").
						Set("spec.ports", result),
				)
			})

		cue := gen.GenerateFullDefinition(comp)

		Expect(cue).To(ContainSubstring(`"strconv"`))
	})

	It("should auto-detect strings import from ForEachWith ItemBuilder ops", func() {
		ports := defkit.List("ports").WithFields(
			defkit.Int("port").Required(),
			defkit.String("protocol"),
		)
		comp := defkit.NewComponent("test").
			Workload("apps/v1", "Deployment").
			WithImports().
			Params(ports).
			Template(func(tpl *defkit.Template) {
				result := defkit.NewArray().ForEachWith(ports, func(item *defkit.ItemBuilder) {
					v := item.Var()
					item.Set("name", defkit.StringsToLower(v.Field("protocol")))
				})
				tpl.Output(
					defkit.NewResource("apps/v1", "Deployment").
						Set("spec.ports", result),
				)
			})

		cue := gen.GenerateFullDefinition(comp)

		Expect(cue).To(ContainSubstring(`"strings"`))
	})

	It("should generate CUE for helper backed by FromArray with ArrayBuilder", func() {
		ports := defkit.List("ports").WithFields(
			defkit.Int("port").Required(),
			defkit.Bool("expose").Default(false),
		)
		comp := defkit.NewComponent("test").
			Workload("apps/v1", "Deployment").
			Params(ports).
			Template(func(tpl *defkit.Template) {
				exposePortsArray := defkit.NewArray().ForEachWithGuardedFiltered(
					ports.IsSet(),
					defkit.FieldEquals("expose", true),
					ports,
					func(item *defkit.ItemBuilder) {
						v := item.Var()
						item.Set("port", v.Field("port"))
					},
				)
				exposePorts := tpl.Helper("exposePorts").
					FromArray(exposePortsArray).
					AfterOutput().
					Build()

				tpl.Output(
					defkit.NewResource("apps/v1", "Deployment").
						Set("metadata.name", defkit.VelaCtx().Name()),
				)
				tpl.OutputsIf(exposePorts.NotEmpty(), "svc",
					defkit.NewResource("v1", "Service").
						Set("spec.ports", exposePorts),
				)
			})

		cue := gen.GenerateFullDefinition(comp)

		// Helper should be named exposePorts
		Expect(cue).To(ContainSubstring("exposePorts:"))
		// Guard + filter + iteration
		Expect(cue).To(ContainSubstring(`parameter["ports"] != _|_`))
		Expect(cue).To(ContainSubstring("v.expose == true"))
		Expect(cue).To(ContainSubstring("port: v.port"))
		// Outputs should reference exposePorts
		Expect(cue).To(ContainSubstring("len(exposePorts) != 0"))
	})
})
