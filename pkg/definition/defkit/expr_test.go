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

var _ = Describe("Expressions", func() {

	Context("Literal", func() {
		It("should create string literal", func() {
			lit := defkit.Lit("hello")
			Expect(lit.Val()).To(Equal("hello"))
		})

		It("should create integer literal", func() {
			lit := defkit.Lit(42)
			Expect(lit.Val()).To(Equal(42))
		})

		It("should create boolean literal", func() {
			lit := defkit.Lit(true)
			Expect(lit.Val()).To(Equal(true))
		})

		It("should create float literal", func() {
			lit := defkit.Lit(3.14)
			Expect(lit.Val()).To(Equal(3.14))
		})

		It("should create nil literal", func() {
			lit := defkit.Lit(nil)
			Expect(lit.Val()).To(BeNil())
		})
	})

	Context("Comparisons", func() {
		var left, right defkit.Expr

		BeforeEach(func() {
			left = defkit.String("count")
			right = defkit.Lit(10)
		})

		It("should create equality comparison", func() {
			cmp := defkit.Eq(left, right)
			Expect(cmp.Op()).To(Equal(defkit.OpEq))
			Expect(cmp.Left()).To(Equal(left))
			Expect(cmp.Right()).To(Equal(right))
		})

		It("should create inequality comparison", func() {
			cmp := defkit.Ne(left, right)
			Expect(cmp.Op()).To(Equal(defkit.OpNe))
		})

		It("should create less-than comparison", func() {
			cmp := defkit.Lt(left, right)
			Expect(cmp.Op()).To(Equal(defkit.OpLt))
		})

		It("should create less-than-or-equal comparison", func() {
			cmp := defkit.Le(left, right)
			Expect(cmp.Op()).To(Equal(defkit.OpLe))
		})

		It("should create greater-than comparison", func() {
			cmp := defkit.Gt(left, right)
			Expect(cmp.Op()).To(Equal(defkit.OpGt))
		})

		It("should create greater-than-or-equal comparison", func() {
			cmp := defkit.Ge(left, right)
			Expect(cmp.Op()).To(Equal(defkit.OpGe))
		})

		It("should support comparison with two parameters", func() {
			p1 := defkit.Int("min")
			p2 := defkit.Int("max")
			cmp := defkit.Lt(p1, p2)
			Expect(cmp.Left()).To(Equal(p1))
			Expect(cmp.Right()).To(Equal(p2))
		})

		It("should support comparison with two literals", func() {
			l1 := defkit.Lit(5)
			l2 := defkit.Lit(10)
			cmp := defkit.Lt(l1, l2)
			Expect(cmp.Left()).To(Equal(l1))
			Expect(cmp.Right()).To(Equal(l2))
		})
	})

	Context("Logical Operators", func() {
		var cond1, cond2, cond3 defkit.Condition

		BeforeEach(func() {
			p := defkit.Int("count")
			cond1 = defkit.Gt(p, defkit.Lit(0))
			cond2 = defkit.Lt(p, defkit.Lit(100))
			cond3 = defkit.Ne(p, defkit.Lit(50))
		})

		Context("And", func() {
			It("should create AND of two conditions", func() {
				and := defkit.And(cond1, cond2)
				Expect(and.Op()).To(Equal(defkit.OpAnd))
				Expect(and.Conditions()).To(HaveLen(2))
				Expect(and.Conditions()[0]).To(Equal(cond1))
				Expect(and.Conditions()[1]).To(Equal(cond2))
			})

			It("should create AND of multiple conditions", func() {
				and := defkit.And(cond1, cond2, cond3)
				Expect(and.Op()).To(Equal(defkit.OpAnd))
				Expect(and.Conditions()).To(HaveLen(3))
			})

			It("should support empty conditions", func() {
				and := defkit.And()
				Expect(and.Conditions()).To(BeEmpty())
			})
		})

		Context("Or", func() {
			It("should create OR of two conditions", func() {
				or := defkit.Or(cond1, cond2)
				Expect(or.Op()).To(Equal(defkit.OpOr))
				Expect(or.Conditions()).To(HaveLen(2))
			})

			It("should create OR of multiple conditions", func() {
				or := defkit.Or(cond1, cond2, cond3)
				Expect(or.Conditions()).To(HaveLen(3))
			})
		})

		Context("Not", func() {
			It("should create NOT of a condition", func() {
				not := defkit.Not(cond1)
				Expect(not.Cond()).To(Equal(cond1))
			})

			It("should support negating a comparison", func() {
				p := defkit.Bool("enabled")
				enabled := defkit.Eq(p, defkit.Lit(true))
				not := defkit.Not(enabled)
				Expect(not.Cond()).To(Equal(enabled))
			})
		})

		Context("Nested Logical Expressions", func() {
			It("should support And within Or", func() {
				inner := defkit.And(cond1, cond2)
				outer := defkit.Or(inner, cond3)
				Expect(outer.Op()).To(Equal(defkit.OpOr))
				Expect(outer.Conditions()).To(HaveLen(2))
				Expect(outer.Conditions()[0]).To(Equal(inner))
			})

			It("should support Not within And", func() {
				notCond := defkit.Not(cond3)
				and := defkit.And(cond1, cond2, notCond)
				Expect(and.Conditions()).To(HaveLen(3))
				Expect(and.Conditions()[2]).To(Equal(notCond))
			})

			It("should support complex nesting", func() {
				// (cond1 AND cond2) OR (NOT cond3)
				left := defkit.And(cond1, cond2)
				right := defkit.Not(cond3)
				result := defkit.Or(left, right)
				Expect(result.Conditions()).To(HaveLen(2))
			})
		})
	})

	Context("CueFunc expressions", func() {
		It("should create StrconvFormatInt function with correct args", func() {
			num := defkit.Int("port")
			fn := defkit.StrconvFormatInt(num, 10)
			Expect(fn.Package()).To(Equal("strconv"))
			Expect(fn.Function()).To(Equal("FormatInt"))
			Expect(fn.Args()).To(HaveLen(2))
			Expect(fn.Args()[0]).To(Equal(num))
			Expect(fn.Args()[1]).To(Equal(defkit.Lit(10)))
		})

		It("should create StringsToLower function with correct arg", func() {
			str := defkit.String("name")
			fn := defkit.StringsToLower(str)
			Expect(fn.Package()).To(Equal("strings"))
			Expect(fn.Function()).To(Equal("ToLower"))
			Expect(fn.Args()).To(HaveLen(1))
			Expect(fn.Args()[0]).To(Equal(str))
		})

		It("should create StringsToUpper function with correct arg", func() {
			str := defkit.String("name")
			fn := defkit.StringsToUpper(str)
			Expect(fn.Package()).To(Equal("strings"))
			Expect(fn.Function()).To(Equal("ToUpper"))
			Expect(fn.Args()).To(HaveLen(1))
			Expect(fn.Args()[0]).To(Equal(str))
		})

		It("should create StringsHasPrefix function with correct args", func() {
			str := defkit.String("path")
			fn := defkit.StringsHasPrefix(str, "/api")
			Expect(fn.Package()).To(Equal("strings"))
			Expect(fn.Function()).To(Equal("HasPrefix"))
			Expect(fn.Args()).To(HaveLen(2))
			Expect(fn.Args()[0]).To(Equal(str))
			Expect(fn.Args()[1]).To(Equal(defkit.Lit("/api")))
		})

		It("should create StringsHasSuffix function with correct args", func() {
			str := defkit.String("file")
			fn := defkit.StringsHasSuffix(str, ".yaml")
			Expect(fn.Package()).To(Equal("strings"))
			Expect(fn.Function()).To(Equal("HasSuffix"))
			Expect(fn.Args()).To(HaveLen(2))
			Expect(fn.Args()[0]).To(Equal(str))
			Expect(fn.Args()[1]).To(Equal(defkit.Lit(".yaml")))
		})

		It("should create ListConcat function with correct args", func() {
			list1 := defkit.List("list1")
			list2 := defkit.List("list2")
			fn := defkit.ListConcat(list1, list2)
			Expect(fn.Package()).To(Equal("list"))
			Expect(fn.Function()).To(Equal("Concat"))
			Expect(fn.Args()).To(HaveLen(2))
			Expect(fn.Args()[0]).To(Equal(list1))
			Expect(fn.Args()[1]).To(Equal(list2))
		})
	})

	Context("ArrayElement", func() {
		It("should create new array element", func() {
			elem := defkit.NewArrayElement()
			Expect(elem).NotTo(BeNil())
			Expect(elem.Ops()).To(BeEmpty())
			Expect(elem.Fields()).To(BeEmpty())
		})

		It("should set fields on array element", func() {
			nameLit := defkit.Lit("test")
			portLit := defkit.Lit(8080)
			elem := defkit.NewArrayElement().
				Set("name", nameLit).
				Set("port", portLit)
			Expect(elem.Fields()).To(HaveLen(2))
			Expect(elem.Fields()["name"]).To(Equal(nameLit))
			Expect(elem.Fields()["port"]).To(Equal(portLit))
		})

		It("should set conditional fields on array element", func() {
			enabled := defkit.Bool("enabled")
			elem := defkit.NewArrayElement().
				SetIf(enabled.IsTrue(), "active", defkit.Lit(true))
			Expect(elem.Ops()).To(HaveLen(1))
		})

		It("should return fields from array element", func() {
			elem := defkit.NewArrayElement().
				Set("name", defkit.Lit("test"))
			fields := elem.Fields()
			Expect(fields).To(HaveLen(1))
		})
	})

	Context("ForEachMap", func() {
		It("should create ForEachMap expression", func() {
			forEach := defkit.ForEachMap()
			Expect(forEach).NotTo(BeNil())
			Expect(forEach.Source()).To(Equal("parameter"))
			Expect(forEach.KeyVar()).To(Equal("k"))
			Expect(forEach.ValVar()).To(Equal("v"))
			Expect(forEach.KeyExpr()).To(BeEmpty())
			Expect(forEach.ValExpr()).To(BeEmpty())
		})

		It("should set source and variable names", func() {
			forEach := defkit.ForEachMap().
				Over("parameter.labels").
				WithVars("k", "v")
			Expect(forEach.KeyVar()).To(Equal("k"))
			Expect(forEach.ValVar()).To(Equal("v"))
			Expect(forEach.Source()).To(Equal("parameter.labels"))
		})

		It("should set key expression", func() {
			forEach := defkit.ForEachMap().
				Over("parameter.labels").
				WithKeyExpr("k")
			Expect(forEach.KeyExpr()).To(Equal("k"))
		})

		It("should set value expression", func() {
			forEach := defkit.ForEachMap().
				Over("parameter.labels").
				WithValExpr("v")
			Expect(forEach.ValExpr()).To(Equal("v"))
		})
	})

	Context("ParamRef", func() {
		It("should create parameter reference", func() {
			ref := defkit.ParamRef("image")
			Expect(ref).NotTo(BeNil())
			Expect(ref.Path()).To(Equal("parameter.image"))
		})

		It("should reference nested parameter", func() {
			ref := defkit.ParamRef("config.port")
			Expect(ref).NotTo(BeNil())
			Expect(ref.Path()).To(Equal("parameter.config.port"))
		})
	})

	Context("InlineArrayValue", func() {
		It("should create inline array with fields and store correct values", func() {
			port := defkit.Int("port")
			arr := defkit.InlineArray(map[string]defkit.Value{
				"containerPort": port,
			})
			Expect(arr.Fields()).To(HaveLen(1))
			Expect(arr.Fields()).To(HaveKey("containerPort"))
			Expect(arr.Fields()["containerPort"]).To(Equal(port))
		})

		It("should create inline array with multiple fields and store correct values", func() {
			port := defkit.Int("port")
			protocol := defkit.Lit("TCP")
			arr := defkit.InlineArray(map[string]defkit.Value{
				"containerPort": port,
				"protocol":      protocol,
			})
			Expect(arr.Fields()).To(HaveLen(2))
			Expect(arr.Fields()["containerPort"]).To(Equal(port))
			Expect(arr.Fields()["protocol"]).To(Equal(protocol))
		})

		It("should implement Value interface", func() {
			port := defkit.Int("port")
			arr := defkit.InlineArray(map[string]defkit.Value{
				"containerPort": port,
			})
			var v defkit.Value = arr // compile-time check
			Expect(v).NotTo(BeNil())
			Expect(arr.Fields()["containerPort"]).To(Equal(port))
		})
	})
})
