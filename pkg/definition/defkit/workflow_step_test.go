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

var _ = Describe("WorkflowStepDefinition", func() {

	Context("Basic Builder Methods", func() {
		It("should create workflow step with name", func() {
			step := defkit.NewWorkflowStep("deploy")
			Expect(step.DefName()).To(Equal("deploy"))
			Expect(step.DefType()).To(Equal(defkit.DefinitionTypeWorkflowStep))
		})

		It("should set description", func() {
			step := defkit.NewWorkflowStep("deploy").
				Description("A powerful deploy step")
			Expect(step.GetDescription()).To(Equal("A powerful deploy step"))
		})

		It("should set category", func() {
			step := defkit.NewWorkflowStep("deploy").
				Category("Application Delivery")
			Expect(step.GetCategory()).To(Equal("Application Delivery"))
		})

		It("should set scope", func() {
			step := defkit.NewWorkflowStep("deploy").
				Scope("Application")
			Expect(step.GetScope()).To(Equal("Application"))
		})

		It("should add parameters using Params", func() {
			step := defkit.NewWorkflowStep("deploy").
				Params(
					defkit.Bool("auto").Default(true),
					defkit.Int("parallelism").Default(5),
				)
			Expect(step.GetParams()).To(HaveLen(2))
		})

		It("should add single parameter using Param", func() {
			step := defkit.NewWorkflowStep("deploy").
				Param(defkit.Bool("auto"))
			Expect(step.GetParams()).To(HaveLen(1))
		})
	})

	Context("Helper Method", func() {
		It("should add helper definition with param", func() {
			helperParam := defkit.Struct("placement").Fields(
				defkit.Field("clusterName", defkit.ParamTypeString),
			)
			step := defkit.NewWorkflowStep("deploy").
				Helper("Placement", helperParam)

			helpers := step.GetHelperDefinitions()
			Expect(helpers).To(HaveLen(1))
			Expect(helpers[0].GetName()).To(Equal("Placement"))
			Expect(helpers[0].HasParam()).To(BeTrue())
		})
	})

	Context("Status and Health Methods", func() {
		It("should set custom status", func() {
			step := defkit.NewWorkflowStep("deploy").
				CustomStatus("message: \"Deploying...\"")
			Expect(step.GetCustomStatus()).To(Equal("message: \"Deploying...\""))
		})

		It("should set health policy", func() {
			step := defkit.NewWorkflowStep("deploy").
				HealthPolicy("isHealth: true")
			Expect(step.GetHealthPolicy()).To(Equal("isHealth: true"))
		})

		It("should set health policy expression", func() {
			h := defkit.Health()
			step := defkit.NewWorkflowStep("deploy").
				HealthPolicyExpr(h.Condition("Ready").IsTrue())
			Expect(step.GetHealthPolicy()).NotTo(BeEmpty())
		})
	})

	Context("Imports", func() {
		It("should add imports with WithImports", func() {
			step := defkit.NewWorkflowStep("deploy").
				WithImports("vela/multicluster", "vela/builtin")
			Expect(step.GetImports()).To(ConsistOf("vela/multicluster", "vela/builtin"))
		})
	})

	Context("RawCUE", func() {
		It("should set raw CUE and bypass template generation", func() {
			rawCUE := `import (
	"vela/multicluster"
)

"deploy": {
	type: "workflow-step"
	description: "Raw CUE step"
}
template: {
	deploy: multicluster.#Deploy
	parameter: auto: *true | bool
}`
			step := defkit.NewWorkflowStep("deploy").RawCUE(rawCUE)
			Expect(step.ToCue()).To(Equal(rawCUE))
		})
	})

	Context("ToCue Generation", func() {
		It("should generate complete CUE definition", func() {
			step := defkit.NewWorkflowStep("deploy").
				Description("Deploy step").
				Category("Application Delivery").
				Scope("Application").
				Params(
					defkit.Bool("auto").Default(true),
					defkit.Int("parallelism").Default(5),
				)

			cue := step.ToCue()

			Expect(cue).To(ContainSubstring(`type: "workflow-step"`))
			Expect(cue).To(ContainSubstring(`annotations:`))
			Expect(cue).To(ContainSubstring(`"category": "Application Delivery"`))
			Expect(cue).To(ContainSubstring(`labels:`))
			Expect(cue).To(ContainSubstring(`"scope": "Application"`))
			Expect(cue).To(ContainSubstring(`template:`))
			Expect(cue).To(ContainSubstring(`parameter:`))
		})

		It("should include imports in CUE output", func() {
			step := defkit.NewWorkflowStep("deploy").
				Description("Deploy step").
				WithImports("vela/multicluster", "vela/builtin")

			cue := step.ToCue()

			Expect(cue).To(ContainSubstring(`import (`))
			Expect(cue).To(ContainSubstring(`"vela/multicluster"`))
			Expect(cue).To(ContainSubstring(`"vela/builtin"`))
		})
	})

	Context("ToYAML Generation", func() {
		It("should generate valid YAML manifest", func() {
			step := defkit.NewWorkflowStep("deploy").
				Description("Deploy components").
				Category("Application Delivery").
				Scope("Application").
				Params(defkit.Bool("auto").Default(true))

			yamlBytes, err := step.ToYAML()
			Expect(err).NotTo(HaveOccurred())

			yaml := string(yamlBytes)
			Expect(yaml).To(ContainSubstring("kind: WorkflowStepDefinition"))
			Expect(yaml).To(ContainSubstring("name: deploy"))
		})
	})

	Context("WorkflowStepTemplate", func() {
		It("should create template with Suspend action", func() {
			tpl := defkit.NewWorkflowStepTemplate()
			tpl.Suspend("Waiting for approval")

			Expect(tpl.GetSuspendMessage()).To(Equal("Waiting for approval"))
		})

		It("should create template with SuspendIf action", func() {
			auto := defkit.Bool("auto").Default(true)
			tpl := defkit.NewWorkflowStepTemplate()
			tpl.SuspendIf(defkit.Not(auto.IsTrue()), "Waiting for approval")

			Expect(tpl.GetActions()).To(HaveLen(1))
		})

		It("should create template with Builtin action", func() {
			policies := defkit.StringList("policies")
			parallelism := defkit.Int("parallelism").Default(5)

			tpl := defkit.NewWorkflowStepTemplate()
			tpl.Builtin("deploy", "multicluster.#Deploy").
				WithParams(map[string]defkit.Value{
					"policies":    policies,
					"parallelism": parallelism,
				}).Build()

			Expect(tpl.GetActions()).To(HaveLen(1))
		})
	})

	Context("Template with Actions", func() {
		It("should generate CUE with template actions", func() {
			auto := defkit.Bool("auto").Default(true)
			policies := defkit.StringList("policies")
			parallelism := defkit.Int("parallelism").Default(5)

			step := defkit.NewWorkflowStep("deploy").
				Description("Deploy step").
				WithImports("vela/multicluster", "vela/builtin").
				Params(auto, policies, parallelism).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.SuspendIf(defkit.Not(auto.IsTrue()), "Waiting approval to the deploy step")
					tpl.Builtin("deploy", "multicluster.#Deploy").
						WithParams(map[string]defkit.Value{
							"policies":    policies,
							"parallelism": parallelism,
						}).Build()
				})

			cue := step.ToCue()
			Expect(cue).To(ContainSubstring(`template:`))
		})

		It("should preserve explicit builtin action names", func() {
			step := defkit.NewWorkflowStep("apply-deployment").
				Description("Apply deployment with specified image and cmd.").
				WithImports("vela/kube", "vela/builtin").
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.Builtin("output", "kube.#Apply").
						WithParams(map[string]defkit.Value{
							"value": defkit.Reference("parameter.value"),
						}).
						Build()
					tpl.Builtin("wait", "builtin.#ConditionalWait").
						WithParams(map[string]defkit.Value{
							"continue": defkit.Reference("output.$returns.value.status.readyReplicas == parameter.replicas"),
						}).
						Build()
				})

			cue := step.ToCue()
			Expect(cue).To(ContainSubstring(`output: kube.#Apply & {`))
			Expect(cue).To(ContainSubstring(`wait: builtin.#ConditionalWait & {`))
			Expect(cue).NotTo(ContainSubstring(`apply: kube.#Apply & {`))
			Expect(cue).NotTo(ContainSubstring(`conditionalwait: builtin.#ConditionalWait & {`))
		})
	})

	Context("Registry", func() {
		BeforeEach(func() {
			defkit.Clear()
		})

		AfterEach(func() {
			defkit.Clear()
		})

		It("should register workflow steps", func() {
			step1 := defkit.NewWorkflowStep("deploy").Description("Deploy").Category("Application Delivery")
			step2 := defkit.NewWorkflowStep("suspend").Description("Suspend").Category("Workflow Control")
			comp := defkit.NewComponent("webservice").Description("Component")

			defkit.Register(step1)
			defkit.Register(step2)
			defkit.Register(comp)

			Expect(defkit.Count()).To(Equal(3))
			Expect(defkit.WorkflowSteps()).To(HaveLen(2))
		})
	})
})
