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

var _ = Describe("Parameters", func() {

	Context("StringParam", func() {
		It("should create a string parameter with name", func() {
			p := defkit.String("image")
			Expect(p.Name()).To(Equal("image"))
			Expect(p.IsRequired()).To(BeFalse())
			Expect(p.IsOptional()).To(BeTrue())
			Expect(p.HasDefault()).To(BeFalse())
		})

		It("should support required modifier", func() {
			p := defkit.String("image").Required()
			Expect(p.IsRequired()).To(BeTrue())
			Expect(p.IsOptional()).To(BeFalse())
		})

		It("should support default value", func() {
			p := defkit.String("image").Default("nginx:latest")
			Expect(p.HasDefault()).To(BeTrue())
			Expect(p.GetDefault()).To(Equal("nginx:latest"))
		})

		It("should support description", func() {
			p := defkit.String("image").Description("Container image to use")
			Expect(p.GetDescription()).To(Equal("Container image to use"))
		})

		It("should support fluent chaining", func() {
			p := defkit.String("image").
				Required().
				Default("nginx:latest").
				Description("Container image")
			Expect(p.Name()).To(Equal("image"))
			Expect(p.IsRequired()).To(BeTrue())
			Expect(p.GetDefault()).To(Equal("nginx:latest"))
			Expect(p.GetDescription()).To(Equal("Container image"))
		})

		It("should support ForceOptional to stay optional even with a default", func() {
			p := defkit.String("policy").Default("Honor").ForceOptional().Enum("Honor", "Ignore")
			Expect(p.HasDefault()).To(BeTrue())
			Expect(p.GetDefault()).To(Equal("Honor"))
			Expect(p.IsForceOptional()).To(BeTrue())
			Expect(p.IsRequired()).To(BeFalse())
		})

		It("should not be force-optional by default", func() {
			p := defkit.String("policy").Default("Honor")
			Expect(p.IsForceOptional()).To(BeFalse())
		})
	})

	Context("IntParam", func() {
		It("should create an int parameter with name", func() {
			p := defkit.Int("replicas")
			Expect(p.Name()).To(Equal("replicas"))
			Expect(p.IsRequired()).To(BeFalse())
		})

		It("should support default value", func() {
			p := defkit.Int("replicas").Default(3)
			Expect(p.HasDefault()).To(BeTrue())
			Expect(p.GetDefault()).To(Equal(3))
		})

		It("should support fluent chaining", func() {
			p := defkit.Int("port").
				Required().
				Default(8080).
				Description("Service port")
			Expect(p.Name()).To(Equal("port"))
			Expect(p.IsRequired()).To(BeTrue())
			Expect(p.GetDefault()).To(Equal(8080))
		})
	})

	Context("BoolParam", func() {
		It("should create a bool parameter with name", func() {
			p := defkit.Bool("enabled")
			Expect(p.Name()).To(Equal("enabled"))
			Expect(p.IsRequired()).To(BeFalse())
		})

		It("should support default value", func() {
			p := defkit.Bool("enabled").Default(true)
			Expect(p.HasDefault()).To(BeTrue())
			Expect(p.GetDefault()).To(Equal(true))
		})

		It("should support fluent chaining", func() {
			p := defkit.Bool("debug").
				Optional().
				Default(false).
				Description("Enable debug mode")
			Expect(p.Name()).To(Equal("debug"))
			Expect(p.IsOptional()).To(BeTrue())
			Expect(p.GetDefault()).To(Equal(false))
		})
	})

	Context("FloatParam", func() {
		It("should create a float parameter with name", func() {
			p := defkit.Float("cpu")
			Expect(p.Name()).To(Equal("cpu"))
			Expect(p.IsRequired()).To(BeFalse())
		})

		It("should support default value", func() {
			p := defkit.Float("memory").Default(0.5)
			Expect(p.HasDefault()).To(BeTrue())
			Expect(p.GetDefault()).To(Equal(0.5))
		})

		It("should support fluent chaining", func() {
			p := defkit.Float("ratio").
				Required().
				Default(1.0).
				Description("Resource ratio")
			Expect(p.Name()).To(Equal("ratio"))
			Expect(p.IsRequired()).To(BeTrue())
			Expect(p.GetDefault()).To(Equal(1.0))
		})
	})

	Context("ArrayParam", func() {
		It("should create an array parameter with name", func() {
			p := defkit.Array("ports")
			Expect(p.Name()).To(Equal("ports"))
			Expect(p.IsRequired()).To(BeFalse())
		})

		It("should support element type specification", func() {
			p := defkit.Array("ports").Of(defkit.ParamTypeInt)
			Expect(p.ElementType()).To(Equal(defkit.ParamTypeInt))
		})

		It("should support default value", func() {
			p := defkit.Array("tags").Default([]any{"app", "web"})
			Expect(p.HasDefault()).To(BeTrue())
			Expect(p.GetDefault()).To(Equal([]any{"app", "web"}))
		})

		It("should support fluent chaining", func() {
			p := defkit.Array("volumes").
				Of(defkit.ParamTypeString).
				Required().
				Description("Volume mounts")
			Expect(p.Name()).To(Equal("volumes"))
			Expect(p.IsRequired()).To(BeTrue())
			Expect(p.ElementType()).To(Equal(defkit.ParamTypeString))
		})
	})

	Context("MapParam", func() {
		It("should create a map parameter with name", func() {
			p := defkit.Map("labels")
			Expect(p.Name()).To(Equal("labels"))
			Expect(p.IsRequired()).To(BeFalse())
		})

		It("should support value type specification", func() {
			p := defkit.Map("annotations").Of(defkit.ParamTypeString)
			Expect(p.ValueType()).To(Equal(defkit.ParamTypeString))
		})

		It("should support default value", func() {
			p := defkit.Map("env").Default(map[string]any{"DEBUG": "true"})
			Expect(p.HasDefault()).To(BeTrue())
			Expect(p.GetDefault()).To(Equal(map[string]any{"DEBUG": "true"}))
		})

		It("should support fluent chaining", func() {
			p := defkit.Map("config").
				Of(defkit.ParamTypeString).
				Optional().
				Description("Configuration map")
			Expect(p.Name()).To(Equal("config"))
			Expect(p.IsOptional()).To(BeTrue())
			Expect(p.ValueType()).To(Equal(defkit.ParamTypeString))
		})
	})

	Context("StructParam", func() {
		It("should create a struct parameter with name", func() {
			p := defkit.Struct("resources")
			Expect(p.Name()).To(Equal("resources"))
			Expect(p.IsRequired()).To(BeFalse())
		})

		It("should support field definitions", func() {
			p := defkit.Struct("resources").Fields(
				defkit.Field("cpu", defkit.ParamTypeString),
				defkit.Field("memory", defkit.ParamTypeString),
			)
			Expect(p.GetFields()).To(HaveLen(2))
			Expect(p.GetField("cpu")).NotTo(BeNil())
			Expect(p.GetField("memory")).NotTo(BeNil())
		})

		It("should return nil for non-existent field", func() {
			p := defkit.Struct("resources").Fields(
				defkit.Field("cpu", defkit.ParamTypeString),
			)
			Expect(p.GetField("nonexistent")).To(BeNil())
		})

		It("should support field modifiers", func() {
			f := defkit.Field("cpu", defkit.ParamTypeString).
				Required().
				Default("100m").
				Description("CPU request")
			Expect(f.Name()).To(Equal("cpu"))
			Expect(f.FieldType()).To(Equal(defkit.ParamTypeString))
			Expect(f.IsRequired()).To(BeTrue())
			Expect(f.HasDefault()).To(BeTrue())
			Expect(f.GetDefault()).To(Equal("100m"))
			Expect(f.GetDescription()).To(Equal("CPU request"))
		})

		It("should support nested structs", func() {
			requests := defkit.Struct("requests").Fields(
				defkit.Field("cpu", defkit.ParamTypeString),
				defkit.Field("memory", defkit.ParamTypeString),
			)
			p := defkit.Struct("resources").Fields(
				defkit.Field("requests", defkit.ParamTypeStruct).Nested(requests),
			)
			reqField := p.GetField("requests")
			Expect(reqField).NotTo(BeNil())
			Expect(reqField.GetNested()).NotTo(BeNil())
			Expect(reqField.GetNested().GetField("cpu")).NotTo(BeNil())
		})

		It("should support fluent chaining", func() {
			p := defkit.Struct("container").
				Fields(
					defkit.Field("name", defkit.ParamTypeString).Required(),
					defkit.Field("image", defkit.ParamTypeString).Required(),
					defkit.Field("port", defkit.ParamTypeInt).Default(80),
				).
				Required().
				Description("Container specification")
			Expect(p.Name()).To(Equal("container"))
			Expect(p.IsRequired()).To(BeTrue())
			Expect(p.GetDescription()).To(Equal("Container specification"))
			Expect(p.GetFields()).To(HaveLen(3))
		})
	})

	Context("EnumParam", func() {
		It("should create an enum parameter with name", func() {
			p := defkit.Enum("protocol")
			Expect(p.Name()).To(Equal("protocol"))
			Expect(p.IsRequired()).To(BeFalse())
		})

		It("should support enum values", func() {
			p := defkit.Enum("protocol").Values("TCP", "UDP", "SCTP")
			Expect(p.GetValues()).To(Equal([]string{"TCP", "UDP", "SCTP"}))
		})

		It("should support default value", func() {
			p := defkit.Enum("protocol").
				Values("TCP", "UDP").
				Default("TCP")
			Expect(p.HasDefault()).To(BeTrue())
			Expect(p.GetDefault()).To(Equal("TCP"))
		})

		It("should support fluent chaining", func() {
			p := defkit.Enum("restartPolicy").
				Values("Always", "OnFailure", "Never").
				Required().
				Default("Always").
				Description("Pod restart policy")
			Expect(p.Name()).To(Equal("restartPolicy"))
			Expect(p.IsRequired()).To(BeTrue())
			Expect(p.GetValues()).To(HaveLen(3))
		})
	})

	Context("OneOfParam", func() {
		It("should create a oneof parameter with name", func() {
			p := defkit.OneOf("probe")
			Expect(p.Name()).To(Equal("probe"))
			Expect(p.GetDiscriminator()).To(Equal("type")) // default
		})

		It("should support custom discriminator", func() {
			p := defkit.OneOf("probe").Discriminator("kind")
			Expect(p.GetDiscriminator()).To(Equal("kind"))
		})

		It("should support variant definitions", func() {
			p := defkit.OneOf("probe").Variants(
				defkit.Variant("http").Fields(
					defkit.Field("path", defkit.ParamTypeString).Required(),
					defkit.Field("port", defkit.ParamTypeInt).Required(),
				),
				defkit.Variant("tcp").Fields(
					defkit.Field("port", defkit.ParamTypeInt).Required(),
				),
			)
			Expect(p.GetVariants()).To(HaveLen(2))
			Expect(p.GetVariant("http")).NotTo(BeNil())
			Expect(p.GetVariant("tcp")).NotTo(BeNil())
			Expect(p.GetVariant("http").GetFields()).To(HaveLen(2))
		})

		It("should return nil for non-existent variant", func() {
			p := defkit.OneOf("probe").Variants(
				defkit.Variant("http"),
			)
			Expect(p.GetVariant("nonexistent")).To(BeNil())
		})

		It("should support fluent chaining", func() {
			p := defkit.OneOf("storage").
				Discriminator("type").
				Variants(
					defkit.Variant("pvc"),
					defkit.Variant("emptyDir"),
					defkit.Variant("hostPath"),
				).
				Required().
				Description("Storage configuration")
			Expect(p.Name()).To(Equal("storage"))
			Expect(p.IsRequired()).To(BeTrue())
			Expect(p.GetDiscriminator()).To(Equal("type"))
			Expect(p.GetVariants()).To(HaveLen(3))
		})
	})

	Context("Parameter as Variable Pattern", func() {
		Context("Comparison methods", func() {
			It("should create Eq condition from IntParam", func() {
				replicas := defkit.Int("replicas")
				cond := replicas.Eq(3)
				pcc, ok := cond.(*defkit.ParamCompareCondition)
				Expect(ok).To(BeTrue(), "expected *ParamCompareCondition")
				Expect(pcc.ParamName()).To(Equal("replicas"))
				Expect(pcc.Op()).To(Equal("=="))
				Expect(pcc.CompareValue()).To(Equal(3))
			})

			It("should create comparison conditions from StringParam", func() {
				status := defkit.String("status")

				eqCond, ok := status.Eq("running").(*defkit.ParamCompareCondition)
				Expect(ok).To(BeTrue())
				Expect(eqCond.ParamName()).To(Equal("status"))
				Expect(eqCond.Op()).To(Equal("=="))
				Expect(eqCond.CompareValue()).To(Equal("running"))

				neCond, ok := status.Ne("error").(*defkit.ParamCompareCondition)
				Expect(ok).To(BeTrue())
				Expect(neCond.ParamName()).To(Equal("status"))
				Expect(neCond.Op()).To(Equal("!="))
				Expect(neCond.CompareValue()).To(Equal("error"))
			})

			It("should create numeric comparison conditions", func() {
				replicas := defkit.Int("replicas")

				gtCond, ok := replicas.Gt(1).(*defkit.ParamCompareCondition)
				Expect(ok).To(BeTrue())
				Expect(gtCond.Op()).To(Equal(">"))
				Expect(gtCond.CompareValue()).To(Equal(1))

				gteCond, ok := replicas.Gte(1).(*defkit.ParamCompareCondition)
				Expect(ok).To(BeTrue())
				Expect(gteCond.Op()).To(Equal(">="))
				Expect(gteCond.CompareValue()).To(Equal(1))

				ltCond, ok := replicas.Lt(10).(*defkit.ParamCompareCondition)
				Expect(ok).To(BeTrue())
				Expect(ltCond.Op()).To(Equal("<"))
				Expect(ltCond.CompareValue()).To(Equal(10))

				lteCond, ok := replicas.Lte(10).(*defkit.ParamCompareCondition)
				Expect(ok).To(BeTrue())
				Expect(lteCond.Op()).To(Equal("<="))
				Expect(lteCond.CompareValue()).To(Equal(10))
			})
		})

		Context("Arithmetic expressions", func() {
			It("should create Add expression from IntParam", func() {
				replicas := defkit.Int("replicas")
				expr := replicas.Add(1)
				arith, ok := expr.(*defkit.ParamArithExpr)
				Expect(ok).To(BeTrue(), "expected *ParamArithExpr")
				Expect(arith.ParamName()).To(Equal("replicas"))
				Expect(arith.Op()).To(Equal("+"))
				Expect(arith.ArithValue()).To(Equal(1))
			})

			It("should create arithmetic expressions from IntParam", func() {
				replicas := defkit.Int("replicas")

				addExpr, ok := replicas.Add(1).(*defkit.ParamArithExpr)
				Expect(ok).To(BeTrue())
				Expect(addExpr.Op()).To(Equal("+"))
				Expect(addExpr.ArithValue()).To(Equal(1))

				subExpr, ok := replicas.Sub(1).(*defkit.ParamArithExpr)
				Expect(ok).To(BeTrue())
				Expect(subExpr.Op()).To(Equal("-"))
				Expect(subExpr.ArithValue()).To(Equal(1))

				mulExpr, ok := replicas.Mul(2).(*defkit.ParamArithExpr)
				Expect(ok).To(BeTrue())
				Expect(mulExpr.Op()).To(Equal("*"))
				Expect(mulExpr.ArithValue()).To(Equal(2))

				divExpr, ok := replicas.Div(2).(*defkit.ParamArithExpr)
				Expect(ok).To(BeTrue())
				Expect(divExpr.Op()).To(Equal("/"))
				Expect(divExpr.ArithValue()).To(Equal(2))
			})
		})

		Context("String expressions", func() {
			It("should create Concat expression from StringParam", func() {
				name := defkit.String("name")
				expr := name.Concat("-suffix")
				concat, ok := expr.(*defkit.ParamConcatExpr)
				Expect(ok).To(BeTrue(), "expected *ParamConcatExpr")
				Expect(concat.ParamName()).To(Equal("name"))
				Expect(concat.Suffix()).To(Equal("-suffix"))
				Expect(concat.Prefix()).To(BeEmpty())
			})

			It("should create Prepend expression from StringParam", func() {
				name := defkit.String("name")
				expr := name.Prepend("prefix-")
				concat, ok := expr.(*defkit.ParamConcatExpr)
				Expect(ok).To(BeTrue(), "expected *ParamConcatExpr")
				Expect(concat.ParamName()).To(Equal("name"))
				Expect(concat.Prefix()).To(Equal("prefix-"))
				Expect(concat.Suffix()).To(BeEmpty())
			})
		})

		Context("Struct field access", func() {
			It("should create field reference from StructParam", func() {
				config := defkit.Struct("config").
					Fields(
						defkit.Field("host", defkit.ParamTypeString),
						defkit.Field("port", defkit.ParamTypeInt),
					)
				fieldRef := config.Field("port")
				Expect(fieldRef).NotTo(BeNil())
				Expect(fieldRef.ParamName()).To(Equal("config"))
				Expect(fieldRef.FieldPath()).To(Equal("port"))
			})

			It("should create nested field reference", func() {
				config := defkit.Struct("config")
				fieldRef := config.Field("database.host")
				Expect(fieldRef.ParamName()).To(Equal("config"))
				Expect(fieldRef.FieldPath()).To(Equal("database.host"))
			})

			It("should create IsSet condition from field ref", func() {
				config := defkit.Struct("config")
				fieldRef := config.Field("port")
				cond := fieldRef.IsSet()
				isSet, ok := cond.(*defkit.ParamPathIsSetCondition)
				Expect(ok).To(BeTrue(), "expected *ParamPathIsSetCondition")
				Expect(isSet.Path()).To(Equal("config.port"))
			})

			It("should create Eq condition from field ref", func() {
				config := defkit.Struct("config")
				fieldRef := config.Field("enabled")
				cond := fieldRef.Eq(true)
				pcc, ok := cond.(*defkit.ParamCompareCondition)
				Expect(ok).To(BeTrue(), "expected *ParamCompareCondition")
				Expect(pcc.ParamName()).To(Equal("config.enabled"))
				Expect(pcc.Op()).To(Equal("=="))
				Expect(pcc.CompareValue()).To(Equal(true))
			})
		})
	})

	Context("Additional Param Types", func() {
		It("should create StringList parameter", func() {
			p := defkit.StringList("tags")
			Expect(p.Name()).To(Equal("tags"))
			Expect(p.ElementType()).To(Equal(defkit.ParamTypeString))
			Expect(p.IsRequired()).To(BeFalse())
		})

		It("should create IntList parameter", func() {
			p := defkit.IntList("ports")
			Expect(p.Name()).To(Equal("ports"))
			Expect(p.ElementType()).To(Equal(defkit.ParamTypeInt))
			Expect(p.IsRequired()).To(BeFalse())
		})

		It("should create StringKeyMap parameter", func() {
			p := defkit.StringKeyMap("labels")
			Expect(p.Name()).To(Equal("labels"))
			Expect(p.GetType()).To(Equal(defkit.ParamTypeMap))
		})

		It("should create List parameter", func() {
			p := defkit.List("items")
			Expect(p.Name()).To(Equal("items"))
			Expect(p.IsRequired()).To(BeFalse())
		})

		It("should create Object parameter", func() {
			p := defkit.Object("config")
			Expect(p.Name()).To(Equal("config"))
			Expect(p.IsRequired()).To(BeFalse())
		})
	})

	Context("IsSet and NotSet conditions", func() {
		It("should create IsSet condition from param", func() {
			replicas := defkit.Int("replicas")
			cond := replicas.IsSet()
			isSet, ok := cond.(*defkit.IsSetCondition)
			Expect(ok).To(BeTrue(), "expected *IsSetCondition")
			Expect(isSet.ParamName()).To(Equal("replicas"))
		})

		It("should create NotSet condition from param", func() {
			replicas := defkit.Int("replicas")
			cond := replicas.NotSet()
			notCond, ok := cond.(*defkit.NotCondition)
			Expect(ok).To(BeTrue(), "expected *NotCondition")
			inner, ok := notCond.Inner().(*defkit.IsSetCondition)
			Expect(ok).To(BeTrue(), "expected inner *IsSetCondition")
			Expect(inner.ParamName()).To(Equal("replicas"))
		})
	})

	Context("BoolParam conditions", func() {
		It("should create IsTrue condition", func() {
			enabled := defkit.Bool("enabled")
			cond := enabled.IsTrue()
			truthy, ok := cond.(*defkit.TruthyCondition)
			Expect(ok).To(BeTrue(), "expected *TruthyCondition")
			Expect(truthy.ParamName()).To(Equal("enabled"))
		})

		It("should create IsFalse condition", func() {
			enabled := defkit.Bool("enabled")
			cond := enabled.IsFalse()
			falsy, ok := cond.(*defkit.FalsyCondition)
			Expect(ok).To(BeTrue(), "expected *FalsyCondition")
			Expect(falsy.ParamName()).To(Equal("enabled"))
		})
	})

	Context("ArrayParam additional methods", func() {
		It("should set min and max items constraints", func() {
			p := defkit.Array("tags").
				Of(defkit.ParamTypeString).
				MinItems(1).
				MaxItems(10)
			Expect(*p.GetMinItems()).To(Equal(1))
			Expect(*p.GetMaxItems()).To(Equal(10))
		})

		It("should create length constraint conditions", func() {
			arr := defkit.Array("items")
			gteCond := arr.LenGte(1)
			lenCond, ok := gteCond.(*defkit.LenCondition)
			Expect(ok).To(BeTrue(), "expected *LenCondition")
			Expect(lenCond.ParamName()).To(Equal("items"))
			Expect(lenCond.Op()).To(Equal(">="))
			Expect(lenCond.Length()).To(Equal(1))
		})

		It("should set WithFields for array items", func() {
			p := defkit.List("ports").WithFields(
				defkit.Int("port").Required(),
				defkit.String("name"),
			)
			Expect(p.GetFields()).To(HaveLen(2))
		})
	})

	Context("MapParam Optional method", func() {
		It("should set map as optional", func() {
			p := defkit.Map("labels").Optional()
			Expect(p.IsOptional()).To(BeTrue())
		})

		It("should set map as required", func() {
			p := defkit.Map("labels").Required()
			Expect(p.IsRequired()).To(BeTrue())
		})
	})

	Context("StructParam Optional method", func() {
		It("should set struct as optional", func() {
			p := defkit.Struct("config").Optional()
			Expect(p.IsOptional()).To(BeTrue())
		})
	})

	Context("EnumParam Optional method", func() {
		It("should set enum as optional", func() {
			p := defkit.Enum("protocol").
				Values("TCP", "UDP").
				Optional()
			Expect(p.IsOptional()).To(BeTrue())
		})
	})

	Context("OneOfParam Optional method", func() {
		It("should set oneof as optional", func() {
			p := defkit.OneOf("storage").Optional()
			Expect(p.IsOptional()).To(BeTrue())
		})
	})

	Context("OneOfParam Default method", func() {
		It("should set default variant name", func() {
			p := defkit.OneOf("type").Default("emptyDir")
			Expect(p.HasDefault()).To(BeTrue())
			Expect(p.GetDefault()).To(Equal("emptyDir"))
		})

		It("should support fluent chaining with Default", func() {
			p := defkit.OneOf("type").
				Default("emptyDir").
				Description("Volume type").
				Variants(
					defkit.Variant("pvc").Fields(
						defkit.Field("claimName", defkit.ParamTypeString).Required(),
					),
					defkit.Variant("emptyDir"),
				)
			Expect(p.HasDefault()).To(BeTrue())
			Expect(p.GetDefault()).To(Equal("emptyDir"))
			Expect(p.GetDescription()).To(Equal("Volume type"))
			Expect(p.GetVariants()).To(HaveLen(2))
		})
	})

	Context("IntParam Optional method", func() {
		It("should set int as optional", func() {
			p := defkit.Int("replicas").Optional()
			Expect(p.IsOptional()).To(BeTrue())
		})
	})

	Context("FloatParam Optional method", func() {
		It("should set float as optional", func() {
			p := defkit.Float("ratio").Optional()
			Expect(p.IsOptional()).To(BeTrue())
		})
	})

	Context("StringParam Enum method", func() {
		It("should set enum values on string param", func() {
			p := defkit.String("protocol").Enum("TCP", "UDP", "SCTP")
			Expect(p.GetEnumValues()).To(ConsistOf("TCP", "UDP", "SCTP"))
		})
	})

	Context("ArrayParam Length Conditions", func() {
		It("should create LenLt condition", func() {
			arr := defkit.Array("items")
			cond := arr.LenLt(10)
			lenCond, ok := cond.(*defkit.LenCondition)
			Expect(ok).To(BeTrue(), "expected *LenCondition")
			Expect(lenCond.ParamName()).To(Equal("items"))
			Expect(lenCond.Op()).To(Equal("<"))
			Expect(lenCond.Length()).To(Equal(10))
		})

		It("should create LenLte condition", func() {
			arr := defkit.Array("items")
			cond := arr.LenLte(10)
			lenCond, ok := cond.(*defkit.LenCondition)
			Expect(ok).To(BeTrue(), "expected *LenCondition")
			Expect(lenCond.ParamName()).To(Equal("items"))
			Expect(lenCond.Op()).To(Equal("<="))
			Expect(lenCond.Length()).To(Equal(10))
		})
	})

	Context("ArrayParam WithSchema methods", func() {
		It("should set WithSchema on array", func() {
			p := defkit.Array("items").WithSchema("{ name: string }")
			Expect(p.GetSchema()).To(Equal("{ name: string }"))
		})

		It("should set WithSchemaRef on array", func() {
			p := defkit.Array("items").WithSchemaRef("#Schema")
			Expect(p.GetSchemaRef()).To(Equal("#Schema"))
		})
	})

	Context("DynamicMap param", func() {
		It("should create dynamic map", func() {
			p := defkit.DynamicMap()
			Expect(p.IsDynamicMap()).To(BeTrue())
		})

		It("should set ValueType", func() {
			p := defkit.DynamicMap().ValueType(defkit.ParamTypeString)
			Expect(p.GetValueType()).To(Equal(defkit.ParamTypeString))
		})

		It("should set ValueTypeUnion", func() {
			p := defkit.DynamicMap().ValueTypeUnion("string | int")
			Expect(p.GetValueTypeUnion()).To(Equal("string | int"))
		})

		It("should set Description", func() {
			p := defkit.DynamicMap().Description("Dynamic key-value pairs")
			Expect(p.GetDescription()).To(Equal("Dynamic key-value pairs"))
		})
	})

	Context("MapParam additional methods", func() {
		It("should set Description", func() {
			p := defkit.Map("labels").Description("Key-value labels")
			Expect(p.GetDescription()).To(Equal("Key-value labels"))
		})

		It("should get ValueType after setting with Of", func() {
			p := defkit.Map("labels").Of(defkit.ParamTypeString)
			Expect(p.ValueType()).To(Equal(defkit.ParamTypeString))
		})
	})

	Context("StructParam additional methods", func() {
		It("should set WithSchemaRef on struct", func() {
			p := defkit.Struct("config").WithSchemaRef("#ConfigSchema")
			Expect(p.GetSchemaRef()).To(Equal("#ConfigSchema"))
		})
	})

	Context("OpenStruct param", func() {
		It("should create open struct", func() {
			p := defkit.OpenStruct()
			Expect(p.GetName()).To(Equal("")) // OpenStruct has no name - represents entire parameter schema
			Expect(p.IsOpen()).To(BeTrue())
		})

		It("should set description", func() {
			p := defkit.OpenStruct().Description("Open config")
			Expect(p.GetDescription()).To(Equal("Open config"))
		})

		It("should return correct type", func() {
			p := defkit.OpenStruct()
			Expect(p.GetType()).To(Equal(defkit.ParamTypeStruct))
		})

		It("should be optional by default", func() {
			p := defkit.OpenStruct()
			Expect(p.IsRequired()).To(BeFalse())
		})

		It("should have nil default", func() {
			p := defkit.OpenStruct()
			Expect(p.GetDefault()).To(BeNil())
		})
	})

	Context("OpenArray param", func() {
		It("should create open array", func() {
			p := defkit.OpenArray("items")
			Expect(p.Name()).To(Equal("items"))
		})

		It("should set description", func() {
			p := defkit.OpenArray("items").Description("Open items array")
			Expect(p.GetDescription()).To(Equal("Open items array"))
		})

		It("should return correct type", func() {
			p := defkit.OpenArray("items")
			Expect(p.GetType()).To(Equal(defkit.ParamTypeArray))
		})

		It("should be optional by default", func() {
			p := defkit.OpenArray("items")
			Expect(p.IsRequired()).To(BeFalse())
		})

		It("should have nil default", func() {
			p := defkit.OpenArray("items")
			Expect(p.GetDefault()).To(BeNil())
		})
	})

	Context("ParamPath", func() {
		It("should create param path", func() {
			path := defkit.ParamPath("config.database.host")
			Expect(path).NotTo(BeNil())
			Expect(path.Path()).To(Equal("config.database.host"))
		})

		It("should create IsSet condition", func() {
			path := defkit.ParamPath("config.port")
			cond := path.IsSet()
			isSet, ok := cond.(*defkit.ParamPathIsSetCondition)
			Expect(ok).To(BeTrue(), "expected *ParamPathIsSetCondition")
			Expect(isSet.Path()).To(Equal("config.port"))
		})
	})

	Context("StructField Name method", func() {
		It("should return field name", func() {
			f := defkit.Field("port", defkit.ParamTypeInt)
			Expect(f.Name()).To(Equal("port"))
		})
	})

	Context("StructField Required, Optional, Default", func() {
		It("should mark field as required", func() {
			f := defkit.Field("port", defkit.ParamTypeInt).Required()
			Expect(f.IsRequired()).To(BeTrue())
		})

		It("should mark field as optional", func() {
			f := defkit.Field("port", defkit.ParamTypeInt).Optional()
			Expect(f.IsRequired()).To(BeFalse())
		})

		It("should set default value", func() {
			f := defkit.Field("port", defkit.ParamTypeInt).Default(8080)
			Expect(f.HasDefault()).To(BeTrue())
			Expect(f.GetDefault()).To(Equal(8080))
		})
	})

	Context("IntParam Ne condition", func() {
		It("should create Ne condition", func() {
			p := defkit.Int("count")
			cond := p.Ne(0)
			pcc, ok := cond.(*defkit.ParamCompareCondition)
			Expect(ok).To(BeTrue(), "expected *ParamCompareCondition")
			Expect(pcc.ParamName()).To(Equal("count"))
			Expect(pcc.Op()).To(Equal("!="))
			Expect(pcc.CompareValue()).To(Equal(0))
		})
	})

	Context("Short method", func() {
		It("should set short flag on StringParam", func() {
			p := defkit.String("image").Short("i")
			Expect(p.GetShort()).To(Equal("i"))
		})
		It("should set short flag on IntParam", func() {
			p := defkit.Int("port").Short("p")
			Expect(p.GetShort()).To(Equal("p"))
		})
		It("should set short flag on BoolParam", func() {
			p := defkit.Bool("debug").Short("d")
			Expect(p.GetShort()).To(Equal("d"))
		})
		It("should set short flag on EnumParam", func() {
			p := defkit.Enum("protocol").Values("TCP", "UDP").Short("p")
			Expect(p.GetShort()).To(Equal("p"))
		})
		It("should return empty string when not set", func() {
			p := defkit.String("image")
			Expect(p.GetShort()).To(BeEmpty())
		})
		It("should support fluent chaining with other methods", func() {
			p := defkit.String("image").Required().Description("Container image").Short("i")
			Expect(p.Name()).To(Equal("image"))
			Expect(p.IsRequired()).To(BeTrue())
			Expect(p.GetDescription()).To(Equal("Container image"))
			Expect(p.GetShort()).To(Equal("i"))
		})
	})

	Context("Ignore method", func() {
		It("should mark StringParam as ignored", func() {
			p := defkit.String("port").Ignore()
			Expect(p.IsIgnore()).To(BeTrue())
		})
		It("should mark IntParam as ignored", func() {
			p := defkit.Int("port").Ignore()
			Expect(p.IsIgnore()).To(BeTrue())
		})
		It("should mark BoolParam as ignored", func() {
			p := defkit.Bool("debug").Ignore()
			Expect(p.IsIgnore()).To(BeTrue())
		})
		It("should mark EnumParam as ignored", func() {
			p := defkit.Enum("type").Values("A", "B").Ignore()
			Expect(p.IsIgnore()).To(BeTrue())
		})
		It("should not be ignored by default", func() {
			p := defkit.String("image")
			Expect(p.IsIgnore()).To(BeFalse())
		})
		It("should support fluent chaining with Short and other methods", func() {
			p := defkit.Int("port").Ignore().Description("Deprecated field").Short("p")
			Expect(p.IsIgnore()).To(BeTrue())
			Expect(p.GetShort()).To(Equal("p"))
			Expect(p.GetDescription()).To(Equal("Deprecated field"))
		})
	})
})
