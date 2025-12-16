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

	Describe("StringParam", func() {
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
	})

	Describe("IntParam", func() {
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

	Describe("BoolParam", func() {
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

	Describe("FloatParam", func() {
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

	Describe("ArrayParam", func() {
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

	Describe("MapParam", func() {
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

	Describe("StructParam", func() {
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

	Describe("EnumParam", func() {
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

	Describe("OneOfParam", func() {
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

	Describe("Parameter as Variable Pattern", func() {
		Describe("Comparison methods", func() {
			It("should create Eq condition from IntParam", func() {
				replicas := defkit.Int("replicas")
				cond := replicas.Eq(3)
				Expect(cond).NotTo(BeNil())
				// Check it implements Condition interface
				_, ok := cond.(defkit.Condition)
				Expect(ok).To(BeTrue())
			})

			It("should create comparison conditions from StringParam", func() {
				status := defkit.String("status")

				// Eq
				eqCond := status.Eq("running")
				Expect(eqCond).NotTo(BeNil())

				// Ne
				neCond := status.Ne("error")
				Expect(neCond).NotTo(BeNil())
			})

			It("should create numeric comparison conditions", func() {
				replicas := defkit.Int("replicas")

				// Gt
				gtCond := replicas.Gt(1)
				Expect(gtCond).NotTo(BeNil())

				// Gte
				gteCond := replicas.Gte(1)
				Expect(gteCond).NotTo(BeNil())

				// Lt
				ltCond := replicas.Lt(10)
				Expect(ltCond).NotTo(BeNil())

				// Lte
				lteCond := replicas.Lte(10)
				Expect(lteCond).NotTo(BeNil())
			})
		})

		Describe("Arithmetic expressions", func() {
			It("should create Add expression from IntParam", func() {
				replicas := defkit.Int("replicas")
				expr := replicas.Add(1)
				Expect(expr).NotTo(BeNil())
				// Check it implements Value interface
				_, ok := expr.(defkit.Value)
				Expect(ok).To(BeTrue())
			})

			It("should create arithmetic expressions from IntParam", func() {
				replicas := defkit.Int("replicas")

				// Add
				addExpr := replicas.Add(1)
				Expect(addExpr).NotTo(BeNil())

				// Sub
				subExpr := replicas.Sub(1)
				Expect(subExpr).NotTo(BeNil())

				// Mul
				mulExpr := replicas.Mul(2)
				Expect(mulExpr).NotTo(BeNil())

				// Div
				divExpr := replicas.Div(2)
				Expect(divExpr).NotTo(BeNil())
			})
		})

		Describe("String expressions", func() {
			It("should create Concat expression from StringParam", func() {
				name := defkit.String("name")
				expr := name.Concat("-suffix")
				Expect(expr).NotTo(BeNil())
				// Check it implements Value interface
				_, ok := expr.(defkit.Value)
				Expect(ok).To(BeTrue())
			})

			It("should create Prepend expression from StringParam", func() {
				name := defkit.String("name")
				expr := name.Prepend("prefix-")
				Expect(expr).NotTo(BeNil())
				// Check it implements Value interface
				_, ok := expr.(defkit.Value)
				Expect(ok).To(BeTrue())
			})
		})

		Describe("Struct field access", func() {
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
				Expect(cond).NotTo(BeNil())
			})

			It("should create Eq condition from field ref", func() {
				config := defkit.Struct("config")
				fieldRef := config.Field("enabled")
				cond := fieldRef.Eq(true)
				Expect(cond).NotTo(BeNil())
			})
		})
	})
})
