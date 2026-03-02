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

var _ = Describe("TraitDefinition", func() {

	Context("Basic Builder Methods", func() {
		It("should set name, type, and description correctly", func() {
			trait := defkit.NewTrait("scaler").
				Description("Manually scale K8s pod for your workload.").
				AppliesTo("deployments.apps", "statefulsets.apps").
				PodDisruptive(false)

			Expect(trait.DefName()).To(Equal("scaler"))
			Expect(trait.GetName()).To(Equal("scaler"))
			Expect(trait.DefType()).To(Equal(defkit.DefinitionTypeTrait))
			Expect(trait.GetDescription()).To(Equal("Manually scale K8s pod for your workload."))
			Expect(trait.GetAppliesToWorkloads()).To(Equal([]string{"deployments.apps", "statefulsets.apps"}))
			Expect(trait.IsPodDisruptive()).To(BeFalse())
		})

		It("should set ConflictsWith correctly", func() {
			trait := defkit.NewTrait("hpa").
				Description("HPA scaler trait.").
				AppliesTo("deployments.apps").
				ConflictsWith("scaler", "cpuscaler")

			Expect(trait.GetConflictsWith()).To(Equal([]string{"scaler", "cpuscaler"}))
		})

		It("should set Stage correctly", func() {
			trait := defkit.NewTrait("expose").
				Description("Expose service.").
				Stage("PostDispatch").
				AppliesTo("deployments.apps")

			Expect(trait.GetStage()).To(Equal("PostDispatch"))
		})

		It("should add parameters with Params and Param methods", func() {
			trait := defkit.NewTrait("test").
				AppliesTo("deployments.apps").
				Params(
					defkit.Int("replicas").Default(1),
					defkit.String("image"),
				).
				Param(defkit.Bool("enabled").Default(true))

			params := trait.GetParams()
			Expect(params).To(HaveLen(3))
			Expect(params[0].Name()).To(Equal("replicas"))
			Expect(params[1].Name()).To(Equal("image"))
			Expect(params[2].Name()).To(Equal("enabled"))
		})

		It("should set Labels correctly", func() {
			labels := map[string]string{
				"ui-hidden": "true",
				"custom":    "value",
			}
			trait := defkit.NewTrait("labeled").
				Description("Trait with labels").
				AppliesTo("deployments.apps").
				Labels(labels)

			gotLabels := trait.GetLabels()
			Expect(gotLabels["ui-hidden"]).To(Equal("true"))
			Expect(gotLabels["custom"]).To(Equal("value"))
		})

		It("should set imports correctly", func() {
			trait := defkit.NewTrait("import-trait").
				WithImports("strconv", "strings").
				AppliesTo("deployments.apps")

			Expect(trait.GetImports()).To(Equal([]string{"strconv", "strings"}))
		})
	})

	Context("Status and Health Methods", func() {
		It("should set custom status and health policy", func() {
			trait := defkit.NewTrait("status-trait").
				AppliesTo("deployments.apps").
				CustomStatus("message: \"Ready\"").
				HealthPolicy("isHealth: true")

			Expect(trait.GetCustomStatus()).To(Equal("message: \"Ready\""))
			Expect(trait.GetHealthPolicy()).To(Equal("isHealth: true"))
		})

		It("should set health policy from expression", func() {
			trait := defkit.NewTrait("health").
				AppliesTo("deployments.apps").
				HealthPolicyExpr(defkit.Health().Condition("Ready").IsTrue())

			policy := trait.GetHealthPolicy()
			Expect(policy).NotTo(BeEmpty())
			Expect(policy).To(ContainSubstring("isHealth"))
		})
	})

	Context("RawCUE Methods", func() {
		It("should set raw CUE with complete definition", func() {
			rawCUE := `scaler: {
	type: "trait"
	description: "Raw CUE trait"
}
template: {
	patch: spec: replicas: parameter.replicas
	parameter: replicas: *1 | int
}`
			trait := defkit.NewTrait("scaler").RawCUE(rawCUE)

			Expect(trait.HasRawCUE()).To(BeTrue())
			Expect(trait.GetRawCUE()).To(Equal(rawCUE))
		})

		It("should set template block correctly", func() {
			templateBlock := `
#PatchParams: {
	containerName: *"" | string
	command: *null | [...string]
}
patch: spec: template: spec: containers: [{name: parameter.containerName}]
parameter: #PatchParams
`
			trait := defkit.NewTrait("command").
				Description("Add command").
				AppliesTo("deployments.apps").
				TemplateBlock(templateBlock)

			Expect(trait.HasTemplateBlock()).To(BeTrue())
			Expect(trait.GetTemplateBlock()).To(Equal(templateBlock))
		})
	})

	Context("Helper Methods", func() {
		It("should add helper definitions", func() {
			probeSchema := defkit.Struct("probe").Fields(
				defkit.Field("path", defkit.ParamTypeString).Default("/health"),
				defkit.Field("port", defkit.ParamTypeInt).Default(8080),
			)

			trait := defkit.NewTrait("health-probe").
				Description("Add health probes").
				AppliesTo("deployments.apps").
				Helper("HealthProbe", probeSchema)

			helpers := trait.GetHelperDefinitions()
			Expect(helpers).To(HaveLen(1))
			Expect(helpers[0].GetName()).To(Equal("HealthProbe"))
		})
	})

	Context("ToCue Generation - Metadata", func() {
		It("should generate complete CUE definition with header", func() {
			trait := defkit.NewTrait("scaler").
				Description("Scale workloads").
				AppliesTo("deployments.apps").
				Params(defkit.Int("replicas").Default(1).Required().Description("Number of replicas"))

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring(`scaler: {`))
			Expect(cue).To(ContainSubstring(`type: "trait"`))
			Expect(cue).To(ContainSubstring(`description: "Scale workloads"`))
			// podDisruptive is always emitted
			Expect(cue).To(ContainSubstring(`podDisruptive: false`))
			Expect(cue).To(ContainSubstring(`appliesToWorkloads: ["deployments.apps"]`))
		})

		It("should quote trait names with special characters", func() {
			trait := defkit.NewTrait("my-trait-v1.0").
				Description("Trait with special characters").
				AppliesTo("deployments.apps")

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring(`"my-trait-v1.0": {`))
		})

		It("should include labels in CUE output", func() {
			trait := defkit.NewTrait("labeled").
				Description("Trait with labels").
				AppliesTo("deployments.apps").
				Labels(map[string]string{"ui-hidden": "true"})

			cue := trait.ToCue()

			// CUE formatter may output labels inline or as block
			Expect(cue).To(ContainSubstring(`labels:`))
			Expect(cue).To(ContainSubstring(`"ui-hidden": "true"`))
		})

		It("should include stage in CUE output", func() {
			trait := defkit.NewTrait("staged").
				Description("Trait with stage").
				AppliesTo("deployments.apps").
				Stage("PostDispatch")

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring(`stage:`))
			Expect(cue).To(ContainSubstring(`"PostDispatch"`))
		})

		It("should include conflictsWith in CUE output", func() {
			trait := defkit.NewTrait("exclusive").
				Description("Exclusive trait").
				AppliesTo("deployments.apps").
				ConflictsWith("other-trait", "incompatible-trait")

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring(`conflictsWith: ["other-trait", "incompatible-trait"]`))
		})

		It("should include imports in CUE output", func() {
			trait := defkit.NewTrait("with-imports").
				Description("Trait with imports").
				AppliesTo("deployments.apps").
				WithImports("strconv", "strings")

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring(`import (`))
			Expect(cue).To(ContainSubstring(`"strconv"`))
			Expect(cue).To(ContainSubstring(`"strings"`))
		})
	})

	Context("ToCue Generation - Status", func() {
		It("should include status block with customStatus and healthPolicy", func() {
			trait := defkit.NewTrait("health-aware").
				Description("Health aware trait").
				AppliesTo("deployments.apps").
				CustomStatus("message: \"Running\"").
				HealthPolicy("isHealth: output.status.ready == true")

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring(`status: {`))
			Expect(cue).To(ContainSubstring(`customStatus:`))
			Expect(cue).To(ContainSubstring(`message: "Running"`))
			Expect(cue).To(ContainSubstring(`healthPolicy:`))
			Expect(cue).To(ContainSubstring(`isHealth: output.status.ready == true`))
		})
	})

	Context("ToCue Generation - Template with Patch", func() {
		It("should generate patch block from Template API", func() {
			replicas := defkit.Int("replicas").Default(1)

			trait := defkit.NewTrait("scaler").
				Description("Manually scale K8s pod.").
				AppliesTo("deployments.apps", "statefulsets.apps").
				PodDisruptive(false).
				Params(replicas).
				Template(func(tpl *defkit.Template) {
					tpl.PatchStrategy("retainKeys").
						Patch().Set("spec.replicas", replicas)
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring(`// +patchStrategy=retainKeys`))
			Expect(cue).To(ContainSubstring(`patch:`))
			Expect(cue).To(ContainSubstring(`spec:`))
			Expect(cue).To(ContainSubstring(`replicas:`))
		})
	})

	Context("ToCue Generation - Template with Outputs", func() {
		It("should generate outputs block from Template API", func() {
			trait := defkit.NewTrait("expose").
				Description("Expose workload via Service").
				AppliesTo("deployments.apps").
				Stage("PostDispatch").
				Params(
					defkit.Int("port").Default(80).Description("Service port"),
					defkit.String("type").Default("ClusterIP").Description("Service type"),
				).
				Template(func(tpl *defkit.Template) {
					service := defkit.NewResource("v1", "Service").
						Set("metadata.name", defkit.VelaCtx().Name()).
						Set("spec.type", defkit.ParamRef("type")).
						Set("spec.ports[0].port", defkit.ParamRef("port"))
					tpl.Outputs("service", service)
				})

			cue := trait.ToCue()

			// CUE formatter may inline single outputs
			Expect(cue).To(ContainSubstring(`outputs:`))
			Expect(cue).To(ContainSubstring(`service:`))
			Expect(cue).To(ContainSubstring(`apiVersion: "v1"`))
			Expect(cue).To(ContainSubstring(`kind:`))
			Expect(cue).To(ContainSubstring(`"Service"`))
			Expect(cue).To(ContainSubstring(`metadata: name: context.name`))
		})
	})

	Context("ToCue Generation - RawCUE", func() {
		It("should handle raw CUE with top-level template block", func() {
			rawCUE := `scaler: {
	type: "trait"
	description: "Raw CUE trait"
}
template: {
	patch: spec: replicas: parameter.replicas
	parameter: replicas: *1 | int
}`
			trait := defkit.NewTrait("scaler").RawCUE(rawCUE)

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring(`scaler:`))
			Expect(cue).To(ContainSubstring(`template:`))
			Expect(cue).To(ContainSubstring(`patch: spec: replicas: parameter.replicas`))
		})

		It("should handle raw CUE without top-level template (partial)", func() {
			rawCUE := `patch: spec: replicas: parameter.replicas
parameter: replicas: *1 | int`

			trait := defkit.NewTrait("partial").
				Description("Partial raw CUE trait").
				AppliesTo("deployments.apps").
				RawCUE(rawCUE)

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring(`partial:`))
			Expect(cue).To(ContainSubstring(`type:`))
			Expect(cue).To(ContainSubstring(`"trait"`))
			Expect(cue).To(ContainSubstring(`template:`))
		})

		It("should handle TemplateBlock", func() {
			templateBlock := `#PatchParams: {
	containerName: *"" | string
}
patch: spec: template: spec: containers: [{name: parameter.containerName}]
parameter: #PatchParams`

			trait := defkit.NewTrait("command").
				Description("Add command").
				AppliesTo("deployments.apps").
				TemplateBlock(templateBlock)

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring(`command: {`))
			Expect(cue).To(ContainSubstring(`template: {`))
			Expect(cue).To(ContainSubstring(`#PatchParams:`))
		})

		It("should preserve raw CUE content when it has template block", func() {
			rawCUE := `myname: {
	type: "trait"
}
template: {
	patch: spec: replicas: 1
}`
			trait := defkit.NewTrait("myname").RawCUE(rawCUE)

			cue := trait.ToCue()

			// Raw CUE with top-level template block is returned as-is (formatted)
			Expect(cue).To(ContainSubstring(`myname:`))
			Expect(cue).To(ContainSubstring(`template:`))
			Expect(cue).To(ContainSubstring(`patch:`))
		})
	})

	Context("ToCue Generation - Parameters", func() {
		It("should generate parameter block", func() {
			trait := defkit.NewTrait("params").
				AppliesTo("deployments.apps").
				Params(
					defkit.Int("replicas").Default(1).Description("Number of replicas"),
					defkit.String("image").Required().Description("Container image"),
				)

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring(`parameter: {`))
			Expect(cue).To(ContainSubstring(`// +usage=Number of replicas`))
			Expect(cue).To(ContainSubstring(`replicas: *1 | int`))
			Expect(cue).To(ContainSubstring(`// +usage=Container image`))
			Expect(cue).To(ContainSubstring(`image: string`))
		})
	})

	Context("ToYAML Generation", func() {
		It("should generate valid Kubernetes YAML", func() {
			trait := defkit.NewTrait("scaler").
				Description("Scale workload.").
				AppliesTo("deployments.apps").
				PodDisruptive(false).
				Params(defkit.Int("replicas").Default(1))

			yamlBytes, err := trait.ToYAML()
			Expect(err).NotTo(HaveOccurred())

			yamlStr := string(yamlBytes)
			Expect(yamlStr).To(ContainSubstring("kind: TraitDefinition"))
			Expect(yamlStr).To(ContainSubstring("name: scaler"))
			Expect(yamlStr).To(ContainSubstring("appliesToWorkloads:"))
		})

		It("should include conflictsWith and stage in YAML", func() {
			trait := defkit.NewTrait("exclusive").
				Description("Exclusive trait").
				AppliesTo("deployments.apps").
				ConflictsWith("other-trait").
				Stage("PreDispatch").
				PodDisruptive(true)

			yamlBytes, err := trait.ToYAML()
			Expect(err).NotTo(HaveOccurred())

			yamlStr := string(yamlBytes)
			Expect(yamlStr).To(ContainSubstring("conflictsWith:"))
			Expect(yamlStr).To(ContainSubstring("stage: PreDispatch"))
			Expect(yamlStr).To(ContainSubstring("podDisruptive: true"))
		})
	})

	Context("Registry Integration", func() {
		It("should register and retrieve traits", func() {
			defkit.Clear() // Reset registry

			trait1 := defkit.NewTrait("scaler").Description("Scale").AppliesTo("deployments.apps")
			trait2 := defkit.NewTrait("expose").Description("Expose").AppliesTo("deployments.apps")
			comp := defkit.NewComponent("webservice").Description("Component")

			defkit.Register(trait1)
			defkit.Register(trait2)
			defkit.Register(comp)

			Expect(defkit.Count()).To(Equal(3))
			Expect(defkit.Traits()).To(HaveLen(2))
			Expect(defkit.Components()).To(HaveLen(1))

			defkit.Clear() // Clean up
		})
	})

	Context("ToCue with Patch operations", func() {
		It("should generate patch with SpreadIf", func() {
			labels := defkit.Object("labels")
			trait := defkit.NewTrait("label").
				Description("Add labels").
				AppliesTo("deployments.apps").
				Params(labels).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().
						SpreadIf(labels.IsSet(), "spec.template.metadata.labels", labels)
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("patch:"))
			Expect(cue).To(ContainSubstring("labels"))
		})

		It("should generate patch with ForEach", func() {
			labels := defkit.Object("labels")
			trait := defkit.NewTrait("label").
				Description("Add labels").
				AppliesTo("deployments.apps").
				Params(labels).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().ForEach(labels, "spec.template.metadata.labels")
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("patch:"))
		})

		It("should generate patch with If/EndIf block", func() {
			enabled := defkit.Bool("enabled")
			replicas := defkit.Int("replicas")
			trait := defkit.NewTrait("conditional").
				Description("Conditional scaling").
				AppliesTo("deployments.apps").
				Params(enabled, replicas).
				Template(func(tpl *defkit.Template) {
					cond := defkit.Eq(enabled, defkit.Lit(true))
					tpl.Patch().
						If(cond).
						Set("spec.replicas", replicas).
						EndIf()
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("patch:"))
			Expect(cue).To(ContainSubstring("if"))
		})

		It("should generate patch with PatchKey", func() {
			containerName := defkit.String("containerName")
			image := defkit.String("image")
			trait := defkit.NewTrait("sidecar").
				Description("Add sidecar container").
				AppliesTo("deployments.apps").
				Params(containerName, image).
				Template(func(tpl *defkit.Template) {
					container := defkit.NewArrayElement().
						Set("name", containerName).
						Set("image", image)
					tpl.Patch().PatchKey("spec.template.spec.containers", "name", container)
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("patch:"))
			Expect(cue).To(ContainSubstring("patchKey"))
		})

		It("should generate patch with Passthrough", func() {
			trait := defkit.NewTrait("json-patch").
				Description("Apply JSON patch").
				AppliesTo("*").
				Params(defkit.OpenStruct()).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().Passthrough()
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("patch:"))
		})

		It("should generate Optional field guards in Map comprehension", func() {
			items := defkit.Array("items").WithFields(
				defkit.String("name").Required(),
				defkit.String("label"),
				defkit.Int("priority"),
			)
			trait := defkit.NewTrait("optional-test").
				Description("Test optional fields").
				AppliesTo("deployments.apps").
				Params(items).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().
						SetIf(items.IsSet(), "spec.template.spec.items",
							defkit.From(items).Map(defkit.FieldMap{
								"name":     defkit.F("name"),
								"label":    defkit.Optional("label"),
								"priority": defkit.Optional("priority"),
							}))
				})

			cue := trait.ToCue()

			// Required field should use direct access
			Expect(cue).To(ContainSubstring("name: v.name"))
			// Optional fields should have if guards
			Expect(cue).To(ContainSubstring("if v.label != _|_"))
			Expect(cue).To(ContainSubstring("label: v.label"))
			Expect(cue).To(ContainSubstring("if v.priority != _|_"))
			Expect(cue).To(ContainSubstring("priority: v.priority"))
			// Should NOT contain top-level underscore for optional fields
			Expect(cue).NotTo(ContainSubstring("label: _"))
			Expect(cue).NotTo(ContainSubstring("priority: _"))
		})

		It("should generate If/EndIf with SetIf using sub-field conditions", func() {
			parent := defkit.Map("parent").WithFields(
				defkit.Array("required").WithFields(
					defkit.String("key").Required(),
				),
				defkit.Array("preferred").WithFields(
					defkit.Int("weight").Required(),
				),
			)
			trait := defkit.NewTrait("if-subfield-test").
				Description("Test If/EndIf with sub-field conditions").
				AppliesTo("deployments.apps").
				Params(parent).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().
						If(parent.IsSet()).
						SetIf(parent.Field("required").IsSet(),
							"spec.requiredItems",
							defkit.From(defkit.ParamPath("parent.required")).Map(defkit.FieldMap{
								"key": defkit.F("key"),
							})).
						SetIf(parent.Field("preferred").IsSet(),
							"spec.preferredItems",
							defkit.From(defkit.ParamPath("parent.preferred")).Map(defkit.FieldMap{
								"weight": defkit.F("weight"),
							})).
						EndIf()
				})

			cue := trait.ToCue()

			// Both conditions should appear (AND-combined by the field tree)
			Expect(cue).To(ContainSubstring(`parameter["parent"] != _|_`))
			Expect(cue).To(ContainSubstring("parameter.parent.required != _|_"))
			Expect(cue).To(ContainSubstring("parameter.parent.preferred != _|_"))
			// Verify the For comprehension sources
			Expect(cue).To(ContainSubstring("for v in parameter.parent.required"))
			Expect(cue).To(ContainSubstring("for v in parameter.parent.preferred"))
			Expect(cue).To(ContainSubstring("key: v.key"))
			Expect(cue).To(ContainSubstring("weight: v.weight"))
		})
	})

	Context("Let Bindings with ForEachMap", func() {
		It("should generate let binding with ForEachMap and LetVariable references", func() {
			trait := defkit.NewTrait("let-foreach-test").
				Description("Test let binding with ForEachMap").
				AppliesTo("*").
				PodDisruptive(true).
				Param(defkit.DynamicMap().ValueTypeUnion("string | null")).
				Template(func(tpl *defkit.Template) {
					tpl.PatchStrategy("jsonMergePatch")
					tpl.AddLetBinding("content", defkit.ForEachMap())
					tpl.Patch().
						Set("metadata.annotations", defkit.LetVariable("content"))
					tpl.Patch().
						If(defkit.And(
							defkit.ContextOutput().HasPath("spec"),
							defkit.ContextOutput().HasPath("spec.template"),
						)).
						Set("spec.template.metadata.annotations", defkit.LetVariable("content")).
						EndIf()
				})

			cue := trait.ToCue()

			// Let binding should appear before the patch block
			Expect(cue).To(ContainSubstring("let content ="))
			// ForEachMap should render as a struct comprehension
			Expect(cue).To(ContainSubstring("for k, v in parameter"))
			Expect(cue).To(ContainSubstring("(k): v"))
			// Let variable references should appear in the patch
			Expect(cue).To(ContainSubstring("metadata: annotations: content"))
			// Conditional block should reference the let variable
			Expect(cue).To(ContainSubstring("annotations: content"))
			Expect(cue).To(ContainSubstring("context.output.spec != _|_"))
			Expect(cue).To(ContainSubstring("context.output.spec.template != _|_"))
			// The for-each comprehension should NOT be inlined at each usage site
			// It should only appear once in the let binding
			Expect(strings.Count(cue, "for k, v in parameter")).To(Equal(1))
			// Patch strategy should be present
			Expect(cue).To(ContainSubstring("// +patchStrategy=jsonMergePatch"))
		})

		It("should render ForEachMap with custom source and vars via valueToCUE", func() {
			trait := defkit.NewTrait("custom-foreach-test").
				Description("Test custom ForEachMap rendering").
				AppliesTo("deployments.apps").
				Param(defkit.DynamicMap().ValueTypeUnion("string | null")).
				Template(func(tpl *defkit.Template) {
					tpl.AddLetBinding("labelContent",
						defkit.ForEachMap().Over("parameter.labels").WithVars("key", "val"))
					tpl.Patch().
						Set("metadata.labels", defkit.LetVariable("labelContent"))
				})

			cue := trait.ToCue()

			// Custom variable names and source
			Expect(cue).To(ContainSubstring("let labelContent ="))
			Expect(cue).To(ContainSubstring("for key, val in parameter.labels"))
			Expect(cue).To(ContainSubstring("(key): val"))
			Expect(cue).To(ContainSubstring("metadata: labels: labelContent"))
		})
	})

	Context("PatchKey with ArrayParam (no array wrapping)", func() {
		It("should emit direct assignment when single element is an ArrayParam", func() {
			items := defkit.Array("items").WithFields(
				defkit.String("name").Required(),
				defkit.String("value").Required(),
			).Required()

			trait := defkit.NewTrait("patchkey-array-test").
				Description("Test PatchKey with ArrayParam").
				AppliesTo("deployments.apps").
				Params(items).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().
						PatchKey("spec.template.spec.items", "name", items)
				})

			cue := trait.ToCue()

			// Should emit patchKey annotation
			Expect(cue).To(ContainSubstring("// +patchKey=name"))
			// Should assign parameter directly, NOT wrapped in [...]
			Expect(cue).To(ContainSubstring("items: parameter.items"))
			// Should NOT have array wrapping around the parameter
			Expect(cue).NotTo(ContainSubstring("[parameter.items]"))
		})

		It("should still wrap individual ArrayElements in array brackets", func() {
			elem := defkit.NewArrayElement().
				Set("name", defkit.Lit("test")).
				Set("value", defkit.Lit("foo"))

			trait := defkit.NewTrait("patchkey-elem-test").
				Description("Test PatchKey with ArrayElement").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.Patch().
						PatchKey("spec.items", "name", elem)
				})

			cue := trait.ToCue()

			// ArrayElement should still be wrapped in [...]
			Expect(cue).To(ContainSubstring("// +patchKey=name"))
			Expect(cue).To(ContainSubstring("items: [{"))
		})
	})

	Context("IsSet bracket notation for optional field checks", func() {
		It("should generate bracket notation for IsSet conditions", func() {
			optParam := defkit.Array("env").Of(defkit.ParamTypeString).Optional()

			trait := defkit.NewTrait("isset-bracket-test").
				Description("Test bracket notation for IsSet").
				AppliesTo("deployments.apps").
				Params(optParam).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().
						SetIf(optParam.IsSet(), "spec.env", optParam)
				})

			cue := trait.ToCue()

			// Should use bracket notation: parameter["env"] != _|_
			Expect(cue).To(ContainSubstring(`parameter["env"] != _|_`))
			// Should NOT use dot notation for the condition
			Expect(cue).NotTo(ContainSubstring("parameter.env != _|_"))
		})

		It("should generate bracket notation for NotSet (negated IsSet) conditions", func() {
			optParam := defkit.String("debug").Optional()

			trait := defkit.NewTrait("notset-bracket-test").
				Description("Test bracket notation for NotSet").
				AppliesTo("deployments.apps").
				Params(optParam).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().
						SetIf(optParam.NotSet(), "spec.debug", defkit.Lit(false))
				})

			cue := trait.ToCue()

			// Should use bracket notation: parameter["debug"] == _|_
			Expect(cue).To(ContainSubstring(`parameter["debug"] == _|_`))
			// Should NOT use dot notation
			Expect(cue).NotTo(ContainSubstring("parameter.debug == _|_"))
		})

		It("should use bracket notation in compound conditions", func() {
			cpu := defkit.String("cpu").Optional()
			memory := defkit.String("memory").Optional()

			trait := defkit.NewTrait("compound-bracket-test").
				Description("Test bracket notation in compound conditions").
				AppliesTo("deployments.apps").
				Params(cpu, memory).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().
						SetIf(defkit.And(cpu.IsSet(), memory.IsSet()),
							"spec.resources", defkit.Lit("configured"))
				})

			cue := trait.ToCue()

			// Both parts of the compound condition should use bracket notation
			Expect(cue).To(ContainSubstring(`parameter["cpu"] != _|_`))
			Expect(cue).To(ContainSubstring(`parameter["memory"] != _|_`))
		})
	})

	Context("PatchStrategyAnnotation", func() {
		It("should record PatchStrategyAnnotation ops on PatchResource", func() {
			p := defkit.NewPatchResource()
			p.PatchStrategyAnnotation("spec.strategy", "retainKeys")

			ops := p.Ops()
			Expect(ops).To(HaveLen(1))
			ann, ok := ops[0].(*defkit.PatchStrategyAnnotationOp)
			Expect(ok).To(BeTrue())
			Expect(ann.Path()).To(Equal("spec.strategy"))
			Expect(ann.Strategy()).To(Equal("retainKeys"))
		})

		It("should record PatchStrategyAnnotation inside If block", func() {
			p := defkit.NewPatchResource()
			cond := defkit.Eq(defkit.ParameterField("kind"), defkit.Lit("Deployment"))
			p.If(cond).
				PatchStrategyAnnotation("spec.strategy", "retainKeys").
				Set("spec.strategy.type", defkit.ParameterField("strategyType")).
				EndIf()

			ops := p.Ops()
			Expect(ops).To(HaveLen(1))
			ifBlock, ok := ops[0].(*defkit.IfBlock)
			Expect(ok).To(BeTrue())
			Expect(ifBlock.Ops()).To(HaveLen(2)) // PatchStrategyAnnotation + Set
		})

		It("should emit patchStrategy annotation in CUE output for unconditional field", func() {
			strategy := defkit.Struct("strategy").Required().Fields(
				defkit.Field("type", defkit.ParamTypeString).Default("RollingUpdate"),
			)

			trait := defkit.NewTrait("annotation-test").
				Description("Test patchStrategy annotation").
				AppliesTo("deployments.apps").
				Params(strategy).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().
						PatchStrategyAnnotation("spec.strategy", "retainKeys").
						Set("spec.strategy.type", defkit.ParameterField("strategy.type"))
				})

			cue := trait.ToCue()

			// Should contain the annotation comment
			Expect(cue).To(ContainSubstring("// +patchStrategy=retainKeys"))
			// Annotation should appear before the field
			Expect(cue).To(ContainSubstring("strategy: type: parameter.strategy.type"))
		})

		It("should emit patchStrategy annotation inside conditional block", func() {
			kind := defkit.String("kind").Default("Deployment").Enum("Deployment", "StatefulSet")
			strategy := defkit.Struct("strategy").Required().Fields(
				defkit.Field("type", defkit.ParamTypeString).Default("RollingUpdate"),
			)

			trait := defkit.NewTrait("cond-annotation-test").
				Description("Test patchStrategy in conditional block").
				AppliesTo("deployments.apps").
				Params(kind, strategy).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().
						If(defkit.Eq(kind, defkit.Lit("Deployment"))).
						PatchStrategyAnnotation("spec.strategy", "retainKeys").
						Set("spec.strategy.type", defkit.ParameterField("strategy.type")).
						EndIf()
				})

			cue := trait.ToCue()

			// Should contain the annotation inside the if block
			Expect(cue).To(ContainSubstring("// +patchStrategy=retainKeys"))
			Expect(cue).To(ContainSubstring(`parameter.kind == "Deployment"`))
		})
	})

	Context("Condition hoisting", func() {
		It("should hoist matching conditions from children to parent", func() {
			enabled := defkit.Bool("enabled")
			replicas := defkit.Int("replicas")
			image := defkit.String("image")

			trait := defkit.NewTrait("hoist-test").
				Description("Test condition hoisting").
				AppliesTo("deployments.apps").
				Params(enabled, replicas, image).
				Template(func(tpl *defkit.Template) {
					cond := defkit.Eq(enabled, defkit.Lit(true))
					tpl.Patch().
						If(cond).
						Set("spec.replicas", replicas).
						Set("spec.template.spec.image", image).
						EndIf()
				})

			cue := trait.ToCue()

			// The If condition should appear
			Expect(cue).To(ContainSubstring(`parameter.enabled == true`))
			// Both fields should be set inside the condition
			Expect(cue).To(ContainSubstring("replicas: parameter.replicas"))
			Expect(cue).To(ContainSubstring("image: parameter.image"))
		})

		It("should hoist common condition from AND combinations", func() {
			kind := defkit.String("kind").Default("Deployment")
			replicas := defkit.Int("replicas")
			image := defkit.String("image")

			trait := defkit.NewTrait("hoist-and-test").
				Description("Test AND condition hoisting").
				AppliesTo("deployments.apps").
				Params(kind, replicas, image).
				Template(func(tpl *defkit.Template) {
					isDeployment := defkit.Eq(kind, defkit.Lit("Deployment"))
					replicasSet := replicas.IsSet()
					imageSet := image.IsSet()
					tpl.Patch().
						If(isDeployment).
						SetIf(replicasSet, "spec.replicas", replicas).
						SetIf(imageSet, "spec.template.spec.image", image).
						EndIf()
				})

			cue := trait.ToCue()

			// The outer condition should appear at the parent level
			Expect(cue).To(ContainSubstring(`parameter.kind == "Deployment"`))
			// The inner conditions should remain on the children
			Expect(cue).To(ContainSubstring(`parameter["replicas"] != _|_`))
			Expect(cue).To(ContainSubstring(`parameter["image"] != _|_`))
		})
	})

	Context("Multiple IfBlocks with overlapping paths", func() {
		It("should render multiple IfBlocks as separate conditional blocks", func() {
			kind := defkit.String("kind").Default("Deployment").Enum("Deployment", "StatefulSet", "DaemonSet")
			strategyType := defkit.String("strategyType").Default("RollingUpdate")
			maxSurge := defkit.String("maxSurge").Default("25%")
			partition := defkit.Int("partition").Default(0)

			trait := defkit.NewTrait("multi-ifblock-test").
				Description("Test multiple IfBlocks").
				AppliesTo("deployments.apps", "statefulsets.apps", "daemonsets.apps").
				Params(kind, strategyType, maxSurge, partition).
				Template(func(tpl *defkit.Template) {
					isDeployment := defkit.Eq(kind, defkit.Lit("Deployment"))
					isStatefulSet := defkit.Eq(kind, defkit.Lit("StatefulSet"))

					tpl.Patch().
						If(isDeployment).
						PatchStrategyAnnotation("spec.strategy", "retainKeys").
						Set("spec.strategy.type", strategyType).
						Set("spec.strategy.rollingUpdate.maxSurge", maxSurge).
						EndIf().
						If(isStatefulSet).
						PatchStrategyAnnotation("spec.updateStrategy", "retainKeys").
						Set("spec.updateStrategy.type", strategyType).
						Set("spec.updateStrategy.rollingUpdate.partition", partition).
						EndIf()
				})

			cue := trait.ToCue()

			// Should have two separate if blocks (not merged)
			Expect(cue).To(ContainSubstring(`parameter.kind == "Deployment"`))
			Expect(cue).To(ContainSubstring(`parameter.kind == "StatefulSet"`))
			// Each should have its own patchStrategy annotation
			Expect(strings.Count(cue, "// +patchStrategy=retainKeys")).To(Equal(2))
			// Deployment uses "strategy", StatefulSet uses "updateStrategy"
			Expect(cue).To(ContainSubstring("strategy: {"))
			Expect(cue).To(ContainSubstring("updateStrategy: {"))
			// Fields should be present
			Expect(cue).To(ContainSubstring("maxSurge: parameter.maxSurge"))
			Expect(cue).To(ContainSubstring("partition: parameter.partition"))
		})

		It("should produce correct k8s-update-strategy-like pattern", func() {
			targetKind := defkit.String("targetKind").Default("Deployment").Enum("Deployment", "StatefulSet", "DaemonSet")
			strategy := defkit.Struct("strategy").Required().Fields(
				defkit.Field("type", defkit.ParamTypeString).Default("RollingUpdate").Enum("RollingUpdate", "Recreate", "OnDelete"),
				defkit.Field("rollingStrategy", defkit.ParamTypeStruct).
					Nested(defkit.Struct("rollingStrategy").Fields(
						defkit.Field("maxSurge", defkit.ParamTypeString).Default("25%"),
						defkit.Field("maxUnavailable", defkit.ParamTypeString).Default("25%"),
						defkit.Field("partition", defkit.ParamTypeInt).Default(0),
					)),
			)

			trait := defkit.NewTrait("k8s-update-strategy-test").
				Description("Test k8s-update-strategy pattern").
				AppliesTo("deployments.apps", "statefulsets.apps", "daemonsets.apps").
				PodDisruptive(false).
				Params(targetKind, strategy).
				Template(func(tpl *defkit.Template) {
					strategyType := defkit.ParameterField("strategy.type")
					maxSurge := defkit.ParameterField("strategy.rollingStrategy.maxSurge")
					maxUnavailable := defkit.ParameterField("strategy.rollingStrategy.maxUnavailable")
					partition := defkit.ParameterField("strategy.rollingStrategy.partition")

					isDeployment := defkit.Eq(defkit.ParameterField("targetKind"), defkit.Lit("Deployment"))
					isStatefulSet := defkit.Eq(defkit.ParameterField("targetKind"), defkit.Lit("StatefulSet"))
					isDaemonSet := defkit.Eq(defkit.ParameterField("targetKind"), defkit.Lit("DaemonSet"))
					isNotOnDelete := defkit.Ne(strategyType, defkit.Lit("OnDelete"))
					isNotRecreate := defkit.Ne(strategyType, defkit.Lit("Recreate"))
					isRollingUpdate := defkit.Eq(strategyType, defkit.Lit("RollingUpdate"))

					tpl.Patch().
						If(defkit.And(isDeployment, isNotOnDelete)).
						PatchStrategyAnnotation("spec.strategy", "retainKeys").
						Set("spec.strategy.type", strategyType).
						SetIf(isRollingUpdate, "spec.strategy.rollingUpdate.maxSurge", maxSurge).
						SetIf(isRollingUpdate, "spec.strategy.rollingUpdate.maxUnavailable", maxUnavailable).
						EndIf().
						If(defkit.And(isStatefulSet, isNotRecreate)).
						PatchStrategyAnnotation("spec.updateStrategy", "retainKeys").
						Set("spec.updateStrategy.type", strategyType).
						SetIf(isRollingUpdate, "spec.updateStrategy.rollingUpdate.partition", partition).
						EndIf().
						If(defkit.And(isDaemonSet, isNotRecreate)).
						PatchStrategyAnnotation("spec.updateStrategy", "retainKeys").
						Set("spec.updateStrategy.type", strategyType).
						SetIf(isRollingUpdate, "spec.updateStrategy.rollingUpdate.maxSurge", maxSurge).
						SetIf(isRollingUpdate, "spec.updateStrategy.rollingUpdate.maxUnavailable", maxUnavailable).
						EndIf()
				})

			cue := trait.ToCue()

			// Three separate if blocks
			Expect(cue).To(ContainSubstring(`parameter.targetKind == "Deployment" && parameter.strategy.type != "OnDelete"`))
			Expect(cue).To(ContainSubstring(`parameter.targetKind == "StatefulSet" && parameter.strategy.type != "Recreate"`))
			Expect(cue).To(ContainSubstring(`parameter.targetKind == "DaemonSet" && parameter.strategy.type != "Recreate"`))
			// Three patchStrategy annotations
			Expect(strings.Count(cue, "// +patchStrategy=retainKeys")).To(Equal(3))
			// RollingUpdate inner conditions
			Expect(cue).To(ContainSubstring(`parameter.strategy.type == "RollingUpdate"`))
			// Deployment uses "strategy", others use "updateStrategy"
			Expect(cue).To(ContainSubstring("strategy: {"))
			Expect(cue).To(ContainSubstring("updateStrategy: {"))
			// Correct field assignments
			Expect(cue).To(ContainSubstring("maxSurge:       parameter.strategy.rollingStrategy.maxSurge"))
			Expect(cue).To(ContainSubstring("maxUnavailable: parameter.strategy.rollingStrategy.maxUnavailable"))
			Expect(cue).To(ContainSubstring("partition: parameter.strategy.rollingStrategy.partition"))
		})
	})

	Context("Raw patch block with fluent params and helpers", func() {
		It("should render raw patch block followed by fluent parameter and helper definitions", func() {
			postStart := defkit.Map("postStart").WithSchemaRef("Handler")
			preStop := defkit.Map("preStop").WithSchemaRef("Handler")

			trait := defkit.NewTrait("lifecycle").
				Description("test").
				AppliesTo("deployments.apps").
				PodDisruptive(true).
				Params(postStart, preStop).
				Helper("Handler", defkit.Struct("Handler").Fields(
					defkit.Field("exec", defkit.ParamTypeStruct).
						Nested(defkit.Struct("exec").Fields(
							defkit.Field("command", defkit.ParamTypeArray).ArrayOf(defkit.ParamTypeString).Required(),
						)),
				)).
				Template(func(tpl *defkit.Template) {
					tpl.SetRawPatchBlock(`patch: spec: containers: [...{
	lifecycle: {
		if parameter.postStart != _|_ {
			postStart: parameter.postStart
		}
	}
}]`)
				})

			cue := trait.ToCue()

			// Raw patch block is rendered
			Expect(cue).To(ContainSubstring("containers: [...{"))
			Expect(cue).To(ContainSubstring("lifecycle: {"))
			Expect(cue).To(ContainSubstring("if parameter.postStart != _|_"))

			// Fluent parameter block is rendered (not skipped)
			Expect(cue).To(ContainSubstring("parameter: {"))
			Expect(cue).To(ContainSubstring("postStart?: #Handler"))
			// CUE formatter may add alignment spaces: preStop?:   #Handler
			Expect(cue).To(ContainSubstring("preStop?:"))
			Expect(cue).To(MatchRegexp(`preStop\?:\s+#Handler`))

			// Helper definition is rendered (not skipped)
			// CUE formatter collapses single-field struct to inline form
			Expect(cue).To(ContainSubstring("#Handler:"))
			Expect(cue).To(ContainSubstring("command: [...string]"))
		})

		It("should still use full raw mode when raw parameter block is set", func() {
			trait := defkit.NewTrait("raw-all").
				Description("test").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.SetRawPatchBlock(`patch: spec: replicas: parameter.replicas`)
					tpl.SetRawParameterBlock(`parameter: {
	replicas: *1 | int
}`)
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("replicas: parameter.replicas"))
			Expect(cue).To(ContainSubstring("replicas: *1 | int"))
			// Should NOT have a duplicate parameter block
			Expect(strings.Count(cue, "parameter:")).To(Equal(2)) // one in patch ref, one in param block
		})
	})

	Context("IntParam helper definition rendering", func() {
		It("should render constrained int helper with min and max", func() {
			trait := defkit.NewTrait("int-helper-test").
				Description("test").
				AppliesTo("deployments.apps").
				Helper("Port", defkit.Int("Port").Min(1).Max(65535)).
				Template(func(tpl *defkit.Template) {
					tpl.SetRawPatchBlock(`patch: spec: port: parameter.port`)
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("#Port: int & >=1 & <=65535"))
		})

		It("should render int helper with only min constraint", func() {
			trait := defkit.NewTrait("int-min-test").
				Description("test").
				AppliesTo("deployments.apps").
				Helper("Positive", defkit.Int("Positive").Min(0)).
				Template(func(tpl *defkit.Template) {
					tpl.SetRawPatchBlock(`patch: spec: count: parameter.count`)
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("#Positive: int & >=0"))
			Expect(cue).NotTo(ContainSubstring("<="))
		})

		It("should render int helper without constraints", func() {
			trait := defkit.NewTrait("int-bare-test").
				Description("test").
				AppliesTo("deployments.apps").
				Helper("Count", defkit.Int("Count")).
				Template(func(tpl *defkit.Template) {
					tpl.SetRawPatchBlock(`patch: spec: count: parameter.count`)
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("#Count: int"))
			Expect(cue).NotTo(ContainSubstring(">="))
			Expect(cue).NotTo(ContainSubstring("<="))
		})
	})

	Context("SpreadAll operation", func() {
		It("should record SpreadAll ops on PatchResource", func() {
			p := defkit.NewPatchResource()
			elem := defkit.NewArrayElement().Set("name", defkit.Lit("test"))
			p.SpreadAll("spec.containers", elem)

			ops := p.Ops()
			Expect(ops).To(HaveLen(1))
			sa, ok := ops[0].(*defkit.SpreadAllOp)
			Expect(ok).To(BeTrue())
			Expect(sa.Path()).To(Equal("spec.containers"))
			Expect(sa.Elements()).To(HaveLen(1))
		})

		It("should record SpreadAll inside If block", func() {
			p := defkit.NewPatchResource()
			cond := defkit.Eq(defkit.ParameterField("enabled"), defkit.Lit(true))
			elem := defkit.NewArrayElement().Set("name", defkit.Lit("test"))
			p.If(cond).
				SpreadAll("spec.containers", elem).
				EndIf()

			ops := p.Ops()
			Expect(ops).To(HaveLen(1))
			ifBlock, ok := ops[0].(*defkit.IfBlock)
			Expect(ok).To(BeTrue())
			Expect(ifBlock.Ops()).To(HaveLen(1))
			_, ok = ifBlock.Ops()[0].(*defkit.SpreadAllOp)
			Expect(ok).To(BeTrue())
		})

		It("should render unconditional SpreadAll with simple value", func() {
			image := defkit.String("image").Required()

			trait := defkit.NewTrait("spreadall-simple-test").
				Description("Test SpreadAll with simple value").
				AppliesTo("deployments.apps").
				Params(image).
				Template(func(tpl *defkit.Template) {
					elem := defkit.NewArrayElement().
						Set("image", image)
					tpl.Patch().SpreadAll("spec.template.spec.containers", elem)
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("containers: [...{"))
			Expect(cue).To(ContainSubstring("image: parameter.image"))
		})

		It("should render conditional SpreadAll with ArrayElement SetIf", func() {
			postStart := defkit.String("postStart")
			preStop := defkit.String("preStop")

			trait := defkit.NewTrait("spreadall-conditional-test").
				Description("Test SpreadAll with conditional fields").
				AppliesTo("deployments.apps").
				Params(postStart, preStop).
				Template(func(tpl *defkit.Template) {
					elem := defkit.NewArrayElement().
						SetIf(postStart.IsSet(), "lifecycle.postStart.exec.command", postStart).
						SetIf(preStop.IsSet(), "lifecycle.preStop.exec.command", preStop)
					tpl.Patch().SpreadAll("spec.template.spec.containers", elem)
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("containers: [...{"))
			Expect(cue).To(ContainSubstring(`parameter["postStart"] != _|_`))
			Expect(cue).To(ContainSubstring(`parameter["preStop"] != _|_`))
		})

		It("should render SpreadAll inside an IfBlock", func() {
			enabled := defkit.Bool("enabled").Default(false)
			image := defkit.String("image").Required()

			trait := defkit.NewTrait("spreadall-ifblock-test").
				Description("Test SpreadAll inside IfBlock").
				AppliesTo("deployments.apps").
				Params(enabled, image).
				Template(func(tpl *defkit.Template) {
					elem := defkit.NewArrayElement().
						Set("image", image)
					tpl.Patch().
						If(defkit.Eq(enabled, defkit.Lit(true))).
						SpreadAll("spec.template.spec.containers", elem).
						EndIf()
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("parameter.enabled == true"))
			Expect(cue).To(ContainSubstring("containers: [...{"))
			Expect(cue).To(ContainSubstring("image: parameter.image"))
		})
	})

	Context("Multiple IfBlocks with bare ops", func() {
		It("should render bare ops before conditional IfBlocks", func() {
			kind := defkit.String("kind").Default("Deployment").Enum("Deployment", "StatefulSet")
			replicas := defkit.Int("replicas").Default(1)
			strategyType := defkit.String("strategyType").Default("RollingUpdate")

			trait := defkit.NewTrait("bare-ops-ifblock-test").
				Description("Test bare ops with multiple IfBlocks").
				AppliesTo("deployments.apps", "statefulsets.apps").
				Params(kind, replicas, strategyType).
				Template(func(tpl *defkit.Template) {
					isDeployment := defkit.Eq(kind, defkit.Lit("Deployment"))
					isStatefulSet := defkit.Eq(kind, defkit.Lit("StatefulSet"))

					tpl.Patch().
						Set("spec.replicas", replicas).
						If(isDeployment).
						Set("spec.strategy.type", strategyType).
						EndIf().
						If(isStatefulSet).
						Set("spec.updateStrategy.type", strategyType).
						EndIf()
				})

			cue := trait.ToCue()

			// Bare op should be present
			Expect(cue).To(ContainSubstring("replicas: parameter.replicas"))
			// Both conditional blocks
			Expect(cue).To(ContainSubstring(`parameter.kind == "Deployment"`))
			Expect(cue).To(ContainSubstring(`parameter.kind == "StatefulSet"`))
			Expect(cue).To(ContainSubstring("strategy: type:"))
			Expect(cue).To(ContainSubstring("updateStrategy: type:"))
		})
	})

	Context("findIfBlockCommonPrefix edge cases", func() {
		It("should handle IfBlocks with no common prefix", func() {
			kind := defkit.String("kind").Default("a").Enum("a", "b")

			trait := defkit.NewTrait("no-common-prefix-test").
				Description("Test no common prefix").
				AppliesTo("deployments.apps").
				Params(kind).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().
						If(defkit.Eq(kind, defkit.Lit("a"))).
						Set("spec.strategy.type", defkit.Lit("RollingUpdate")).
						EndIf().
						If(defkit.Eq(kind, defkit.Lit("b"))).
						Set("metadata.labels.version", defkit.Lit("v2")).
						EndIf()
				})

			cue := trait.ToCue()

			// Both conditions should be rendered
			Expect(cue).To(ContainSubstring(`parameter.kind == "a"`))
			Expect(cue).To(ContainSubstring(`parameter.kind == "b"`))
			Expect(cue).To(ContainSubstring("strategy: type:"))
			Expect(cue).To(ContainSubstring("labels: version:"))
		})
	})

	Context("Nested array struct in helper definitions", func() {
		It("should render array field with nested struct as [...{fields}]", func() {
			trait := defkit.NewTrait("nested-array-test").
				Description("test").
				AppliesTo("deployments.apps").
				Helper("Config", defkit.Struct("Config").Fields(
					defkit.Field("headers", defkit.ParamTypeArray).
						Nested(defkit.Struct("headers").Fields(
							defkit.Field("name", defkit.ParamTypeString).Required(),
							defkit.Field("value", defkit.ParamTypeString).Required(),
						)),
				)).
				Template(func(tpl *defkit.Template) {
					tpl.SetRawPatchBlock(`patch: spec: config: parameter.config`)
				})

			cue := trait.ToCue()

			// CUE formatter collapses single-field struct to inline form
			Expect(cue).To(ContainSubstring("#Config: headers?: [...{"))
			Expect(cue).To(ContainSubstring("name:  string"))
			Expect(cue).To(ContainSubstring("value: string"))
		})

		It("should render schema ref on fields within helper structs", func() {
			trait := defkit.NewTrait("schema-ref-test").
				Description("test").
				AppliesTo("deployments.apps").
				Helper("Port", defkit.Int("Port").Min(1).Max(65535)).
				Helper("Endpoint", defkit.Struct("Endpoint").Fields(
					defkit.Field("port", defkit.ParamTypeInt).WithSchemaRef("Port").Required(),
					defkit.Field("host", defkit.ParamTypeString),
				)).
				Template(func(tpl *defkit.Template) {
					tpl.SetRawPatchBlock(`patch: spec: endpoint: parameter.endpoint`)
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("#Port: int & >=1 & <=65535"))
			Expect(cue).To(ContainSubstring("#Endpoint: {"))
			Expect(cue).To(ContainSubstring("port:  #Port"))
			Expect(cue).To(ContainSubstring("host?: string"))
		})
	})

	Context("podDisruptive always emitted", func() {
		It("should emit podDisruptive: true when set to true", func() {
			trait := defkit.NewTrait("disruptive").
				Description("Disruptive trait").
				AppliesTo("deployments.apps").
				PodDisruptive(true)

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring("podDisruptive: true"))
		})

		It("should emit podDisruptive: false when set to false", func() {
			trait := defkit.NewTrait("nondisruptive").
				Description("Non-disruptive trait").
				AppliesTo("deployments.apps").
				PodDisruptive(false)

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring("podDisruptive: false"))
		})

		It("should emit podDisruptive: false when not explicitly set", func() {
			trait := defkit.NewTrait("default-disruptive").
				Description("Default trait").
				AppliesTo("deployments.apps")

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring("podDisruptive: false"))
		})
	})

	Context("ConflictsWith empty list emission", func() {
		It("should emit conflictsWith with values when set", func() {
			trait := defkit.NewTrait("conflicts-with-values").
				Description("Trait with conflicts").
				AppliesTo("deployments.apps").
				ConflictsWith("scaler", "hpa")

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring(`conflictsWith: ["scaler", "hpa"]`))
		})

		It("should emit conflictsWith: [] when explicitly set with no values", func() {
			trait := defkit.NewTrait("conflicts-empty").
				Description("Trait with empty conflicts").
				AppliesTo("deployments.apps").
				ConflictsWith()

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring("conflictsWith: []"))
		})

		It("should not emit conflictsWith when never called", func() {
			trait := defkit.NewTrait("no-conflicts").
				Description("Trait without conflicts").
				AppliesTo("deployments.apps")

			cue := trait.ToCue()
			Expect(cue).NotTo(ContainSubstring("conflictsWith"))
		})
	})

	Context("Labels nil vs empty emission", func() {
		It("should emit labels with values when set", func() {
			trait := defkit.NewTrait("labels-values").
				Description("Trait with labels").
				AppliesTo("deployments.apps").
				Labels(map[string]string{"ui-hidden": "true"})

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring("labels:"))
			Expect(cue).To(ContainSubstring(`"ui-hidden": "true"`))
		})

		It("should emit labels: {} when explicitly set to empty map", func() {
			trait := defkit.NewTrait("labels-empty").
				Description("Trait with empty labels").
				AppliesTo("deployments.apps").
				Labels(map[string]string{})

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring("labels: {}"))
		})

		It("should not emit labels when never called", func() {
			trait := defkit.NewTrait("no-labels").
				Description("Trait without labels").
				AppliesTo("deployments.apps")

			cue := trait.ToCue()
			Expect(cue).NotTo(ContainSubstring("labels:"))
		})
	})

	Context("WorkloadRefPath attribute", func() {
		It("should emit workloadRefPath when set to empty string", func() {
			trait := defkit.NewTrait("wlref-empty").
				Description("Trait with empty workloadRefPath").
				AppliesTo("deployments.apps").
				WorkloadRefPath("")

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring(`workloadRefPath: ""`))
		})

		It("should emit workloadRefPath when set to a path", func() {
			trait := defkit.NewTrait("wlref-path").
				Description("Trait with workloadRefPath").
				AppliesTo("deployments.apps").
				WorkloadRefPath("spec.workloadRef")

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring(`workloadRefPath: "spec.workloadRef"`))
		})

		It("should not emit workloadRefPath when never called", func() {
			trait := defkit.NewTrait("no-wlref").
				Description("Trait without wlref path").
				AppliesTo("deployments.apps")

			cue := trait.ToCue()
			Expect(cue).NotTo(ContainSubstring("workloadRefPath"))
		})
	})

	Context("PatchContainer ParamsTypeName", func() {
		It("should use default PatchParams when ParamsTypeName is empty", func() {
			trait := defkit.NewTrait("default-params-name").
				Description("Test default params type name").
				AppliesTo("deployments.apps").
				PodDisruptive(true).
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						PatchFields: defkit.PatchFields(
							defkit.PatchField("image").Strategy("retainKeys"),
						),
					})
				})

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring("#PatchParams: {"))
			Expect(cue).To(ContainSubstring("_params:         #PatchParams"))
			Expect(cue).NotTo(ContainSubstring("#StartupProbeParams"))
		})

		It("should use custom ParamsTypeName when set", func() {
			trait := defkit.NewTrait("custom-params-name").
				Description("Test custom params type name").
				AppliesTo("deployments.apps").
				PodDisruptive(true).
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						AllowMultiple:        true,
						MultiContainerParam:  "probes",
						ParamsTypeName:       "StartupProbeParams",
						CustomParamsBlock:    "initialDelaySeconds: *0 | int",
						PatchFields: defkit.PatchFields(
							defkit.PatchField("initialDelaySeconds").Int().IsSet().Default("0"),
						),
					})
				})

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring("#StartupProbeParams: {"))
			Expect(cue).To(ContainSubstring("_params:         #StartupProbeParams"))
			Expect(cue).To(ContainSubstring("parameter: *#StartupProbeParams | close({"))
			Expect(cue).To(ContainSubstring("probes: [...#StartupProbeParams]"))
			Expect(cue).NotTo(ContainSubstring("#PatchParams"))
		})
	})

	Context("PatchContainer NoDefaultDisjunction", func() {
		It("should include * default marker by default", func() {
			trait := defkit.NewTrait("default-disjunction").
				Description("Test default disjunction").
				AppliesTo("deployments.apps").
				PodDisruptive(true).
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						AllowMultiple:        true,
						ContainersParam:      "containers",
						PatchFields: defkit.PatchFields(
							defkit.PatchField("image").Strategy("retainKeys"),
						),
					})
				})

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring("parameter: *#PatchParams | close({"))
		})

		It("should omit * default marker when NoDefaultDisjunction is true", func() {
			trait := defkit.NewTrait("no-default-disjunction").
				Description("Test no default disjunction").
				AppliesTo("deployments.apps").
				PodDisruptive(true).
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						AllowMultiple:        true,
						ContainersParam:      "containers",
						NoDefaultDisjunction: true,
						PatchFields: defkit.PatchFields(
							defkit.PatchField("image").Strategy("retainKeys"),
						),
					})
				})

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring("parameter: #PatchParams | close({"))
			Expect(cue).NotTo(ContainSubstring("parameter: *#PatchParams"))
		})
	})

	Context("PatchContainer no _baseContainer singular", func() {
		It("should use _baseContainers (plural) not _baseContainer (singular)", func() {
			trait := defkit.NewTrait("base-containers-test").
				Description("Test base containers plural").
				AppliesTo("deployments.apps").
				PodDisruptive(true).
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						PatchFields: defkit.PatchFields(
							defkit.PatchField("image"),
						),
					})
				})

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring("_baseContainers: context.output.spec.template.spec.containers"))
			Expect(cue).NotTo(ContainSubstring("_baseContainer:"))
		})
	})
})
