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
					defkit.String("image").Description("Container image"),
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
					defkit.Int("port"),
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
					defkit.StringList("cmd").Optional().Description("Commands"),
					defkit.List("env").Optional().Description("Environment variables"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("cmd?: [...string]"))
			// List() creates an untyped array which generates [..._]
			Expect(cue).To(ContainSubstring("env?: [..._]"))
		})

		It("should generate CUE for object parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.Object("config").Optional().Description("Configuration object"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("config?: {...}"))
		})

		It("should generate CUE for string-key map parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.StringKeyMap("labels").Optional().Description("Labels"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("labels?: [string]: string"))
		})

		It("should emit ! for required parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.String("required").Required(),
					defkit.String("optional").Optional(),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("required!: string"))
			Expect(cue).To(ContainSubstring("optional?: string"))
		})

		It("should handle two-state field presence markers", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.String("accessKey").Required(), // ! — user must explicitly provide
					defkit.Int("port"),                    // no marker — bare param
					defkit.Bool("enabled"),                // no marker — bare param
					defkit.String("tag").Optional(),       // ? — explicitly optional
				)

			cue := gen.GenerateParameterSchema(comp)

			// Required fields get "!" marker
			Expect(cue).To(ContainSubstring("accessKey!: string"))
			// Bare params get no marker (no ? and no !)
			Expect(cue).To(ContainSubstring("port: int"))
			Expect(cue).NotTo(ContainSubstring("port!:"))
			Expect(cue).NotTo(ContainSubstring("port?:"))
			Expect(cue).To(ContainSubstring("enabled: bool"))
			// Optional gets ?
			Expect(cue).To(ContainSubstring("tag?: string"))
		})

		It("should emit ! for required parameters with defaults", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.String("accessKey").Required().Default("key123"),
				)

			cue := gen.GenerateParameterSchema(comp)

			// Required with default: "!" wins, default value is still in the CUE type
			Expect(cue).To(ContainSubstring(`accessKey!: *"key123" | string`))
		})

		It("should emit ! for required non-string parameters with defaults", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.Int("port").Required().Default(8080),
					defkit.Bool("enabled").Required().Default(true),
					defkit.Float("ratio").Required().Default(0.5),
					defkit.Enum("mode").Required().Default("fast").Values("fast", "slow"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring(`port!: *8080 | int`))
			Expect(cue).To(ContainSubstring(`enabled!: *true | bool`))
			Expect(cue).To(ContainSubstring(`ratio!: *0.5 | float`))
			Expect(cue).To(ContainSubstring(`mode!: *"fast" | "slow"`))
		})

		It("should keep ? for ForceOptional parameters even with defaults", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.String("normalDefault").Default("Honor").Values("Honor", "Ignore"),
					defkit.String("optionalDefault").Default("Honor").Optional().Values("Honor", "Ignore"),
				)

			cue := gen.GenerateParameterSchema(comp)

			// Normal default: no ? (field is always present)
			Expect(cue).To(ContainSubstring(`normalDefault: *"Honor" | "Ignore"`))
			Expect(cue).NotTo(ContainSubstring(`normalDefault?:`))

			// ForceOptional with default: has ? (field can be absent, defaults when present)
			Expect(cue).To(ContainSubstring(`optionalDefault?: *"Honor" | "Ignore"`))
		})

		It("should append string to enum when OpenEnum is set", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.String("verbosity").Default("info").Values("info", "debug", "warn").OpenEnum(),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring(`*"info" | "debug" | "warn" | string`))
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
					defkit.String("image").Description("Container image").Short("i"),
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
					defkit.Struct("resources").Optional().WithFields(
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
					defkit.List("ports").Optional().WithFields(
						defkit.Int("port"),
						defkit.String("name").Optional(),
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
					defkit.Struct("selector").WithFields(
						defkit.Field("matchExpressions", defkit.ParamTypeArray).Optional().
							Nested(defkit.Struct("matchExpression").WithFields(
								defkit.Field("key", defkit.ParamTypeString),
								defkit.Field("operator", defkit.ParamTypeString).Optional(),
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
							defkit.Variant("pvc").WithFields(
								defkit.Field("claimName", defkit.ParamTypeString),
							),
							defkit.Variant("emptyDir").WithFields(
								defkit.Field("medium", defkit.ParamTypeString).Default("").Values("", "Memory"),
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
					defkit.List("volumes").Optional().WithFields(
						defkit.String("name"),
						defkit.OneOf("type").Default("emptyDir").Variants(
							defkit.Variant("pvc").WithFields(
								defkit.Field("claimName", defkit.ParamTypeString),
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
							defkit.Variant("complex").WithFields(
								defkit.Field("config", defkit.ParamTypeString),
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
					defkit.String("image"),
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
					defkit.Map("labels").Optional().Description("Labels to apply"),
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

	Describe("GenerateFullDefinition with OutputsGroupIf", func() {
		It("should render a grouped if block with multiple outputs on a component", func() {
			enabled := defkit.Bool("enabled")
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(enabled).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", defkit.VelaCtx().Name()),
					)
					tpl.OutputsGroupIf(defkit.Eq(enabled, defkit.Lit(true)), func(g *defkit.OutputGroup) {
						g.Add("service", defkit.NewResource("v1", "Service").
							Set("metadata.name", defkit.VelaCtx().Name()))
						g.Add("ingress", defkit.NewResource("networking.k8s.io/v1", "Ingress").
							Set("metadata.name", defkit.VelaCtx().Name()))
					})
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring("outputs: {"))
			Expect(cue).To(ContainSubstring("if parameter.enabled == true {"))
			Expect(cue).To(ContainSubstring("service: {"))
			Expect(cue).To(ContainSubstring("ingress: {"))

			ifIdx := strings.Index(cue, "if parameter.enabled == true {")
			svcIdx := strings.Index(cue, "service: {")
			ingIdx := strings.Index(cue, "ingress: {")
			Expect(ifIdx).To(BeNumerically(">=", 0))
			Expect(ifIdx).To(BeNumerically("<", ingIdx))
			Expect(ifIdx).To(BeNumerically("<", svcIdx))
			Expect(ingIdx).To(BeNumerically("<", svcIdx))
		})

		It("should render plain Outputs before grouped outputs", func() {
			enabled := defkit.Bool("enabled")
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(enabled).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", defkit.VelaCtx().Name()),
					)
					tpl.Outputs("configmap",
						defkit.NewResource("v1", "ConfigMap").
							Set("metadata.name", defkit.VelaCtx().Name()),
					)
					tpl.OutputsGroupIf(defkit.Eq(enabled, defkit.Lit(true)), func(g *defkit.OutputGroup) {
						g.Add("service", defkit.NewResource("v1", "Service").
							Set("metadata.name", defkit.VelaCtx().Name()))
					})
				})

			cue := gen.GenerateFullDefinition(comp)

			cmIdx := strings.Index(cue, "configmap: {")
			ifIdx := strings.Index(cue, "if parameter.enabled == true {")
			svcIdx := strings.Index(cue, "service: {")
			Expect(cmIdx).To(BeNumerically(">=", 0))
			Expect(ifIdx).To(BeNumerically(">=", 0))
			Expect(svcIdx).To(BeNumerically(">=", 0))
			Expect(cmIdx).To(BeNumerically("<", ifIdx))
			Expect(ifIdx).To(BeNumerically("<", svcIdx))
		})

		It("should render multiple OutputsGroupIf blocks independently", func() {
			enabled := defkit.Bool("enabled")
			debug := defkit.Bool("debug")
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(enabled, debug).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", defkit.VelaCtx().Name()),
					)
					tpl.OutputsGroupIf(defkit.Eq(enabled, defkit.Lit(true)), func(g *defkit.OutputGroup) {
						g.Add("service", defkit.NewResource("v1", "Service").
							Set("metadata.name", defkit.VelaCtx().Name()))
					})
					tpl.OutputsGroupIf(defkit.Eq(debug, defkit.Lit(true)), func(g *defkit.OutputGroup) {
						g.Add("debug-cm", defkit.NewResource("v1", "ConfigMap").
							Set("metadata.name", defkit.VelaCtx().Name()))
					})
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring("if parameter.enabled == true {"))
			Expect(cue).To(ContainSubstring("if parameter.debug == true {"))
			Expect(cue).To(ContainSubstring("service: {"))
			Expect(cue).To(ContainSubstring("debug-cm"))
		})

		It("should render an outputs block even with only grouped outputs (no plain Outputs)", func() {
			enabled := defkit.Bool("enabled")
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(enabled).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", defkit.VelaCtx().Name()),
					)
					tpl.OutputsGroupIf(defkit.Eq(enabled, defkit.Lit(true)), func(g *defkit.OutputGroup) {
						g.Add("service", defkit.NewResource("v1", "Service").
							Set("metadata.name", defkit.VelaCtx().Name()))
						g.Add("ingress", defkit.NewResource("networking.k8s.io/v1", "Ingress").
							Set("metadata.name", defkit.VelaCtx().Name()))
					})
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring("outputs: {"))
			Expect(cue).To(ContainSubstring("if parameter.enabled == true {"))
			Expect(cue).To(ContainSubstring("service: {"))
			Expect(cue).To(ContainSubstring("ingress: {"))
		})

		It("should render grouped output names sorted alphabetically", func() {
			enabled := defkit.Bool("enabled")
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(enabled).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", defkit.VelaCtx().Name()),
					)
					tpl.OutputsGroupIf(defkit.Eq(enabled, defkit.Lit(true)), func(g *defkit.OutputGroup) {
						g.Add("z-ingress", defkit.NewResource("networking.k8s.io/v1", "Ingress").
							Set("metadata.name", defkit.VelaCtx().Name()))
						g.Add("a-svc", defkit.NewResource("v1", "Service").
							Set("metadata.name", defkit.VelaCtx().Name()))
					})
				})

			cue := gen.GenerateFullDefinition(comp)

			aIdx := strings.Index(cue, "a-svc")
			zIdx := strings.Index(cue, "z-ingress")
			Expect(aIdx).To(BeNumerically(">=", 0))
			Expect(zIdx).To(BeNumerically(">=", 0))
			Expect(aIdx).To(BeNumerically("<", zIdx))
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

		It("should detect strings import from StringParam.MinLen", func() {
			hostname := defkit.String("hostname").MinLen(1)
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(hostname).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", hostname),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(MatchRegexp(`import\s+\(\s+"strings"\s+\)`))
			Expect(cue).To(ContainSubstring("strings.MinRunes(1)"))
		})

		It("should detect strings import from StringParam.MaxLen", func() {
			hostname := defkit.String("hostname").MaxLen(63)
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(hostname).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", hostname),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(MatchRegexp(`import\s+\(\s+"strings"\s+\)`))
			Expect(cue).To(ContainSubstring("strings.MaxRunes(63)"))
		})

		It("should detect strings import when both MinLen and MaxLen are set", func() {
			hostname := defkit.String("hostname").
				Pattern("^[a-z0-9-]+$").
				MinLen(1).
				MaxLen(63)
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(hostname).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", hostname),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(MatchRegexp(`import\s+\(\s+"strings"\s+\)`))
			Expect(cue).To(ContainSubstring("strings.MinRunes(1)"))
			Expect(cue).To(ContainSubstring("strings.MaxRunes(63)"))
			// Only one import of "strings", not duplicated
			Expect(strings.Count(cue, `"strings"`)).To(Equal(1))
		})

		It("should not emit strings import when no param uses MinLen/MaxLen", func() {
			name := defkit.String("name")
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(name).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", name),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).NotTo(MatchRegexp(`import\s+\(\s+"strings"\s+\)`))
			Expect(cue).NotTo(ContainSubstring("strings.MinRunes"))
			Expect(cue).NotTo(ContainSubstring("strings.MaxRunes"))
		})

		It("should detect strings import even when MinLen param is not referenced in template", func() {
			// The param is declared but the template doesn't reference it — the
			// import must still be emitted because the parameter schema uses it.
			hostname := defkit.String("hostname").MinLen(1).MaxLen(63)
			name := defkit.String("name")
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Params(hostname, name).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", name),
					)
				})

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(MatchRegexp(`import\s+\(\s+"strings"\s+\)`))
		})
	})

	Describe("GenerateFullDefinition with ConditionalOrFieldRef", func() {
		It("should generate if/else pattern for conditional field reference", func() {
			gen := defkit.NewCUEGenerator()

			ports := defkit.List("ports").WithFields(
				defkit.Int("port"),
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
				defkit.Int("port"),
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
				defkit.Int("port"),
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

	Describe("Struct field enum generation in helper definitions", func() {
		It("should generate enum with default on a helper struct field", func() {
			rule := defkit.Struct("rule").WithFields(
				defkit.Field("strategy", defkit.ParamTypeString).
					Default("onAppUpdate").
					Values("onAppUpdate", "onAppDelete", "never"),
			)

			p := defkit.NewPolicy("test-enum-default").
				Description("Test").
				Helper("Rule", rule)

			cue := p.ToCue()

			Expect(cue).To(ContainSubstring(`strategy: *"onAppUpdate" | "onAppDelete" | "never"`))
			Expect(cue).NotTo(ContainSubstring(`strategy: *"onAppUpdate" | string`))
		})

		It("should generate enum without default on a helper struct field", func() {
			rule := defkit.Struct("rule").WithFields(
				defkit.Field("propagation", defkit.ParamTypeString).
					Values("orphan", "cascading").
					Optional(),
			)

			p := defkit.NewPolicy("test-enum-no-default").
				Description("Test").
				Helper("Rule", rule)

			cue := p.ToCue()

			Expect(cue).To(ContainSubstring(`propagation?: "orphan" | "cascading"`))
			Expect(cue).NotTo(ContainSubstring("propagation?: string"))
		})

		It("should generate enum without default on a helper struct field", func() {
			rule := defkit.Struct("rule").WithFields(
				defkit.Field("mode", defkit.ParamTypeString).
					Values("strict", "permissive"),
			)

			p := defkit.NewPolicy("test-enum-mandatory").
				Description("Test").
				Helper("Rule", rule)

			cue := p.ToCue()

			Expect(cue).To(ContainSubstring(`mode: "strict" | "permissive"`))
			Expect(cue).NotTo(ContainSubstring("mode?: "))
			Expect(cue).NotTo(ContainSubstring("mode: string"))
		})

		It("should generate enum without default on a parameter struct field", func() {
			comp := defkit.NewComponent("test-param-enum").
				Workload("v1", "Pod").
				Params(
					defkit.Struct("config").WithFields(
						defkit.Field("level", defkit.ParamTypeString).
							Values("low", "medium", "high").
							Optional(),
						defkit.Field("mode", defkit.ParamTypeString).
							Values("fast", "safe"),
					),
				).
				Template(func(tpl *defkit.Template) {
					tpl.Output(defkit.NewResource("v1", "Pod"))
				})

			cue := defkit.NewCUEGenerator().GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring(`level?: "low" | "medium" | "high"`))
			Expect(cue).To(ContainSubstring(`mode: "fast" | "safe"`))
			Expect(cue).NotTo(ContainSubstring("level?: string"))
			Expect(cue).NotTo(ContainSubstring("mode: string"))
			Expect(cue).NotTo(ContainSubstring("mode?: "))
		})

		It("should handle mixed enum fields: with default, without default, and plain string", func() {
			rule := defkit.Struct("rule").WithFields(
				defkit.Field("strategy", defkit.ParamTypeString).
					Default("always").
					Values("always", "never", "on-failure"),
				defkit.Field("propagation", defkit.ParamTypeString).
					Values("orphan", "cascading").
					Optional(),
				defkit.Field("name", defkit.ParamTypeString).
					Optional(),
			)

			p := defkit.NewPolicy("test-mixed").
				Description("Test").
				Helper("Rule", rule)

			cue := p.ToCue()

			// Enum with default
			Expect(cue).To(ContainSubstring(`strategy: *"always" | "never" | "on-failure"`))
			// Enum without default (optional)
			Expect(cue).To(ContainSubstring(`propagation?: "orphan" | "cascading"`))
			// Plain string (optional)
			Expect(cue).To(ContainSubstring("name?: string"))
		})
	})

	Describe("GenerateParameterSchema with ClosedUnion parameters", func() {
		It("should generate close() disjunction with simple fields", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.ClosedUnion("url").
						Description("Specify the url").
						Options(
							defkit.ClosedStruct().WithFields(
								defkit.Field("value", defkit.ParamTypeString),
							),
							defkit.ClosedStruct().WithFields(
								defkit.Field("ref", defkit.ParamTypeString),
							),
						),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("// +usage=Specify the url"))
			Expect(cue).To(ContainSubstring("url: close({"))
			Expect(cue).To(ContainSubstring("value: string"))
			Expect(cue).To(ContainSubstring("}) | close({"))
			Expect(cue).To(ContainSubstring("ref: string"))
		})

		It("should generate close() disjunction with nested structs", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.ClosedUnion("url").
						Options(
							defkit.ClosedStruct().WithFields(
								defkit.Field("value", defkit.ParamTypeString),
							),
							defkit.ClosedStruct().WithFields(
								defkit.Field("secretRef", defkit.ParamTypeStruct).Nested(
									defkit.Struct("secretRef").WithFields(
										defkit.Field("name", defkit.ParamTypeString).Description("name of the secret"),
										defkit.Field("key", defkit.ParamTypeString).Description("key in the secret"),
									),
								),
							),
						),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("url: close({"))
			Expect(cue).To(ContainSubstring("value: string"))
			Expect(cue).To(ContainSubstring("}) | close({"))
			Expect(cue).To(ContainSubstring("secretRef: {"))
			Expect(cue).To(ContainSubstring("// +usage=name of the secret"))
			Expect(cue).To(ContainSubstring("name: string"))
			Expect(cue).To(ContainSubstring("// +usage=key in the secret"))
			Expect(cue).To(ContainSubstring("key: string"))
		})

		It("should generate optional closed union with ?", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.ClosedUnion("source").
						Optional().
						Options(
							defkit.ClosedStruct().WithFields(
								defkit.Field("hcl", defkit.ParamTypeString),
							),
						),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("source?:"))
		})

		It("should handle close() disjunction field ordering", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.ClosedUnion("url").Options(
						defkit.ClosedStruct().WithFields(
							defkit.Field("value", defkit.ParamTypeString),
						),
						defkit.ClosedStruct().WithFields(
							defkit.Field("secretRef", defkit.ParamTypeStruct),
						),
					),
				)

			cue := gen.GenerateParameterSchema(comp)

			// Verify ordering: first option before second
			valueIdx := strings.Index(cue, "value: string")
			secretIdx := strings.Index(cue, "secretRef:")
			Expect(valueIdx).To(BeNumerically(">=", 0), "expected 'value: string' to be present")
			Expect(secretIdx).To(BeNumerically(">=", 0), "expected 'secretRef:' to be present")
			Expect(valueIdx).To(BeNumerically("<", secretIdx))
		})

		It("should handle empty options gracefully", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.ClosedUnion("empty"),
				)

			cue := gen.GenerateParameterSchema(comp)

			// Empty options should produce fallback
			Expect(cue).To(ContainSubstring("empty: _"))
		})

		It("should generate ClosedUnion in helper definition", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.String("name"),
				).
				Helper("URLConfig", defkit.ClosedUnion("urlConfig").
					Options(
						defkit.ClosedStruct().WithFields(
							defkit.Field("value", defkit.ParamTypeString),
						),
						defkit.ClosedStruct().WithFields(
							defkit.Field("secretRef", defkit.ParamTypeString),
						),
					),
				)

			cue := comp.ToCue()

			Expect(cue).To(ContainSubstring("#URLConfig:"))
			Expect(cue).To(ContainSubstring("close({"))
			Expect(cue).To(ContainSubstring("value: string"))
			Expect(cue).To(ContainSubstring("}) | close({"))
			Expect(cue).To(ContainSubstring("secretRef: string"))
		})
	})

	Describe("ForEachGuarded inner braces", func() {
		It("should wrap each element in inner braces", func() {
			hasStorage := defkit.PathExists("parameter.storage")
			ws := defkit.NewWorkflowStep("test").
				Params(
					defkit.Array("storage").WithFields(
						defkit.String("name").Required(),
						defkit.String("path").Required(),
					),
				).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					arr := defkit.NewArray().
						ForEachGuarded(
							hasStorage,
							defkit.ParamRef("storage"),
							defkit.NewArrayElement().
								Set("name", defkit.Reference("m.name")).
								Set("path", defkit.Reference("m.path")),
						)
					tpl.Set("items", arr)
				})

			cue := ws.ToCue()

			// Should have inner braces around each element
			Expect(cue).To(ContainSubstring("for m in"))
			Expect(cue).To(MatchRegexp(`for m in .+ \{\n[^\n]*\{`))
		})

		It("should wrap elements with conditional fields in inner braces", func() {
			hasStorage := defkit.PathExists("parameter.storage")
			ws := defkit.NewWorkflowStep("test").
				Params(
					defkit.Array("storage").WithFields(
						defkit.String("name").Required(),
						defkit.String("subPath"),
					),
				).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					arr := defkit.NewArray().
						ForEachGuarded(
							hasStorage,
							defkit.ParamRef("storage"),
							defkit.NewArrayElement().
								Set("name", defkit.Reference("m.name")).
								SetIf(defkit.PathExists("m.subPath"), "subPath", defkit.Reference("m.subPath")),
						)
					tpl.Set("mounts", arr)
				})

			cue := ws.ToCue()

			// Inner braces should contain both the field and the conditional
			Expect(cue).To(ContainSubstring("name: m.name"))
			Expect(cue).To(ContainSubstring("if m.subPath != _|_"))
		})
	})

	Describe("Dedupe from Reference source", func() {
		It("should generate dedup pattern when source is a Reference", func() {
			ws := defkit.NewWorkflowStep("test").
				Params(defkit.String("name").Required()).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.Set("deduped", defkit.From(defkit.Reference("volumesList")).Dedupe("name"))
				})

			cue := ws.ToCue()

			// Should generate the full dedup pattern, not simple [for v in ...]
			Expect(cue).To(ContainSubstring("for val in ["))
			Expect(cue).To(ContainSubstring("for i, vi in volumesList"))
			Expect(cue).To(ContainSubstring("for j, vj in volumesList if j < i && vi.name == vj.name"))
			Expect(cue).To(ContainSubstring("_ignore: true"))
			Expect(cue).To(ContainSubstring("if val._ignore == _|_"))
			Expect(cue).NotTo(ContainSubstring("[for v in volumesList { v }]"))
		})

		It("should generate dedup pattern with different key field", func() {
			ws := defkit.NewWorkflowStep("test").
				Params(defkit.String("name").Required()).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.Set("unique", defkit.From(defkit.Reference("items")).Dedupe("id"))
				})

			cue := ws.ToCue()

			Expect(cue).To(ContainSubstring("vi.id == vj.id"))
			Expect(cue).To(ContainSubstring("for i, vi in items"))
		})
	})

	Describe("TemplateBody with no params", func() {
		It("should skip parameter block when TemplateBody is set and no params exist", func() {
			ws := defkit.NewWorkflowStep("test").
				Description("test step").
				TemplateBody("nop: {}")

			cue := ws.ToCue()

			Expect(cue).To(ContainSubstring("nop: {}"))
			Expect(cue).NotTo(ContainSubstring("parameter:"))
		})

		It("should still emit parameter block when TemplateBody is set but params exist", func() {
			ws := defkit.NewWorkflowStep("test").
				Description("test step").
				Params(defkit.String("name")).
				TemplateBody("nop: {}")

			cue := ws.ToCue()

			Expect(cue).To(ContainSubstring("nop: {}"))
			Expect(cue).To(ContainSubstring("parameter:"))
			Expect(cue).To(ContainSubstring("name: string"))
		})

		It("should emit parameter block when no TemplateBody and no params", func() {
			ws := defkit.NewWorkflowStep("test").
				Description("test step")

			cue := ws.ToCue()

			// Default behavior: empty parameter block is emitted
			Expect(cue).To(ContainSubstring("parameter:"))
		})
	})

	Describe("Fluent Builder API Validator Patterns CUE Generation", func() {
		var gen *defkit.CUEGenerator

		BeforeEach(func() {
			gen = defkit.NewCUEGenerator()
		})

		Context("OmitWorkloadType CUE Generation", func() {
			It("should suppress workload type field when omitted", func() {
				comp := defkit.NewComponent("test").
					Workload("apps/v1", "Deployment").
					OmitWorkloadType().
					Template(func(tpl *defkit.Template) {
						tpl.Output(defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", defkit.Lit("test")))
					})

				cue := comp.ToCue()
				Expect(cue).To(ContainSubstring(`kind:       "Deployment"`))
				Expect(cue).To(ContainSubstring("workload:"))
				Expect(cue).NotTo(MatchRegexp(`type:\s+"deployments\.apps"`))
			})

			It("should include workload type when not omitted", func() {
				comp := defkit.NewComponent("test").
					Workload("apps/v1", "Deployment").
					Template(func(tpl *defkit.Template) {
						tpl.Output(defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", defkit.Lit("test")))
					})

				cue := comp.ToCue()
				Expect(cue).To(ContainSubstring(`type: "deployments.apps"`))
			})
		})

		Context("RawHeaderBlock in Component Template", func() {
			It("should emit raw header block before output", func() {
				comp := defkit.NewComponent("test").
					Workload("v1", "PersistentVolumeClaim").
					Template(func(tpl *defkit.Template) {
						tpl.SetRawHeaderBlock(`let _claimName = parameter.claimName + "-pvc"`)
						tpl.Output(defkit.NewResource("v1", "PersistentVolumeClaim").
							Set("metadata.name", defkit.Reference("_claimName")))
					})

				cue := gen.GenerateTemplate(comp)
				Expect(cue).To(ContainSubstring(`let _claimName = parameter.claimName + "-pvc"`))
				Expect(cue).To(ContainSubstring("_claimName"))
			})

			It("should emit multiline raw header block", func() {
				comp := defkit.NewComponent("test").
					Workload("v1", "ConfigMap").
					Template(func(tpl *defkit.Template) {
						tpl.SetRawHeaderBlock("let _a = parameter.a\nlet _b = parameter.b")
						tpl.Output(defkit.NewResource("v1", "ConfigMap").
							Set("metadata.name", defkit.Lit("test")))
					})

				cue := gen.GenerateTemplate(comp)
				Expect(cue).To(ContainSubstring("let _a = parameter.a"))
				Expect(cue).To(ContainSubstring("let _b = parameter.b"))
			})

			It("should not emit header block when empty", func() {
				comp := defkit.NewComponent("test").
					Workload("v1", "ConfigMap").
					Template(func(tpl *defkit.Template) {
						tpl.Output(defkit.NewResource("v1", "ConfigMap").
							Set("metadata.name", defkit.Lit("test")))
					})

				cue := gen.GenerateTemplate(comp)
				Expect(cue).NotTo(ContainSubstring("let _"))
			})
		})

		Context("ArrayParam NotEmpty Elements CUE Generation", func() {
			It("should generate [...(string & !=\"\")] for NotEmpty string arrays", func() {
				p := defkit.StringList("tags").NotEmpty()
				comp := defkit.NewComponent("test").Params(p)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring(`[...(string & !="")]`))
			})

			It("should generate normal [...string] without NotEmpty", func() {
				p := defkit.StringList("tags")
				comp := defkit.NewComponent("test").Params(p)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring(`[...string]`))
				Expect(cue).NotTo(ContainSubstring(`!=""`))
			})
		})

		Context("ArrayParam with Schema and Default CUE Generation", func() {
			It("should generate array with OfEnum and default value", func() {
				p := defkit.Array("methods").
					OfEnum("GET", "POST", "DELETE").
					Default([]any{"GET"})

				comp := defkit.NewComponent("test").Params(p)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring(`["GET"]`))
				Expect(cue).To(ContainSubstring(`"GET" | "POST" | "DELETE"`))
			})

			It("should generate array with OfEnum and nil default as empty array", func() {
				p := defkit.Array("methods").
					OfEnum("GET", "POST").
					Default(nil)

				comp := defkit.NewComponent("test").Params(p)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring("*[]"))
			})

			It("should generate array with OfEnum and multi-value default", func() {
				p := defkit.Array("methods").
					OfEnum("GET", "POST", "DELETE").
					Default([]any{"GET", "POST"})

				comp := defkit.NewComponent("test").Params(p)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring(`["GET", "POST"]`))
			})
		})

		Context("MapParam Closed CUE Generation", func() {
			It("should emit close({...}) for closed struct", func() {
				p := defkit.Object("governance").Closed().WithFields(
					defkit.String("tenantName"),
					defkit.String("departmentCode"),
				)
				comp := defkit.NewComponent("test").Params(p)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring("close({"))
				Expect(cue).To(ContainSubstring("})"))
				Expect(cue).To(ContainSubstring("tenantName: string"))
			})

			It("should not emit close({}) for non-closed struct", func() {
				p := defkit.Object("config").WithFields(
					defkit.String("name"),
				)
				comp := defkit.NewComponent("test").Params(p)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).NotTo(ContainSubstring("close({"))
				Expect(cue).To(ContainSubstring("config: {"))
			})
		})

		Context("MapParam with Validators CUE Generation", func() {
			It("should emit validators inside struct", func() {
				v := defkit.Validate("name required").
					WithName("_validateName").
					FailWhen(defkit.LocalField("name").Eq(""))

				p := defkit.Object("governance").WithFields(
					defkit.String("name"),
				).Validators(v)

				comp := defkit.NewComponent("test").Params(p)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring("governance: {"))
				Expect(cue).To(ContainSubstring("_validateName:"))
				Expect(cue).To(ContainSubstring(`"name required": true`))
			})
		})

		Context("MapParam with ConditionalFields CUE Generation", func() {
			It("should emit conditional fields inside struct", func() {
				flag := defkit.Bool("flag").Default(false)
				p := defkit.Object("config").Optional().ConditionalFields(
					defkit.WhenParam(flag.Eq(true)).Params(
						defkit.String("secret").Required(),
					),
					defkit.WhenParam(flag.Eq(false)).Params(
						defkit.String("secret").Optional(),
					),
				)

				comp := defkit.NewComponent("test").Params(flag, p)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring("config?: {"))
				Expect(cue).To(ContainSubstring("if parameter.flag == true"))
				Expect(cue).To(ContainSubstring("if parameter.flag == false"))
			})

			It("should emit validators inside conditional field branches", func() {
				flag := defkit.Bool("flag").Default(false)
				v := defkit.Validate("check").WithName("_v").
					FailWhen(defkit.LocalField("secret").Eq(""))

				p := defkit.Object("config").Optional().ConditionalFields(
					defkit.WhenParam(flag.Eq(true)).
						Params(defkit.String("secret").Required()).
						Validators(v),
				)

				comp := defkit.NewComponent("test").Params(flag, p)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring("_v:"))
			})
		})

		Context("Closed MapParam with Validators and ConditionalFields Combined", func() {
			It("should combine all features", func() {
				flag := defkit.Bool("flag").Default(false)
				v := defkit.Validate("name required").
					WithName("_validateName").
					FailWhen(defkit.LocalField("name").Eq(""))

				p := defkit.Object("governance").Closed().
					WithFields(
						defkit.String("name").NotEmpty(),
					).
					Validators(v).
					ConditionalFields(
						defkit.WhenParam(flag.Eq(true)).Params(
							defkit.String("extra").Required(),
						),
					)

				comp := defkit.NewComponent("test").Params(flag, p)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring("close({"))
				Expect(cue).To(ContainSubstring(`!=""`))
				Expect(cue).To(ContainSubstring("_validateName:"))
				Expect(cue).To(ContainSubstring("if parameter.flag == true"))
				Expect(cue).To(ContainSubstring("})"))
			})
		})

		Context("RegexMatch CUE Generation", func() {
			It("should generate regex match for StringParam.Matches", func() {
				p := defkit.String("name")
				comp := defkit.NewComponent("test").
					Params(p).
					Workload("v1", "ConfigMap").
					Template(func(tpl *defkit.Template) {
						tpl.Output(defkit.NewResource("v1", "ConfigMap").
							SetIf(p.Matches("^prod-"), "data.env", defkit.Lit("production")))
					})

				cue := gen.GenerateTemplate(comp)
				Expect(cue).To(ContainSubstring(`parameter.name =~ "^prod-"`))
			})

			It("should generate regex match for LocalFieldRef.Matches in validator", func() {
				v := defkit.Validate("bad").
					WithName("_v").
					FailWhen(defkit.LocalField("host").Matches(`\.internal$`))

				comp := defkit.NewComponent("test").Validators(v)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring(`host =~ "\\.internal$"`))
			})
		})

		Context("LocalFieldRef NotSet CUE Generation", func() {
			It("should generate == _|_ for LocalFieldRef.NotSet", func() {
				v := defkit.Validate("role required").
					WithName("_v").
					FailWhen(defkit.LocalField("role").NotSet())

				comp := defkit.NewComponent("test").Validators(v)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring("role == _|_"))
			})
		})

		Context("LocalFieldRef LenGt CUE Generation", func() {
			It("should generate len(field) > n", func() {
				v := defkit.Validate("too many").
					WithName("_v").
					FailWhen(defkit.LocalField("items").LenGt(10))

				comp := defkit.NewComponent("test").Validators(v)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring("len(items) > 10"))
			})
		})

		Context("LenOfExpr CUE Generation", func() {
			It("should generate len(expr) > n for Gt", func() {
				v := defkit.Validate("name too long").
					WithName("_v").
					FailWhen(defkit.LenOf(defkit.LocalField("name")).Gt(63))

				comp := defkit.NewComponent("test").Validators(v)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring("len(name) > 63"))
			})

			It("should generate len(expr) >= n for Gte", func() {
				v := defkit.Validate("check").
					WithName("_v").
					FailWhen(defkit.LenOf(defkit.LocalField("data")).Gte(100))

				comp := defkit.NewComponent("test").Validators(v)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring("len(data) >= 100"))
			})

			It("should generate len(expr) == n for Eq", func() {
				v := defkit.Validate("check").
					WithName("_v").
					FailWhen(defkit.LenOf(defkit.LocalField("code")).Eq(3))

				comp := defkit.NewComponent("test").Validators(v)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring("len(code) == 3"))
			})
		})

		Context("TimeParse CUE Generation", func() {
			It("should generate time.Parse expressions", func() {
				v := defkit.Validate("start before end").
					WithName("_v").
					FailWhen(defkit.TimeParse("2006-01-02T15:04:05Z", defkit.LocalField("start")).
						Gte(defkit.TimeParse("2006-01-02T15:04:05Z", defkit.LocalField("end"))))

				comp := defkit.NewComponent("test").Validators(v)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring(`time.Parse("2006-01-02T15:04:05Z", start)`))
				Expect(cue).To(ContainSubstring(`time.Parse("2006-01-02T15:04:05Z", end)`))
			})
		})

		Context("RawCUECondition CUE Generation", func() {
			It("should emit raw expression verbatim", func() {
				v := defkit.Validate("check").
					WithName("_v").
					FailWhen(defkit.CUEExpr(`len("prefix-"+parameter.name) > 63`))

				comp := defkit.NewComponent("test").Validators(v)
				cue := gen.GenerateParameterSchema(comp)
				Expect(cue).To(ContainSubstring(`len("prefix-"+parameter.name) > 63`))
			})
		})

		Context("ConditionalStructOp CUE Generation", func() {
			It("should generate conditional struct in template output", func() {
				replConfig := defkit.Object("replicationConfiguration").Optional()

				comp := defkit.NewComponent("test").
					Params(replConfig).
					Workload("apps/v1", "Deployment").
					Template(func(tpl *defkit.Template) {
						tpl.Output(defkit.NewResource("v1", "ConfigMap").
							Set("metadata.name", defkit.Lit("test")).
							ConditionalStruct(replConfig.IsSet(), "spec.replication", func(b *defkit.OutputStructBuilder) {
								b.Set("role", defkit.Reference("parameter.replicationConfiguration.role"))
								b.SetIf(replConfig.IsSet(), "enabled", defkit.Lit(true))
							}))
					})

				cue := gen.GenerateTemplate(comp)
				Expect(cue).To(ContainSubstring(`if parameter["replicationConfiguration"] != _|_`))
				Expect(cue).To(ContainSubstring("replication:"))
				Expect(cue).To(ContainSubstring("role: parameter.replicationConfiguration.role"))
			})

			It("should generate nested path correctly", func() {
				config := defkit.Object("config").Optional()

				comp := defkit.NewComponent("test").
					Params(config).
					Workload("v1", "ConfigMap").
					Template(func(tpl *defkit.Template) {
						tpl.Output(defkit.NewResource("v1", "ConfigMap").
							Set("metadata.name", defkit.Lit("test")).
							ConditionalStruct(config.IsSet(), "spec.deep.nested.path", func(b *defkit.OutputStructBuilder) {
								b.Set("key", defkit.Reference("parameter.config.key"))
							}))
					})

				cue := gen.GenerateTemplate(comp)
				Expect(cue).To(ContainSubstring("deep:"))
				Expect(cue).To(ContainSubstring("nested:"))
				Expect(cue).To(ContainSubstring("path:"))
			})
		})
	})

	Describe("Builtin WithDirectFields", func() {
		It("should render fields directly without $params wrapper", func() {
			ws := defkit.NewWorkflowStep("test").
				Params(defkit.String("env").Required()).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.Builtin("app", "op.#ShareCloudResource").
						WithDirectFields().
						WithParams(map[string]defkit.Value{
							"env":       defkit.Reference("parameter.env"),
							"namespace": defkit.Reference("context.namespace"),
						}).
						Build()
				})

			cue := ws.ToCue()

			Expect(cue).To(ContainSubstring("app: op.#ShareCloudResource & {"))
			Expect(cue).To(ContainSubstring("env: parameter.env"))
			Expect(cue).To(ContainSubstring("namespace: context.namespace"))
			Expect(cue).NotTo(ContainSubstring("$params:"))
		})

		It("should still use $params when WithDirectFields is not called", func() {
			ws := defkit.NewWorkflowStep("test").
				Params(defkit.String("name").Required()).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.Builtin("deploy", "kube.#Apply").
						WithParams(map[string]defkit.Value{
							"value": defkit.Reference("object"),
						}).
						Build()
				})

			cue := ws.ToCue()

			Expect(cue).To(ContainSubstring("deploy: kube.#Apply & {"))
			Expect(cue).To(ContainSubstring("$params:"))
		})
	})

	// --- OneOf with Default --------------------------------------------------
	//
	// Background: when a OneOfParam has both Optional() and Default(), the
	// generated discriminator block uses sibling-scope `if name == "..."`
	// references that fail CUE strict mode if the field is marked `?`.
	// Default makes the value concrete; the `?` marker must be dropped.
	Context("OneOf with Default", func() {
		It("should drop the ? marker when a default is set", func() {
			vol := defkit.OneOf("volume").Optional().Default("emptyDir").Variants(
				defkit.Variant("emptyDir").WithFields(
					defkit.Field("medium", defkit.ParamTypeString).Optional(),
				),
				defkit.Variant("configMap").WithFields(
					defkit.Field("name", defkit.ParamTypeString).Required(),
				),
			)
			schema := defkit.NewCUEGenerator().GenerateParameterSchema(
				defkit.NewComponent("c").Params(vol))
			Expect(schema).To(ContainSubstring(`volume: *"emptyDir" | "configMap"`))
			Expect(schema).NotTo(ContainSubstring(`volume?:`))
		})

		It("should keep the ? marker when no default is set", func() {
			vol := defkit.OneOf("volume").Optional().Variants(
				defkit.Variant("a").WithFields(defkit.Field("x", defkit.ParamTypeString).Required()),
				defkit.Variant("b").WithFields(defkit.Field("y", defkit.ParamTypeString).Required()),
			)
			schema := defkit.NewCUEGenerator().GenerateParameterSchema(
				defkit.NewComponent("c").Params(vol))
			Expect(schema).To(ContainSubstring(`volume?:`))
		})
	})

	// --- Auto-import for ArrayParam list constraints ------------------------
	//
	// Background: ArrayParam.MinItems/MaxItems emit list.MinItems(N) /
	// list.MaxItems(N), which require the CUE "list" stdlib import. The
	// auto-import scanner picks this up via ArrayParam.RequiredImports.
	Context("Auto-import for Array list constraints", func() {
		It("should add the list import when Array.MinItems is set", func() {
			ports := defkit.IntList("ports").Optional().MinItems(1)
			cue := defkit.NewComponent("c").
				Workload("v1", "ConfigMap").
				Params(ports).
				Template(func(tpl *defkit.Template) {}).
				ToCue()
			Expect(cue).To(ContainSubstring(`"list"`))
			Expect(cue).To(ContainSubstring(`list.MinItems(1)`))
		})

		It("should add the list import when Array.MaxItems is set", func() {
			ports := defkit.IntList("ports").Optional().MaxItems(10)
			cue := defkit.NewComponent("c").
				Workload("v1", "ConfigMap").
				Params(ports).
				Template(func(tpl *defkit.Template) {}).
				ToCue()
			Expect(cue).To(ContainSubstring(`"list"`))
			Expect(cue).To(ContainSubstring(`list.MaxItems(10)`))
		})

		It("should NOT add the list import for a plain Array param", func() {
			args := defkit.StringList("args").Optional()
			cue := defkit.NewComponent("c").
				Workload("v1", "ConfigMap").
				Params(args).
				Template(func(tpl *defkit.Template) {}).
				ToCue()
			Expect(cue).NotTo(ContainSubstring(`"list"`))
		})

		It("should emit the list import only once when both MinItems and MaxItems are set", func() {
			ports := defkit.IntList("ports").Optional().MinItems(1).MaxItems(10)
			cue := defkit.NewComponent("c").
				Workload("v1", "ConfigMap").
				Params(ports).
				Template(func(tpl *defkit.Template) {}).
				ToCue()
			Expect(strings.Count(cue, `"list"`)).To(Equal(1))
		})

		It("should add the list import when Array.Contains() is used", func() {
			tags := defkit.StringList("tags").Optional()
			cue := defkit.NewComponent("c").
				Workload("v1", "ConfigMap").
				Params(tags).
				Template(func(tpl *defkit.Template) {
					tpl.Output(defkit.NewResource("v1", "ConfigMap").
						SetIf(tags.Contains("gpu"), "data.gpu", defkit.Lit("true")))
				}).
				ToCue()
			Expect(cue).To(ContainSubstring(`"list"`))
			Expect(cue).To(ContainSubstring(`list.Contains`))
		})
	})

	// --- Optional collection rendering (end-to-end) -------------------------
	//
	// Background: in CUE strict mode (and the KubeVela template pipeline
	// specifically), references to optional fields like `parameter.X` (dot)
	// or `len(parameter.X)` trip "cannot reference optional field". The
	// rendering must use the bracket-existence pattern `parameter["X"] != _|_`
	// — the same form every built-in KubeVela component (cron-task.cue,
	// daemon.cue, helmchart.cue) uses.
	Context("Optional collection rendering", func() {
		It("should render IsNotEmpty() on optional Array as bracket existence (no len, no union)", func() {
			args := defkit.StringList("args").Optional()
			cue := defkit.NewComponent("c").
				Workload("v1", "ConfigMap").
				Params(args).
				Template(func(tpl *defkit.Template) {
					tpl.Output(defkit.NewResource("v1", "ConfigMap").
						SetIf(args.IsNotEmpty(), "data.x", defkit.Lit("y")))
				}).
				ToCue()
			Expect(cue).To(ContainSubstring(`if parameter["args"] != _|_`))
			Expect(cue).NotTo(ContainSubstring(`len(parameter.args)`))
			Expect(cue).NotTo(ContainSubstring(`parameter.args | []`))
		})

		It("should render IsEmpty() on optional Array as bracket-existence inverse", func() {
			args := defkit.StringList("args").Optional()
			cue := defkit.NewComponent("c").
				Workload("v1", "ConfigMap").
				Params(args).
				Template(func(tpl *defkit.Template) {
					tpl.Output(defkit.NewResource("v1", "ConfigMap").
						SetIf(args.IsEmpty(), "data.empty", defkit.Lit("yes")))
				}).
				ToCue()
			Expect(cue).To(ContainSubstring(`if parameter["args"] == _|_`))
		})

		It("should render Map.HasKey() with the daemon.cue two-clause guard", func() {
			cfg := defkit.Map("config").Of(defkit.ParamTypeString).Optional()
			schema := defkit.NewCUEGenerator().GenerateParameterSchema(
				defkit.NewComponent("c").Params(cfg).
					Validators(
						defkit.Validate("debug must not be set").
							WithName("_v").
							FailWhen(cfg.HasKey("debug")),
					))
			Expect(schema).To(ContainSubstring(`parameter["config"] != _|_`))
			Expect(schema).To(ContainSubstring(`parameter["config"].debug != _|_`))
			Expect(schema).NotTo(ContainSubstring(`parameter.config.debug != _|_`))
		})

		It("should render a multi-collection ComponentDefinition without dot-references to optional fields", func() {
			image := defkit.String("image").Required()
			args := defkit.StringList("args").Optional()
			ports := defkit.IntList("ports").Optional().MinItems(1).MaxItems(10)
			labels := defkit.StringKeyMap("labels").Optional()
			anns := defkit.Map("annotations").Of(defkit.ParamTypeString).Optional()
			vol := defkit.OneOf("volume").Optional().Default("emptyDir").Variants(
				defkit.Variant("emptyDir").WithFields(
					defkit.Field("medium", defkit.ParamTypeString).Optional(),
				),
				defkit.Variant("configMap").WithFields(
					defkit.Field("name", defkit.ParamTypeString).Required(),
				),
			)

			c := defkit.NewComponent("collection-showcase").
				Workload("apps/v1", "Deployment").
				PodSpecPath("spec.template.spec").
				Params(image, args, ports, labels, anns, vol).
				Template(func(tpl *defkit.Template) {
					vela := defkit.VelaCtx()
					tpl.Output(defkit.NewResource("apps/v1", "Deployment").
						Set("metadata.name", vela.Name()).
						Set("spec.template.spec.containers[0].image", image).
						SetIf(labels.IsNotEmpty(), "metadata.labels", labels).
						SetIf(anns.IsNotEmpty(), "metadata.annotations", anns).
						SetIf(args.IsNotEmpty(), "spec.template.spec.containers[0].args", args).
						SetIf(ports.IsNotEmpty(), "metadata.annotations[showcase/ports-set]", defkit.Lit("true")))
				})

			cue := c.ToCue()

			// 1. Schema-level: list import present, MinItems/MaxItems intact.
			Expect(cue).To(ContainSubstring(`"list"`))
			Expect(cue).To(ContainSubstring(`list.MinItems(1) & list.MaxItems(10)`))

			// 2. Every optional-collection guard uses bracket existence.
			Expect(cue).To(ContainSubstring(`if parameter["labels"] != _|_`))
			Expect(cue).To(ContainSubstring(`if parameter["annotations"] != _|_`))
			Expect(cue).To(ContainSubstring(`if parameter["args"] != _|_`))
			Expect(cue).To(ContainSubstring(`if parameter["ports"] != _|_`))

			// 3. None of the strict-mode-failing forms appear.
			Expect(cue).NotTo(ContainSubstring(`len(parameter.labels)`))
			Expect(cue).NotTo(ContainSubstring(`len(parameter.args)`))
			Expect(cue).NotTo(ContainSubstring(`parameter.labels | {}`))
			Expect(cue).NotTo(ContainSubstring(`parameter.args | []`))

			// 4. OneOf with Default: discriminator field is concrete, not optional.
			Expect(cue).To(ContainSubstring(`volume: *"emptyDir" | "configMap"`))
			Expect(cue).NotTo(ContainSubstring(`volume?:`))

			// 5. Inside a guarded if-block, dot syntax for the value reference
			//    is still emitted (safe because the guard establishes existence).
			Expect(cue).To(ContainSubstring(`labels: parameter.labels`))
			Expect(cue).To(ContainSubstring(`args: parameter.args`))
		})
	})
})
