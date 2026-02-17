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
			Expect(cue).To(ContainSubstring(`attributes: {`))
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
			Expect(cue).To(ContainSubstring("parameter.parent != _|_"))
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
})
