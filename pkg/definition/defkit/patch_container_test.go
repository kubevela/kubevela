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

var _ = Describe("PatchContainer", func() {

	Context("Trait with PatchContainer CUE Generation", func() {
		It("should generate complete PatchContainer trait CUE structure", func() {
			containerName := defkit.String("containerName").Default("")
			command := defkit.StringList("command")
			args := defkit.StringList("args")

			trait := defkit.NewTrait("command").
				Description("Override container command and args").
				AppliesTo("deployments.apps", "statefulsets.apps").
				Params(containerName, command, args).
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						PatchFields: []defkit.PatchContainerField{
							{ParamName: "command", TargetField: "command", PatchStrategy: "replace"},
							{ParamName: "args", TargetField: "args", PatchStrategy: "replace"},
						},
					})
				})

			cue := trait.ToCue()

			// Verify trait metadata block
			Expect(cue).To(ContainSubstring(`command: {`))
			Expect(cue).To(ContainSubstring(`type: "trait"`))
			Expect(cue).To(ContainSubstring(`description: "Override container command and args"`))
			Expect(cue).To(ContainSubstring(`appliesToWorkloads: ["deployments.apps", "statefulsets.apps"]`))

			// Verify #PatchParams definition with all fields
			Expect(cue).To(ContainSubstring(`#PatchParams: {`))
			Expect(cue).To(ContainSubstring(`containerName: *"" | string`))
			Expect(cue).To(ContainSubstring(`command: [...string]`))
			Expect(cue).To(ContainSubstring(`args: [...string]`))

			// Verify PatchContainer helper structure
			Expect(cue).To(ContainSubstring(`PatchContainer: {`))
			Expect(cue).To(ContainSubstring(`_params:         #PatchParams`))
			Expect(cue).To(ContainSubstring(`name:            _params.containerName`))
			Expect(cue).To(ContainSubstring(`_baseContainers: context.output.spec.template.spec.containers`))
			Expect(cue).To(ContainSubstring(`_matchContainers_: [for _container_ in _baseContainers if _container_.name == name {_container_}]`))

			// Verify error handling for container not found
			Expect(cue).To(ContainSubstring(`if len(_matchContainers_) == 0 {`))
			Expect(cue).To(ContainSubstring(`err: "container \(name) not found"`))

			// Verify patch fields with strategy comments
			Expect(cue).To(ContainSubstring(`// +patchStrategy=replace`))
			Expect(cue).To(ContainSubstring(`command: _params.command`))
			Expect(cue).To(ContainSubstring(`args: _params.args`))

			// Verify patch structure with patchKey annotation
			Expect(cue).To(ContainSubstring(`patch: spec: template: spec: {`))
			Expect(cue).To(ContainSubstring(`// +patchKey=name`))
			Expect(cue).To(ContainSubstring(`containers: [{`))

			// Verify default-to-context-name logic
			Expect(cue).To(ContainSubstring(`if parameter.containerName == "" {`))
			Expect(cue).To(ContainSubstring(`containerName: context.name`))
			Expect(cue).To(ContainSubstring(`if parameter.containerName != "" {`))
			Expect(cue).To(ContainSubstring(`containerName: parameter.containerName`))

			// Verify error collection
			Expect(cue).To(ContainSubstring(`errs: [for c in patch.spec.template.spec.containers if c.err != _|_ {c.err}]`))

			// Verify parameter block with optional fields
			Expect(cue).To(ContainSubstring(`parameter: {`))
			Expect(cue).To(ContainSubstring(`command?: [...string]`))
			Expect(cue).To(ContainSubstring(`args?: [...string]`))
		})
	})

	Context("Template Let Bindings", func() {
		It("should accumulate bindings in order for CUE generation", func() {
			tpl := defkit.NewTemplate()

			expr1 := defkit.Lit(100)
			expr2 := defkit.Struct("config")
			expr3 := defkit.List("items")

			tpl.AddLetBinding("count", expr1)
			tpl.AddLetBinding("cfg", expr2)
			tpl.AddLetBinding("items", expr3)

			bindings := tpl.GetLetBindings()
			Expect(bindings).To(HaveLen(3))
			Expect(bindings[0].Name()).To(Equal("count"))
			Expect(bindings[0].Expr()).To(Equal(expr1))
			Expect(bindings[1].Name()).To(Equal("cfg"))
			Expect(bindings[1].Expr()).To(Equal(expr2))
			Expect(bindings[2].Name()).To(Equal("items"))
			Expect(bindings[2].Expr()).To(Equal(expr3))
		})

		It("should return nil when no bindings added", func() {
			tpl := defkit.NewTemplate()
			Expect(tpl.GetLetBindings()).To(BeNil())
		})
	})

	Context("LetVariable for CUE let binding references", func() {
		It("should create referenceable variable usable as Value", func() {
			ref := defkit.LetVariable("resourceContent")

			// Name should match the let binding name
			Expect(ref.Name()).To(Equal("resourceContent"))

			// Should be usable as a Value in template operations
			var v defkit.Value = ref
			Expect(v).NotTo(BeNil())
		})
	})

	Context("ListComprehension for CUE for-each generation", func() {
		It("should capture all components needed for CUE comprehension syntax", func() {
			source := defkit.ParamRef("constraints")
			filter := defkit.ListFieldExists("enabled")
			mappings := defkit.FieldMap{
				"maxSkew":    defkit.FieldRef("maxSkew"),
				"minDomains": defkit.Optional("minDomains"),
			}

			comp := defkit.ForEachIn(source).
				WithFilter(filter).
				MapFields(mappings).
				WithOptionalFields("labelSelector", "topologyKey")

			// All components should be captured for CUE generation
			Expect(comp.Source()).To(Equal(source))
			Expect(comp.FilterCondition()).To(Equal(filter))
			Expect(comp.Mappings()).To(Equal(mappings))
			Expect(comp.ConditionalFields()).To(Equal([]string{"labelSelector", "topologyKey"}))
		})
	})

	Context("ListFieldExists predicate", func() {
		It("should store field name for CUE != _|_ check generation", func() {
			pred := defkit.ListFieldExists("optionalField")

			Expect(pred.GetField()).To(Equal("optionalField"))
		})
	})

	Context("Template Raw CUE Blocks for complex patterns", func() {
		It("should store raw blocks for direct CUE injection", func() {
			tpl := defkit.NewTemplate()

			headerBlock := `let _containers = context.output.spec.template.spec.containers`
			patchBlock := `spec: template: spec: containers: [for c in _containers { c & _patch }]`
			paramBlock := `parameter: { debug: *false | bool }`
			outputsBlock := `outputs: service: { apiVersion: "v1", kind: "Service" }`

			tpl.SetRawHeaderBlock(headerBlock)
			tpl.SetRawPatchBlock(patchBlock)
			tpl.SetRawParameterBlock(paramBlock)
			tpl.SetRawOutputsBlock(outputsBlock)

			Expect(tpl.GetRawHeaderBlock()).To(Equal(headerBlock))
			Expect(tpl.GetRawPatchBlock()).To(Equal(patchBlock))
			Expect(tpl.GetRawParameterBlock()).To(Equal(paramBlock))
			Expect(tpl.GetRawOutputsBlock()).To(Equal(outputsBlock))
		})

		It("should return empty strings when blocks not set", func() {
			tpl := defkit.NewTemplate()

			Expect(tpl.GetRawHeaderBlock()).To(BeEmpty())
			Expect(tpl.GetRawPatchBlock()).To(BeEmpty())
			Expect(tpl.GetRawParameterBlock()).To(BeEmpty())
			Expect(tpl.GetRawOutputsBlock()).To(BeEmpty())
		})
	})

	Context("Condition types for CUE conditional generation", func() {
		It("ParamIsSet should store param name for CUE != _|_ generation", func() {
			cond := defkit.ParamIsSet("replicas")

			Expect(cond.ParamName()).To(Equal("replicas"))
		})

		It("ParamNotSet should store param name for CUE == _|_ generation", func() {
			cond := defkit.ParamNotSet("defaults")

			inner := cond.Inner()
			isSet, ok := inner.(*defkit.IsSetCondition)
			Expect(ok).To(BeTrue())
			Expect(isSet.ParamName()).To(Equal("defaults"))
		})

		It("ContextOutputExists should store path for context.output check", func() {
			cond := defkit.ContextOutputExists("spec.template.spec.containers")

			Expect(cond.Path()).To(Equal("spec.template.spec.containers"))
		})

		It("AllConditions should store all conditions for nested CUE if generation", func() {
			cond1 := defkit.ParamIsSet("cpu")
			cond2 := defkit.ParamIsSet("memory")
			cond3 := defkit.ParamNotSet("defaults")

			compound := defkit.AllConditions(cond1, cond2, cond3)

			conditions := compound.Conditions()
			Expect(conditions).To(HaveLen(3))
			Expect(conditions[0]).To(Equal(cond1))
			Expect(conditions[1]).To(Equal(cond2))
			Expect(conditions[2]).To(Equal(cond3))
		})
	})
})
