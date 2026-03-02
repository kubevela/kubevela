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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("CUEGenerator", func() {
	var gen *defkit.CUEGenerator

	BeforeEach(func() {
		gen = defkit.NewCUEGenerator()
	})

	Describe("GenerateParameterSchema", func() {
		It("should generate CUE for string parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.String("image").Required().Description("Container image"),
					defkit.String("tag").Default("latest").Description("Image tag"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("// +usage=Container image"))
			Expect(cue).To(ContainSubstring("image: string"))
			Expect(cue).To(ContainSubstring("// +usage=Image tag"))
			Expect(cue).To(ContainSubstring(`tag: *"latest" | string`))
		})

		It("should generate CUE for integer parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.Int("replicas").Default(1).Description("Number of replicas"),
					defkit.Int("port").Required(),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("replicas: *1 | int"))
			Expect(cue).To(ContainSubstring("port: int"))
		})

		It("should generate CUE for boolean parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.Bool("enabled").Default(true),
					defkit.Bool("debug").Default(false),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("enabled: *true | bool"))
			Expect(cue).To(ContainSubstring("debug: *false | bool"))
		})

		It("should generate CUE for array parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.StringList("cmd").Description("Commands"),
					defkit.List("env").Description("Environment variables"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("cmd?: [...string]"))
			// List() creates an untyped array which generates [..._]
			Expect(cue).To(ContainSubstring("env?: [..._]"))
		})

		It("should generate CUE for object parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.Object("config").Description("Configuration object"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("config?: {...}"))
		})

		It("should generate CUE for string-key map parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.StringKeyMap("labels").Description("Labels"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("labels?: [string]: string"))
		})

		It("should mark required parameters without ?", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.String("required").Required(),
					defkit.String("optional"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("required: string"))
			Expect(cue).To(ContainSubstring("optional?: string"))
		})

		It("should keep ? for ForceOptional parameters even with defaults", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.String("normalDefault").Default("Honor").Enum("Honor", "Ignore"),
					defkit.String("optionalDefault").Default("Honor").ForceOptional().Enum("Honor", "Ignore"),
				)

			cue := gen.GenerateParameterSchema(comp)

			// Normal default: no ? (field is always present)
			Expect(cue).To(ContainSubstring(`normalDefault: *"Honor" | "Ignore"`))
			Expect(cue).NotTo(ContainSubstring(`normalDefault?:`))

			// ForceOptional with default: has ? (field can be absent, defaults when present)
			Expect(cue).To(ContainSubstring(`optionalDefault?: *"Honor" | "Ignore"`))
		})

		It("should generate // +ignore directive for ignored parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.String("visible").Description("A visible field"),
					defkit.Enum("hidden").Values("A", "B").Default("A").Ignore().Description("An ignored field"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).NotTo(ContainSubstring("// +ignore\n\t// +usage=A visible field"))
			Expect(cue).To(ContainSubstring("// +ignore\n\t// +usage=An ignored field"))
		})

		It("should generate // +short directive for params with short flags", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.String("image").Required().Description("Container image").Short("i"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("// +usage=Container image"))
			Expect(cue).To(ContainSubstring("// +short=i"))
		})

		It("should generate both // +ignore and // +short directives in correct order", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.Int("port").Ignore().Description("Deprecated field, please use ports instead").Short("p"),
				)

			cue := gen.GenerateParameterSchema(comp)

			// Order should be: // +ignore, // +usage=..., // +short=p
			ignoreIdx := strings.Index(cue, "// +ignore")
			usageIdx := strings.Index(cue, "// +usage=Deprecated field")
			shortIdx := strings.Index(cue, "// +short=p")
			Expect(ignoreIdx).To(BeNumerically(">", 0))
			Expect(usageIdx).To(BeNumerically(">", ignoreIdx))
			Expect(shortIdx).To(BeNumerically(">", usageIdx))
		})
	})

	Describe("GenerateParameterSchema with complex types", func() {
		It("should generate CUE for struct parameters with nested fields", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.Struct("resources").Fields(
						defkit.Field("cpu", defkit.ParamTypeString).Default("100m"),
						defkit.Field("memory", defkit.ParamTypeString).Default("128Mi"),
					).Description("Resource limits"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("// +usage=Resource limits"))
			Expect(cue).To(ContainSubstring("resources?:"))
			Expect(cue).To(ContainSubstring("cpu:"))
			Expect(cue).To(ContainSubstring("memory:"))
		})

		It("should generate CUE for enum parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.Enum("protocol").Values("TCP", "UDP", "SCTP").Default("TCP"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring(`*"TCP"`))
			Expect(cue).To(ContainSubstring(`"UDP"`))
			Expect(cue).To(ContainSubstring(`"SCTP"`))
		})

		It("should generate CUE for float parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.Float("ratio").Default(0.5).Description("Scale ratio"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("// +usage=Scale ratio"))
			Expect(cue).To(ContainSubstring("ratio: *0.5 | float"))
		})

		It("should generate CUE for array parameters with fields", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.List("ports").WithFields(
						defkit.Int("port").Required(),
						defkit.String("name"),
						defkit.Enum("protocol").Values("TCP", "UDP").Default("TCP"),
					),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("ports?:"))
			Expect(cue).To(ContainSubstring("port:"))
			Expect(cue).To(ContainSubstring("name?:"))
			Expect(cue).To(ContainSubstring("protocol:"))
		})

		It("should generate CUE for nested array struct fields", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.Struct("selector").Fields(
						defkit.Field("matchExpressions", defkit.ParamTypeArray).
							Nested(defkit.Struct("matchExpression").Fields(
								defkit.Field("key", defkit.ParamTypeString).Required(),
								defkit.Field("operator", defkit.ParamTypeString),
							)),
					),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("matchExpressions?: [...{"))
			Expect(cue).To(ContainSubstring("key: string"))
			Expect(cue).To(ContainSubstring("operator?: string"))
		})
	})

	Describe("GenerateParameterSchema with OneOf parameters", func() {
		It("should generate discriminator field and conditional variant blocks", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.OneOf("type").
						Default("emptyDir").
						Description("Volume type").
						Variants(
							defkit.Variant("pvc").Fields(
								defkit.Field("claimName", defkit.ParamTypeString).Required(),
							),
							defkit.Variant("emptyDir").Fields(
								defkit.Field("medium", defkit.ParamTypeString).Default("").Enum("", "Memory"),
							),
						),
				)

			cue := gen.GenerateParameterSchema(comp)

			// Discriminator field with default
			Expect(cue).To(ContainSubstring(`*"emptyDir"`))
			Expect(cue).To(ContainSubstring(`"pvc"`))
			// Conditional blocks
			Expect(cue).To(ContainSubstring(`if type == "pvc"`))
			Expect(cue).To(ContainSubstring("claimName: string"))
			Expect(cue).To(ContainSubstring(`if type == "emptyDir"`))
			Expect(cue).To(ContainSubstring(`medium:`))
		})

		It("should generate OneOf inside array with shared fields", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.List("volumes").WithFields(
						defkit.String("name").Required(),
						defkit.OneOf("type").Default("emptyDir").Variants(
							defkit.Variant("pvc").Fields(
								defkit.Field("claimName", defkit.ParamTypeString).Required(),
							),
							defkit.Variant("emptyDir"),
						),
					),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("volumes?:"))
			Expect(cue).To(ContainSubstring("[...{"))
			Expect(cue).To(ContainSubstring("name: string"))
			Expect(cue).To(ContainSubstring(`type: *"emptyDir" | "pvc"`))
			Expect(cue).To(ContainSubstring(`if type == "pvc"`))
			Expect(cue).To(ContainSubstring("claimName: string"))
		})

		It("should omit conditional block for empty variants", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.OneOf("kind").
						Variants(
							defkit.Variant("simple"), // no fields
							defkit.Variant("complex").Fields(
								defkit.Field("config", defkit.ParamTypeString).Required(),
							),
						),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).NotTo(ContainSubstring(`if kind == "simple"`))
			Expect(cue).To(ContainSubstring(`if kind == "complex"`))
			Expect(cue).To(ContainSubstring("config: string"))
		})
	})

	Describe("GenerateFullDefinition with MapVariant", func() {
		It("should generate conditional field blocks in comprehension", func() {
			volumes := defkit.List("volumes")
			comp := defkit.NewComponent("test").
				Workload("batch/v1", "Job").
				Params(volumes).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("batch/v1", "Job").
							Set("spec.volumes",
								defkit.Each(volumes).
									Map(defkit.FieldMap{"name": defkit.FieldRef("name")}).
									MapVariant("type", "pvc", defkit.FieldMap{
										"persistentVolumeClaim.claimName": defkit.FieldRef("claimName"),
									}).
									MapVariant("type", "emptyDir", defkit.FieldMap{
										"emptyDir.medium": defkit.FieldRef("medium"),
									}),
							),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring("for v in parameter.volumes"))
			Expect(cue).To(ContainSubstring("name: v.name"))
			Expect(cue).To(ContainSubstring(`if v.type == "pvc"`))
			Expect(cue).To(ContainSubstring("persistentVolumeClaim.claimName: v.claimName"))
			Expect(cue).To(ContainSubstring(`if v.type == "emptyDir"`))
			Expect(cue).To(ContainSubstring("emptyDir.medium: v.medium"))
		})

		It("should generate optional fields inside variant blocks", func() {
			volumes := defkit.List("volumes")
			comp := defkit.NewComponent("test").
				Workload("batch/v1", "Job").
				Params(volumes).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("batch/v1", "Job").
							Set("spec.volumes",
								defkit.Each(volumes).
									Map(defkit.FieldMap{"name": defkit.FieldRef("name")}).
									MapVariant("type", "configMap", defkit.FieldMap{
										"configMap.name":  defkit.FieldRef("cmName"),
										"configMap.items": defkit.OptionalFieldRef("items"),
									}),
							),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring(`if v.type == "configMap"`))
			Expect(cue).To(ContainSubstring("configMap.name: v.cmName"))
			Expect(cue).To(ContainSubstring("if v.items != _|_"))
			Expect(cue).To(ContainSubstring("configMap.items: v.items"))
		})
	})

	Describe("Multi-conditional leaf values (condValues)", func() {
		It("should render both conditional values when same path is set with different conditions", func() {
			gen := defkit.NewCUEGenerator()

			volMounts := defkit.String("volumeMounts")
			volumes := defkit.String("volumes")

			comp := defkit.NewComponent("test-multi-cond").
				Description("Test multi-conditional values").
				AutodetectWorkload().
				Params(volMounts, volumes).
				Template(func(tpl *defkit.Template) {
					res := defkit.NewResource("batch/v1", "Job")
					res.
						If(volMounts.IsSet()).
						Set("spec.containers[0].volumeMounts", defkit.Lit("mountsArray")).
						EndIf().
						If(volumes.IsSet()).
						Set("spec.containers[0].volumeMounts", defkit.Lit("volumesArray")).
						EndIf()
					tpl.Output(res)
				})

			cue := gen.GenerateFullDefinition(comp)
			// Both conditions should appear for the same field
			Expect(cue).To(ContainSubstring(`if parameter["volumeMounts"] != _|_`))
			Expect(cue).To(ContainSubstring(`if parameter["volumes"] != _|_`))
			// The field should appear twice, each in its own if block
			Expect(strings.Count(cue, "volumeMounts:")).To(BeNumerically(">=", 2))
		})

		It("should not activate condValues when path is set unconditionally then conditionally", func() {
			gen := defkit.NewCUEGenerator()

			optional := defkit.String("opt")

			comp := defkit.NewComponent("test-uncond-then-cond").
				Description("Test unconditional then conditional").
				AutodetectWorkload().
				Params(optional).
				Template(func(tpl *defkit.Template) {
					res := defkit.NewResource("v1", "Pod")
					res.
						Set("spec.field", defkit.Lit("default")).
						SetIf(optional.IsSet(), "spec.field", optional)
					tpl.Output(res)
				})

			cue := gen.GenerateFullDefinition(comp)
			// Conditional should win (overwrites unconditional)
			Expect(cue).To(ContainSubstring("field: parameter.opt"))
		})
	})

	Describe("Decomposition with condValues (condValues + canDecomposeByCondition)", func() {
		It("should decompose struct when leaf nodes have condValues from SetIf with different And conditions", func() {
			// This is the webservice CPU/memory limit branching pattern:
			// Two SetIf calls with different conditions target the same leaf path,
			// creating condValues. The parent struct must still decompose correctly.
			gen := defkit.NewCUEGenerator()

			cpu := defkit.String("cpu")
			limit := defkit.Object("limit")

			comp := defkit.NewComponent("test-condval-decompose").
				Description("Test condValues decomposition").
				AutodetectWorkload().
				Params(cpu, limit).
				Template(func(tpl *defkit.Template) {
					res := defkit.NewResource("v1", "Pod")
					// When cpu is set and limit.cpu exists: use limit.cpu for limits, cpu for requests
					res.SetIf(defkit.And(cpu.IsSet(), defkit.PathExists("parameter.limit.cpu")),
						"spec.resources.requests.cpu", cpu)
					res.SetIf(defkit.And(cpu.IsSet(), defkit.PathExists("parameter.limit.cpu")),
						"spec.resources.limits.cpu", defkit.Reference("parameter.limit.cpu"))
					// When cpu is set but limit.cpu does NOT exist: use cpu for both
					res.SetIf(defkit.And(cpu.IsSet(), defkit.Not(defkit.PathExists("parameter.limit.cpu"))),
						"spec.resources.limits.cpu", cpu)
					res.SetIf(defkit.And(cpu.IsSet(), defkit.Not(defkit.PathExists("parameter.limit.cpu"))),
						"spec.resources.requests.cpu", cpu)
					tpl.Output(res)
				})

			cue := gen.GenerateFullDefinition(comp)

			// Both compound conditions should appear as separate blocks
			Expect(cue).To(ContainSubstring(`parameter["cpu"] != _|_`))
			Expect(cue).To(ContainSubstring("parameter.limit.cpu"))

			// resources should be decomposed into per-condition blocks
			// Each block should contain both limits and requests
			Expect(strings.Count(cue, "resources:")).To(BeNumerically(">=", 2))
			Expect(strings.Count(cue, "limits:")).To(BeNumerically(">=", 2))
			Expect(strings.Count(cue, "requests:")).To(BeNumerically(">=", 2))

			// The block where limit.cpu exists should use parameter.limit.cpu for limits
			Expect(cue).To(ContainSubstring("cpu: parameter.limit.cpu"))
			// The block where limit.cpu does NOT exist should use parameter.cpu for limits
			Expect(strings.Count(cue, "cpu: parameter.cpu")).To(BeNumerically(">=", 2))
		})

		It("should decompose when two sibling leaves at the same path have different condValues", func() {
			// Pattern: limits.cpu set under condition A (primary) and condition B (condValue),
			// requests.cpu set under condition A (primary) and condition B (condValue).
			// collectLeafConditions must see both A and B from condValues.
			gen := defkit.NewCUEGenerator()

			flagA := defkit.Bool("flagA")
			flagB := defkit.Bool("flagB")

			comp := defkit.NewComponent("test-condval-siblings").
				Description("Test condValues sibling decomposition").
				AutodetectWorkload().
				Params(flagA, flagB).
				Template(func(tpl *defkit.Template) {
					res := defkit.NewResource("v1", "Pod")
					res.SetIf(flagA.IsSet(), "spec.nested.child1", defkit.Lit("A1"))
					res.SetIf(flagB.IsSet(), "spec.nested.child1", defkit.Lit("B1"))
					res.SetIf(flagA.IsSet(), "spec.nested.child2", defkit.Lit("A2"))
					res.SetIf(flagB.IsSet(), "spec.nested.child2", defkit.Lit("B2"))
					tpl.Output(res)
				})

			cue := gen.GenerateFullDefinition(comp)

			// Both conditions should appear
			Expect(cue).To(ContainSubstring(`parameter["flagA"]`))
			Expect(cue).To(ContainSubstring(`parameter["flagB"]`))
			// nested should be decomposed into per-condition blocks
			Expect(strings.Count(cue, "nested:")).To(BeNumerically(">=", 2))

			// In the flagA block: child1 = "A1", child2 = "A2"
			flagAIdx := strings.Index(cue, `parameter["flagA"]`)
			Expect(flagAIdx).To(BeNumerically(">", 0))
			endA := flagAIdx + 200
			if endA > len(cue) {
				endA = len(cue)
			}
			afterFlagA := cue[flagAIdx:endA]
			Expect(afterFlagA).To(ContainSubstring(`child1: "A1"`))
			Expect(afterFlagA).To(ContainSubstring(`child2: "A2"`))

			// In the flagB block: child1 = "B1", child2 = "B2"
			flagBIdx := strings.Index(cue, `parameter["flagB"]`)
			Expect(flagBIdx).To(BeNumerically(">", 0))
			endB := flagBIdx + 200
			if endB > len(cue) {
				endB = len(cue)
			}
			afterFlagB := cue[flagBIdx:endB]
			Expect(afterFlagB).To(ContainSubstring(`child1: "B1"`))
			Expect(afterFlagB).To(ContainSubstring(`child2: "B2"`))
		})

		It("should filter condValues correctly: primary condition returns primary value, condValue condition returns condValue value", func() {
			// Verifies filterNodeByCondition returns the correct value
			// when the target condition matches a condValue rather than the primary.
			gen := defkit.NewCUEGenerator()

			modeA := defkit.String("modeA")
			modeB := defkit.String("modeB")

			comp := defkit.NewComponent("test-filter-condval").
				Description("Test filterNodeByCondition with condValues").
				AutodetectWorkload().
				Params(modeA, modeB).
				Template(func(tpl *defkit.Template) {
					res := defkit.NewResource("v1", "Pod")
					// Same leaf path, two different conditions, two different values
					res.SetIf(modeA.IsSet(), "spec.wrapper.target", defkit.Lit("value-from-A"))
					res.SetIf(modeB.IsSet(), "spec.wrapper.target", defkit.Lit("value-from-B"))
					// Second leaf to make decomposition viable
					res.SetIf(modeA.IsSet(), "spec.wrapper.other", defkit.Lit("other-A"))
					res.SetIf(modeB.IsSet(), "spec.wrapper.other", defkit.Lit("other-B"))
					tpl.Output(res)
				})

			cue := gen.GenerateFullDefinition(comp)

			// Find the modeA block and verify it has value-from-A
			modeAIdx := strings.Index(cue, `parameter["modeA"]`)
			Expect(modeAIdx).To(BeNumerically(">", 0))
			endA := modeAIdx + 250
			if endA > len(cue) {
				endA = len(cue)
			}
			afterA := cue[modeAIdx:endA]
			Expect(afterA).To(ContainSubstring(`target: "value-from-A"`))
			Expect(afterA).To(ContainSubstring(`other: "other-A"`))

			// Find the modeB block and verify it has value-from-B
			modeBIdx := strings.Index(cue, `parameter["modeB"]`)
			Expect(modeBIdx).To(BeNumerically(">", 0))
			endB := modeBIdx + 250
			if endB > len(cue) {
				endB = len(cue)
			}
			afterB := cue[modeBIdx:endB]
			Expect(afterB).To(ContainSubstring(`target: "value-from-B"`))
			Expect(afterB).To(ContainSubstring(`other: "other-B"`))
		})
	})

	Describe("Intermediate node decomposition (canDecomposeByCondition)", func() {
		It("should decompose struct into per-condition blocks when all leaves share the same condition set", func() {
			gen := defkit.NewCUEGenerator()

			cpu := defkit.String("cpu")
			memory := defkit.String("memory")

			comp := defkit.NewComponent("test-decompose").
				Description("Test condition decomposition").
				AutodetectWorkload().
				Params(cpu, memory).
				Template(func(tpl *defkit.Template) {
					res := defkit.NewResource("v1", "Pod")
					res.
						If(cpu.IsSet()).
						Set("spec.resources.limits.cpu", cpu).
						Set("spec.resources.requests.cpu", cpu).
						EndIf().
						If(memory.IsSet()).
						Set("spec.resources.limits.memory", memory).
						Set("spec.resources.requests.memory", memory).
						EndIf()
					tpl.Output(res)
				})

			cue := gen.GenerateFullDefinition(comp)
			// resources should NOT appear unconditionally
			// It should appear inside if blocks
			Expect(cue).To(ContainSubstring(`if parameter["cpu"] != _|_ {`))
			Expect(cue).To(ContainSubstring(`if parameter["memory"] != _|_ {`))

			// Each condition block should contain resources with limits and requests
			// Find the cpu block
			cpuIdx := strings.Index(cue, `if parameter["cpu"] != _|_ {`)
			Expect(cpuIdx).To(BeNumerically(">", 0))
			// After the cpu condition, resources should appear
			afterCpu := cue[cpuIdx:]
			Expect(afterCpu).To(ContainSubstring("resources:"))
			Expect(afterCpu).To(ContainSubstring("limits:"))
			Expect(afterCpu).To(ContainSubstring("requests:"))
		})

		It("should not decompose when children have unconditional values", func() {
			gen := defkit.NewCUEGenerator()

			cpu := defkit.String("cpu")

			comp := defkit.NewComponent("test-no-decompose").
				Description("Test no decomposition").
				AutodetectWorkload().
				Params(cpu).
				Template(func(tpl *defkit.Template) {
					res := defkit.NewResource("v1", "Pod")
					res.
						If(cpu.IsSet()).
						Set("spec.resources.limits.cpu", cpu).
						EndIf().
						Set("spec.resources.limits.memory", defkit.Lit("128Mi"))
					tpl.Output(res)
				})

			cue := gen.GenerateFullDefinition(comp)
			// resources should appear as a regular struct (not decomposed)
			// because it has a mix of conditional and unconditional children
			Expect(cue).To(ContainSubstring("resources:"))
			Expect(cue).To(ContainSubstring("limits:"))
			Expect(cue).To(ContainSubstring(`memory: "128Mi"`))
		})
	})

	Describe("GenerateFullDefinition", func() {
		It("should generate complete CUE definition", func() {
			comp := defkit.NewComponent("webservice").
				Description("Web service component").
				Workload("apps/v1", "Deployment").
				Params(
					defkit.String("image").Required(),
				)

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring(`webservice: {`))
			Expect(cue).To(ContainSubstring(`type: "component"`))
			Expect(cue).To(ContainSubstring(`description: "Web service component"`))
			Expect(cue).To(ContainSubstring(`apiVersion: "apps/v1"`))
			Expect(cue).To(ContainSubstring(`kind:       "Deployment"`))
			Expect(cue).To(ContainSubstring(`type: "deployments.apps"`))
			Expect(cue).To(ContainSubstring(`parameter: {`))
		})

		It("should quote component names with special characters", func() {
			comp := defkit.NewComponent("my-service").
				Description("Service with dash").
				Workload("apps/v1", "Deployment")

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring(`"my-service": {`))
		})

		It("should include status when defined", func() {
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				CustomStatus("message: \"Ready\"").
				HealthPolicy("isHealth: true")

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring("status: {"))
			Expect(cue).To(ContainSubstring("customStatus:"))
			Expect(cue).To(ContainSubstring("healthPolicy:"))
		})

		It("should infer correct workload types", func() {
			testCases := []struct {
				apiVersion   string
				kind         string
				expectedType string
			}{
				{"apps/v1", "Deployment", "deployments.apps"},
				{"apps/v1", "StatefulSet", "statefulsets.apps"},
				{"apps/v1", "DaemonSet", "daemonsets.apps"},
				{"batch/v1", "Job", "jobs.batch"},
				{"batch/v1", "CronJob", "cronjobs.batch"},
			}

			for _, tc := range testCases {
				comp := defkit.NewComponent("test").
					Workload(tc.apiVersion, tc.kind)

				cue := gen.GenerateFullDefinition(comp)
				Expect(cue).To(ContainSubstring(tc.expectedType),
					"Expected workload type %s for %s/%s", tc.expectedType, tc.apiVersion, tc.kind)
			}
		})
	})

	Describe("GenerateParameterSchema with Map parameters", func() {
		It("should generate CUE for map parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.Map("labels").Description("Labels to apply"),
					defkit.Map("annotations").Optional(),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("labels?:"))
			Expect(cue).To(ContainSubstring("annotations?:"))
			Expect(cue).To(ContainSubstring("Labels to apply"))
		})
	})

	Describe("GenerateFullDefinition with template", func() {
		It("should generate output block with resource", func() {
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(defkit.String("image")).
				Template(func(tpl *defkit.Template) {
					image := defkit.String("image")
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", defkit.VelaCtx().Name()).
							Set("spec.template.spec.containers[0].image", image),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring("output:"))
			Expect(cue).To(ContainSubstring("apiVersion:"))
			Expect(cue).To(ContainSubstring("kind:"))
		})

		It("should generate outputs block with auxiliary resources", func() {
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", defkit.VelaCtx().Name()),
					)
					tpl.Outputs("service",
						defkit.NewResource("v1", "Service").
							Set("metadata.name", defkit.VelaCtx().Name()),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring("output:"))
			Expect(cue).To(ContainSubstring("outputs:"))
			Expect(cue).To(ContainSubstring("service:"))
		})

		It("should generate conditional fields with SetIf", func() {
			cpu := defkit.String("cpu").Optional()
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(cpu).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							SetIf(cpu.IsSet(), "spec.resources.limits.cpu", cpu),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring("output:"))
			Expect(cue).To(ContainSubstring("if"))
		})
	})

	Describe("Import detection", func() {
		It("should detect strconv import from FormatInt", func() {
			port := defkit.Int("port")
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(port).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.annotations.port", defkit.StrconvFormatInt(port, 10)),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring("strconv"))
		})

		It("should detect strings import from ToLower", func() {
			name := defkit.String("name")
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(name).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", defkit.StringsToLower(name)),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring("strings"))
		})
	})

	Describe("GenerateFullDefinition with ConditionalOrFieldRef", func() {
		It("should generate if/else pattern for conditional field reference", func() {
			gen := defkit.NewCUEGenerator()

			ports := defkit.List("ports").WithFields(
				defkit.Int("port").Required(),
				defkit.String("name"),
			)
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(ports).
				Template(func(tpl *defkit.Template) {
					containerPorts := defkit.Each(ports).Map(defkit.FieldMap{
						"containerPort": defkit.FieldRef("port"),
						"name":          defkit.FieldRef("name").OrConditional(defkit.Format("port-%v", defkit.FieldRef("port"))),
					})
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.template.spec.containers[0].ports", containerPorts),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			// Should generate if/else blocks, NOT default syntax
			Expect(cue).To(ContainSubstring("if v.name != _|_"))
			Expect(cue).To(ContainSubstring("name: v.name"))
			Expect(cue).To(ContainSubstring("if v.name == _|_"))
			Expect(cue).To(ContainSubstring("strconv.FormatInt(v.port, 10)"))
		})
	})

	Describe("GenerateFullDefinition with Directive", func() {
		It("should render // +patchKey directive before field value", func() {
			gen := defkit.NewCUEGenerator()

			hostAliases := defkit.Object("hostAliases")
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "DaemonSet").
				Params(hostAliases).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "DaemonSet").
							SetIf(hostAliases.IsSet(), "spec.template.spec.hostAliases", hostAliases).
							Directive("spec.template.spec.hostAliases", "patchKey=ip"),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			// Directive should appear before the field value
			Expect(cue).To(ContainSubstring("// +patchKey=ip"))
			Expect(cue).To(ContainSubstring("hostAliases: parameter.hostAliases"))

			// Verify ordering: directive before field
			patchIdx := strings.Index(cue, "// +patchKey=ip")
			hostIdx := strings.Index(cue, "hostAliases: parameter.hostAliases")
			Expect(patchIdx).To(BeNumerically("<", hostIdx))
		})
	})

	Describe("GenerateFullDefinition with CompoundOptionalField", func() {
		It("should generate compound conditional for OptionalFieldWithCond in collection Map", func() {
			gen := defkit.NewCUEGenerator()

			exposeType := defkit.Enum("exposeType").
				Values("ClusterIP", "NodePort", "LoadBalancer").
				Default("ClusterIP")
			ports := defkit.List("ports").WithFields(
				defkit.Int("port").Required(),
				defkit.String("name"),
				defkit.Int("nodePort"),
				defkit.Bool("expose").Default(false),
			)
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(exposeType, ports).
				Template(func(tpl *defkit.Template) {
					exposePorts := defkit.Each(ports).
						Filter(defkit.FieldEquals("expose", true)).
						Map(defkit.FieldMap{
							"port":     defkit.FieldRef("port"),
							"nodePort": defkit.OptionalFieldWithCond("nodePort", defkit.Eq(exposeType, defkit.Lit("NodePort"))),
						})
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.ports", exposePorts),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			// Should generate compound conditional: if v.nodePort != _|_ if <cond> { nodePort: v.nodePort }
			Expect(cue).To(ContainSubstring("v.nodePort != _|_"))
			Expect(cue).To(ContainSubstring(`parameter.exposeType == "NodePort"`))
			Expect(cue).To(ContainSubstring("nodePort: v.nodePort"))
		})

		It("should generate simple optional conditional alongside compound conditional", func() {
			gen := defkit.NewCUEGenerator()

			exposeType := defkit.Enum("exposeType").
				Values("ClusterIP", "NodePort", "LoadBalancer").
				Default("ClusterIP")
			ports := defkit.List("ports").WithFields(
				defkit.Int("port").Required(),
				defkit.Int("nodePort"),
				defkit.String("protocol"),
				defkit.Bool("expose").Default(false),
			)
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(exposeType, ports).
				Template(func(tpl *defkit.Template) {
					exposePorts := defkit.Each(ports).
						Filter(defkit.FieldEquals("expose", true)).
						Map(defkit.FieldMap{
							"port":     defkit.FieldRef("port"),
							"nodePort": defkit.OptionalFieldWithCond("nodePort", defkit.Eq(exposeType, defkit.Lit("NodePort"))),
							"protocol": defkit.OptionalFieldRef("protocol"),
						})
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.ports", exposePorts),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			// Compound conditional for nodePort
			Expect(cue).To(ContainSubstring("v.nodePort != _|_"))
			Expect(cue).To(ContainSubstring(`parameter.exposeType == "NodePort"`))
			Expect(cue).To(ContainSubstring("nodePort: v.nodePort"))

			// Simple optional conditional for protocol
			Expect(cue).To(ContainSubstring("v.protocol != _|_"))
			Expect(cue).To(ContainSubstring("protocol: v.protocol"))
		})
	})

	Describe("GenerateFullDefinition with InlineArray", func() {
		It("should render inline array value as [{field: value}]", func() {
			gen := defkit.NewCUEGenerator()

			port := defkit.Int("port")
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "DaemonSet").
				Params(port).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "DaemonSet").
							Set("spec.template.spec.containers[0].ports", defkit.InlineArray(map[string]defkit.Value{
								"containerPort": port,
							})),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring("containerPort: parameter.port"))
		})

		It("should render inline array with multiple fields", func() {
			gen := defkit.NewCUEGenerator()

			port := defkit.Int("port")
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "DaemonSet").
				Params(port).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "DaemonSet").
							Set("spec.template.spec.containers[0].ports", defkit.InlineArray(map[string]defkit.Value{
								"containerPort": port,
								"protocol":      defkit.Lit("TCP"),
							})),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring("containerPort: parameter.port"))
			Expect(cue).To(ContainSubstring(`protocol: "TCP"`))
		})
	})
})
