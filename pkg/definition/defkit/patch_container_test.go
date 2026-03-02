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

var _ = Describe("PatchContainer", func() {

	Context("PatchFieldBuilder", func() {
		It("should set ParamName and default TargetField to the same name", func() {
			f := defkit.PatchField("exec").Build()
			Expect(f.ParamName).To(Equal("exec"))
			Expect(f.TargetField).To(Equal("exec"))
			Expect(f.Condition).To(BeEmpty())
			Expect(f.ParamType).To(BeEmpty())
			Expect(f.ParamDefault).To(BeEmpty())
			Expect(f.PatchStrategy).To(BeEmpty())
			Expect(f.Description).To(BeEmpty())
		})

		It("should override TargetField with Target()", func() {
			f := defkit.PatchField("addCapabilities").Target("add").Build()
			Expect(f.ParamName).To(Equal("addCapabilities"))
			Expect(f.TargetField).To(Equal("add"))
		})

		It("should set Condition via IsSet()", func() {
			f := defkit.PatchField("exec").IsSet().Build()
			Expect(f.Condition).To(Equal("!= _|_"))
		})

		It("should set ParamDefault via Default()", func() {
			f := defkit.PatchField("initialDelaySeconds").Default("0").Build()
			Expect(f.ParamDefault).To(Equal("0"))
		})

		It("should set ParamType via Int(), Bool(), Str(), StringArray()", func() {
			Expect(defkit.PatchField("x").Int().Build().ParamType).To(Equal("int"))
			Expect(defkit.PatchField("x").Bool().Build().ParamType).To(Equal("bool"))
			Expect(defkit.PatchField("x").Str().Build().ParamType).To(Equal("string"))
			Expect(defkit.PatchField("x").StringArray().Build().ParamType).To(Equal("[...string]"))
		})

		It("should set PatchStrategy via Strategy()", func() {
			f := defkit.PatchField("image").Strategy("retainKeys").Build()
			Expect(f.PatchStrategy).To(Equal("retainKeys"))
		})

		It("should set Condition via NotEmpty()", func() {
			f := defkit.PatchField("imagePullPolicy").NotEmpty().Build()
			Expect(f.Condition).To(Equal(`!= ""`))
		})

		It("should set Condition via comparison methods", func() {
			Expect(defkit.PatchField("x").Gt("0").Build().Condition).To(Equal("> 0"))
			Expect(defkit.PatchField("x").Gte("1").Build().Condition).To(Equal(">= 1"))
			Expect(defkit.PatchField("x").Lt("100").Build().Condition).To(Equal("< 100"))
			Expect(defkit.PatchField("x").Lte("99").Build().Condition).To(Equal("<= 99"))
			Expect(defkit.PatchField("x").Eq("42").Build().Condition).To(Equal("== 42"))
			Expect(defkit.PatchField("x").Ne("0").Build().Condition).To(Equal("!= 0"))
		})

		It("should set Condition via RawCondition escape hatch", func() {
			f := defkit.PatchField("x").RawCondition(`!= "custom"`).Build()
			Expect(f.Condition).To(Equal(`!= "custom"`))
		})

		It("should set Description via Description()", func() {
			f := defkit.PatchField("image").Description("Specify the image").Build()
			Expect(f.Description).To(Equal("Specify the image"))
		})

		It("should chain all methods together", func() {
			f := defkit.PatchField("initialDelaySeconds").
				Int().
				IsSet().
				Default("0").
				Description("Seconds before probe starts").
				Build()
			Expect(f.ParamName).To(Equal("initialDelaySeconds"))
			Expect(f.TargetField).To(Equal("initialDelaySeconds"))
			Expect(f.ParamType).To(Equal("int"))
			Expect(f.Condition).To(Equal("!= _|_"))
			Expect(f.ParamDefault).To(Equal("0"))
			Expect(f.Description).To(Equal("Seconds before probe starts"))
		})

		It("should produce struct equivalent to manual construction", func() {
			builder := defkit.PatchField("addCapabilities").
				Target("add").
				StringArray().
				IsSet().
				Description("Specify the addCapabilities of the container").
				Build()

			manual := defkit.PatchContainerField{
				ParamName:   "addCapabilities",
				TargetField: "add",
				ParamType:   "[...string]",
				Condition:   "!= _|_",
				Description: "Specify the addCapabilities of the container",
			}

			Expect(builder).To(Equal(manual))
		})

		It("PatchFields should batch-build without .Build()", func() {
			fields := defkit.PatchFields(
				defkit.PatchField("exec").IsSet(),
				defkit.PatchField("delay").Int().IsSet().Default("0"),
			)
			Expect(fields).To(HaveLen(2))
			Expect(fields[0].ParamName).To(Equal("exec"))
			Expect(fields[0].Condition).To(Equal("!= _|_"))
			Expect(fields[1].ParamName).To(Equal("delay"))
			Expect(fields[1].ParamType).To(Equal("int"))
			Expect(fields[1].ParamDefault).To(Equal("0"))
		})

		It("PatchFields should return empty slice for zero builders", func() {
			fields := defkit.PatchFields()
			Expect(fields).To(HaveLen(0))
			Expect(fields).NotTo(BeNil())
		})

		It("should set ParamType via Type() for custom CUE types", func() {
			f := defkit.PatchField("metadata").Type("{...}").Build()
			Expect(f.ParamType).To(Equal("{...}"))
		})

		It("last condition method call should win", func() {
			f := defkit.PatchField("x").IsSet().NotEmpty().Build()
			Expect(f.Condition).To(Equal(`!= ""`))

			f2 := defkit.PatchField("x").NotEmpty().IsSet().Build()
			Expect(f2.Condition).To(Equal("!= _|_"))
		})
	})

	Context("CUE generation with PatchFieldBuilder", func() {
		It("should produce identical CUE from builder and manual struct construction", func() {
			// Build the same trait using builder API
			builderTrait := defkit.NewTrait("builder-cue-test").
				Description("Test builder produces same CUE as manual").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						Groups: []defkit.PatchContainerGroup{
							{
								TargetField: "securityContext",
								Fields: defkit.PatchFields(
									defkit.PatchField("privileged").Bool().Default("false"),
									defkit.PatchField("runAsUser").Int().IsSet(),
								),
								SubGroups: []defkit.PatchContainerGroup{
									{
										TargetField: "capabilities",
										Fields: defkit.PatchFields(
											defkit.PatchField("addCapabilities").Target("add").StringArray().IsSet(),
										),
									},
								},
							},
						},
					})
				})

			// Build the same trait using manual struct construction
			manualTrait := defkit.NewTrait("builder-cue-test").
				Description("Test builder produces same CUE as manual").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						Groups: []defkit.PatchContainerGroup{
							{
								TargetField: "securityContext",
								Fields: []defkit.PatchContainerField{
									{ParamName: "privileged", TargetField: "privileged", ParamType: "bool", ParamDefault: "false"},
									{ParamName: "runAsUser", TargetField: "runAsUser", ParamType: "int", Condition: "!= _|_"},
								},
								SubGroups: []defkit.PatchContainerGroup{
									{
										TargetField: "capabilities",
										Fields: []defkit.PatchContainerField{
											{ParamName: "addCapabilities", TargetField: "add", ParamType: "[...string]", Condition: "!= _|_"},
										},
									},
								},
							},
						},
					})
				})

			Expect(builderTrait.ToCue()).To(Equal(manualTrait.ToCue()))
		})

		It("should generate correct CUE for PatchFields with Strategy and NotEmpty", func() {
			trait := defkit.NewTrait("builder-strategy-test").
				Description("Test Strategy and NotEmpty via builder").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						PatchFields: defkit.PatchFields(
							defkit.PatchField("image").Strategy("retainKeys"),
							defkit.PatchField("imagePullPolicy").Strategy("retainKeys").NotEmpty(),
						),
					})
				})

			cue := trait.ToCue()

			// Strategy should produce patchStrategy comments
			Expect(cue).To(ContainSubstring(`// +patchStrategy=retainKeys`))
			// NotEmpty() should produce conditional block in PatchContainer body
			Expect(cue).To(ContainSubstring(`if _params.imagePullPolicy != ""`))
			// Unconditional field should be assigned directly
			Expect(cue).To(ContainSubstring(`image: _params.image`))
		})

		It("should generate correct CUE for IsSet fields in groups", func() {
			trait := defkit.NewTrait("builder-isset-group-test").
				Description("Test IsSet in groups via builder").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						Groups: []defkit.PatchContainerGroup{
							{
								TargetField: "startupProbe",
								Fields: defkit.PatchFields(
									defkit.PatchField("exec").Str().IsSet(),
									defkit.PatchField("initialDelaySeconds").Int().IsSet().Default("0"),
									defkit.PatchField("terminationGracePeriodSeconds").Int().IsSet(),
								),
							},
						},
					})
				})

			cue := trait.ToCue()

			// Str().IsSet() → optional string param and conditional in PatchContainer body
			Expect(cue).To(ContainSubstring(`exec?: string`))
			Expect(cue).To(ContainSubstring(`if _params.exec != _|_`))

			// Int().IsSet() → optional int in param schema and conditional in body
			Expect(cue).To(ContainSubstring(`terminationGracePeriodSeconds?: int`))
			Expect(cue).To(ContainSubstring(`if _params.terminationGracePeriodSeconds != _|_`))

			// Int().IsSet().Default("0") → default in param schema but still conditional in body
			Expect(cue).To(ContainSubstring(`initialDelaySeconds: *0 | int`))
			Expect(cue).To(ContainSubstring(`if _params.initialDelaySeconds != _|_`))

			// startupProbe group wrapper
			Expect(cue).To(ContainSubstring(`startupProbe: {`))
		})

		It("should generate correct CUE for Default without IsSet (unconditional)", func() {
			trait := defkit.NewTrait("builder-default-only-test").
				Description("Test Default without IsSet via builder").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						Groups: []defkit.PatchContainerGroup{
							{
								TargetField: "securityContext",
								Fields: defkit.PatchFields(
									defkit.PatchField("privileged").Bool().Default("false"),
									defkit.PatchField("runAsUser").Int().IsSet(),
								),
							},
						},
					})
				})

			cue := trait.ToCue()

			// Bool().Default("false") → default in param, unconditional in PatchContainer body
			Expect(cue).To(ContainSubstring(`privileged: *false | bool`))
			Expect(cue).To(ContainSubstring(`privileged: _params.privileged`))
			Expect(cue).NotTo(ContainSubstring(`if _params.privileged`))

			// Int().IsSet() → optional param, conditional in PatchContainer body
			Expect(cue).To(ContainSubstring(`runAsUser?: int`))
			Expect(cue).To(ContainSubstring(`if _params.runAsUser != _|_`))
		})

		It("should generate correct CUE for Target remapping in subgroups", func() {
			trait := defkit.NewTrait("builder-target-test").
				Description("Test Target remapping via builder").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						Groups: []defkit.PatchContainerGroup{
							{
								TargetField: "securityContext",
								Fields: defkit.PatchFields(
									defkit.PatchField("privileged").Bool().Default("false"),
								),
								SubGroups: []defkit.PatchContainerGroup{
									{
										TargetField: "capabilities",
										Fields: defkit.PatchFields(
											defkit.PatchField("addCapabilities").Target("add").StringArray().IsSet(),
											defkit.PatchField("dropCapabilities").Target("drop").StringArray().IsSet(),
										),
									},
								},
							},
						},
					})
				})

			cue := trait.ToCue()

			// Target("add") remaps param name to different container field
			Expect(cue).To(ContainSubstring(`addCapabilities?: [...string]`))
			Expect(cue).To(ContainSubstring(`add: _params.addCapabilities`))

			// Target("drop") remaps param name to different container field
			Expect(cue).To(ContainSubstring(`dropCapabilities?: [...string]`))
			Expect(cue).To(ContainSubstring(`drop: _params.dropCapabilities`))

			// Nested group structure
			Expect(cue).To(ContainSubstring(`securityContext: {`))
			Expect(cue).To(ContainSubstring(`capabilities: {`))
		})

		It("should generate correct CUE for Description on builder fields", func() {
			trait := defkit.NewTrait("builder-desc-test").
				Description("Test Description via builder").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						PatchFields: defkit.PatchFields(
							defkit.PatchField("image").Strategy("retainKeys").Description("Specify the image of the container"),
							defkit.PatchField("imagePullPolicy").Strategy("retainKeys").NotEmpty().Description("Specify the image pull policy"),
						),
					})
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("// +usage=Specify the image of the container"))
			Expect(cue).To(ContainSubstring("// +usage=Specify the image pull policy"))
		})
	})

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

			// Verify parameter block comes from PatchContainer (no duplicate from regular params)
			Expect(cue).To(ContainSubstring(`parameter: #PatchParams`))
			// The extra parameter: {} from regular params should NOT appear
			Expect(strings.Count(cue, "parameter:")).To(Equal(1))
		})

		It("should use optional field syntax for non-string conditions like != _|_", func() {
			trait := defkit.NewTrait("optional-field-test").
				Description("Test optional fields for non-string conditions").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						Groups: []defkit.PatchContainerGroup{
							{
								TargetField: "securityContext",
								Fields: []defkit.PatchContainerField{
									{ParamName: "privileged", TargetField: "privileged", ParamType: "bool", ParamDefault: "false"},
									{ParamName: "runAsUser", TargetField: "runAsUser", ParamType: "int", Condition: "!= _|_"},
									{ParamName: "runAsGroup", TargetField: "runAsGroup", ParamType: "int", Condition: "!= _|_"},
								},
								SubGroups: []defkit.PatchContainerGroup{
									{
										TargetField: "capabilities",
										Fields: []defkit.PatchContainerField{
											{ParamName: "addCapabilities", TargetField: "add", ParamType: "[...string]", Condition: "!= _|_"},
											{ParamName: "dropCapabilities", TargetField: "drop", ParamType: "[...string]", Condition: "!= _|_"},
										},
									},
								},
							},
						},
					})
				})

			cue := trait.ToCue()

			// Fields with != _|_ condition should use optional syntax (field?: type), not *null | type
			Expect(cue).To(ContainSubstring(`runAsUser?: int`))
			Expect(cue).To(ContainSubstring(`runAsGroup?: int`))
			Expect(cue).To(ContainSubstring(`addCapabilities?: [...string]`))
			Expect(cue).To(ContainSubstring(`dropCapabilities?: [...string]`))

			// Must NOT have *null | type for these fields
			Expect(cue).NotTo(ContainSubstring(`runAsUser: *null | int`))
			Expect(cue).NotTo(ContainSubstring(`runAsGroup: *null | int`))
			Expect(cue).NotTo(ContainSubstring(`addCapabilities: *null | [...string]`))
			Expect(cue).NotTo(ContainSubstring(`dropCapabilities: *null | [...string]`))

			// Fields with explicit defaults should still use default syntax
			Expect(cue).To(ContainSubstring(`privileged: *false | bool`))

			// The PatchContainer body should still have conditional blocks for these fields
			Expect(cue).To(ContainSubstring(`if _params.runAsUser != _|_`))
			Expect(cue).To(ContainSubstring(`if _params.runAsGroup != _|_`))
			Expect(cue).To(ContainSubstring(`if _params.addCapabilities != _|_`))
			Expect(cue).To(ContainSubstring(`if _params.dropCapabilities != _|_`))
		})

		It("should use *empty-string default for string-equality conditions", func() {
			trait := defkit.NewTrait("image-test").
				Description("Test string-equality condition default").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						PatchFields: []defkit.PatchContainerField{
							{ParamName: "image", TargetField: "image", PatchStrategy: "retainKeys"},
							{ParamName: "imagePullPolicy", TargetField: "imagePullPolicy", PatchStrategy: "retainKeys", Condition: `!= ""`},
						},
					})
				})

			cue := trait.ToCue()

			// String-equality condition should default to empty string, not null
			Expect(cue).To(ContainSubstring(`imagePullPolicy: *"" |`))
			Expect(cue).NotTo(ContainSubstring(`imagePullPolicy: *null |`))
		})

		It("should map params unconditionally in single-container _params block", func() {
			trait := defkit.NewTrait("unconditional-test").
				Description("Test unconditional param mapping").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						AllowMultiple:        true,
						ContainersParam:      "containers",
						PatchFields: []defkit.PatchContainerField{
							{ParamName: "image", TargetField: "image"},
							{ParamName: "imagePullPolicy", TargetField: "imagePullPolicy", Condition: `!= ""`},
						},
					})
				})

			cue := trait.ToCue()

			// In the single-container _params block, all fields should be mapped unconditionally
			Expect(cue).To(ContainSubstring("image:           parameter.image"))
			Expect(cue).To(ContainSubstring("imagePullPolicy: parameter.imagePullPolicy"))
			// The conditional should NOT wrap the param mapping in the _params block
			Expect(cue).NotTo(MatchRegexp(`if parameter\.imagePullPolicy != ""[^}]*\n[^}]*imagePullPolicy: parameter\.imagePullPolicy`))
			// But the PatchContainer body should still have the condition
			Expect(cue).To(ContainSubstring(`if _params.imagePullPolicy != ""`))
		})

		It("should emit *#PatchParams with star default marker in multi-container parameter block", func() {
			trait := defkit.NewTrait("star-test").
				Description("Test star in parameter").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						AllowMultiple:        true,
						ContainersParam:      "containers",
						PatchFields: []defkit.PatchContainerField{
							{ParamName: "image", TargetField: "image"},
						},
					})
				})

			cue := trait.ToCue()

			// Should be *#PatchParams with star (marks single-container as default branch)
			Expect(cue).To(ContainSubstring("parameter: *#PatchParams | close({"))
		})

		It("should use custom Description and ContainersDescription", func() {
			trait := defkit.NewTrait("desc-test").
				Description("Test custom descriptions").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:    "containerName",
						DefaultToContextName:  true,
						AllowMultiple:         true,
						ContainersParam:       "containers",
						ContainersDescription: "Specify the container image for multiple containers",
						PatchFields: []defkit.PatchContainerField{
							{ParamName: "image", TargetField: "image", Description: "Specify the image of the container"},
							{ParamName: "imagePullPolicy", TargetField: "imagePullPolicy", Condition: `!= ""`, Description: "Specify the image pull policy of the container"},
						},
					})
				})

			cue := trait.ToCue()

			// Custom field descriptions
			Expect(cue).To(ContainSubstring("// +usage=Specify the image of the container"))
			Expect(cue).To(ContainSubstring("// +usage=Specify the image pull policy of the container"))
			// Custom containers description
			Expect(cue).To(ContainSubstring("// +usage=Specify the container image for multiple containers"))
		})

		It("should not emit duplicate parameter: {} when using PatchContainer", func() {
			trait := defkit.NewTrait("no-dup-param-test").
				Description("Test no duplicate parameter block").
				AppliesTo("deployments.apps").
				Params(defkit.String("image").Required()).
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						PatchFields: []defkit.PatchContainerField{
							{ParamName: "image", TargetField: "image"},
						},
					})
				})

			cue := trait.ToCue()

			// Only one parameter: line should appear (from PatchContainer)
			Expect(strings.Count(cue, "parameter:")).To(Equal(1))
			// Should not have parameter: {}
			Expect(cue).NotTo(ContainSubstring("parameter: {}"))
		})
	})

	Context("MultiContainerCheckField and MultiContainerErrMsg", func() {
		It("should use default check field 'containerName' and default error message with camelCase", func() {
			trait := defkit.NewTrait("default-multi-err-test").
				Description("Test default multi-container error").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						AllowMultiple:        true,
						ContainersParam:      "containers",
						PatchFields: []defkit.PatchContainerField{
							{ParamName: "image", TargetField: "image"},
						},
					})
				})

			cue := trait.ToCue()

			// Default check field is "containerName"
			Expect(cue).To(ContainSubstring(`if c.containerName == ""`))
			Expect(cue).To(ContainSubstring(`if c.containerName != ""`))
			// Default error message uses "containerName" (camelCase, matching the field name)
			Expect(cue).To(ContainSubstring(`err: "containerName must be set for containers"`))
		})

		It("should use custom MultiContainerCheckField when set", func() {
			trait := defkit.NewTrait("custom-check-field-test").
				Description("Test custom check field").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:       "containerName",
						DefaultToContextName:     true,
						AllowMultiple:            true,
						MultiContainerParam:      "probes",
						MultiContainerCheckField: "name",
						PatchFields: []defkit.PatchContainerField{
							{ParamName: "image", TargetField: "image"},
						},
					})
				})

			cue := trait.ToCue()

			// Custom check field "name" instead of default "containerName"
			Expect(cue).To(ContainSubstring(`if c.name == ""`))
			Expect(cue).To(ContainSubstring(`if c.name != ""`))
			Expect(cue).NotTo(ContainSubstring(`c.containerName == ""`))
		})

		It("should use custom MultiContainerErrMsg when set", func() {
			trait := defkit.NewTrait("custom-err-msg-test").
				Description("Test custom error message").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:       "containerName",
						DefaultToContextName:     true,
						AllowMultiple:            true,
						MultiContainerParam:      "probes",
						MultiContainerCheckField: "name",
						MultiContainerErrMsg:     "containerName must be set when specifying startup probe for multiple containers",
						PatchFields: []defkit.PatchContainerField{
							{ParamName: "image", TargetField: "image"},
						},
					})
				})

			cue := trait.ToCue()

			// Custom error message overrides the default
			Expect(cue).To(ContainSubstring(`err: "containerName must be set when specifying startup probe for multiple containers"`))
			// Default error message should NOT appear
			Expect(cue).NotTo(ContainSubstring(`err: "containerName must be set for probes"`))
		})

		It("should use default error with MultiContainerParam name", func() {
			trait := defkit.NewTrait("multi-param-err-test").
				Description("Test error message includes multi param name").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						AllowMultiple:        true,
						MultiContainerParam:  "probes",
						PatchFields: []defkit.PatchContainerField{
							{ParamName: "image", TargetField: "image"},
						},
					})
				})

			cue := trait.ToCue()

			// Default error message should include the multi-container param name
			Expect(cue).To(ContainSubstring(`err: "containerName must be set for probes"`))
		})
	})

	Context("writePatchParamMapping typed scalar fields", func() {
		It("should map typed scalar fields unconditionally in _params block even with IsSet and no default", func() {
			trait := defkit.NewTrait("typed-scalar-unconditional-test").
				Description("Test typed scalar fields pass through unconditionally").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						AllowMultiple:        true,
						ContainersParam:      "containers",
						Groups: []defkit.PatchContainerGroup{
							{
								TargetField: "startupProbe",
								Fields: defkit.PatchFields(
									defkit.PatchField("terminationGracePeriodSeconds").Int().IsSet(),
									defkit.PatchField("exec").IsSet(),
								),
							},
						},
					})
				})

			cue := trait.ToCue()

			// Int().IsSet() with no default: typed scalar should be unconditional in _params block
			Expect(cue).To(ContainSubstring("terminationGracePeriodSeconds: parameter.terminationGracePeriodSeconds"))
			// But untyped IsSet() with no default (e.g., exec) should remain conditional in _params block
			Expect(cue).To(MatchRegexp(`if parameter\.exec != _\|_\s*\{[^}]*exec:\s*parameter\.exec`))

			// Both should still be conditional in the PatchContainer body
			Expect(cue).To(ContainSubstring("if _params.terminationGracePeriodSeconds != _|_"))
			Expect(cue).To(ContainSubstring("if _params.exec != _|_"))
		})

		It("should keep untyped IsSet fields conditional in _params block", func() {
			trait := defkit.NewTrait("untyped-conditional-test").
				Description("Test untyped fields remain conditional").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						AllowMultiple:        true,
						ContainersParam:      "containers",
						Groups: []defkit.PatchContainerGroup{
							{
								TargetField: "startupProbe",
								Fields: defkit.PatchFields(
									defkit.PatchField("exec").IsSet(),
									defkit.PatchField("httpGet").IsSet(),
								),
							},
						},
					})
				})

			cue := trait.ToCue()

			// Untyped IsSet() fields should still be conditional in _params block
			Expect(cue).To(MatchRegexp(`if parameter\.exec != _\|_\s*\{[^}]*exec:\s*parameter\.exec`))
			Expect(cue).To(MatchRegexp(`if parameter\.httpGet != _\|_\s*\{[^}]*httpGet:\s*parameter\.httpGet`))
		})

		It("should map typed scalar fields with default unconditionally regardless of IsSet", func() {
			trait := defkit.NewTrait("typed-with-default-test").
				Description("Test typed fields with default are unconditional").
				AppliesTo("deployments.apps").
				Template(func(tpl *defkit.Template) {
					tpl.UsePatchContainer(defkit.PatchContainerConfig{
						ContainerNameParam:   "containerName",
						DefaultToContextName: true,
						AllowMultiple:        true,
						ContainersParam:      "containers",
						Groups: []defkit.PatchContainerGroup{
							{
								TargetField: "startupProbe",
								Fields: defkit.PatchFields(
									defkit.PatchField("initialDelaySeconds").Int().IsSet().Default("0"),
									defkit.PatchField("periodSeconds").Int().IsSet().Default("10"),
								),
							},
						},
					})
				})

			cue := trait.ToCue()

			// Typed fields with defaults are always unconditional in _params block
			Expect(cue).To(ContainSubstring("initialDelaySeconds: parameter.initialDelaySeconds"))
			Expect(cue).To(ContainSubstring("periodSeconds:       parameter.periodSeconds"))
			// Should NOT be wrapped in conditionals in _params block
			Expect(cue).NotTo(MatchRegexp(`if parameter\.initialDelaySeconds[^{]*\{[^}]*initialDelaySeconds:\s*parameter\.initialDelaySeconds`))
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
