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
			helperParam := defkit.Struct("placement").WithFields(
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

		It("should generate separate if blocks for each SetIf operation", func() {
			data := defkit.Object("data")
			noData := defkit.Eq(defkit.ParamRef("data"), defkit.Reference("_|_"))
			hasData := defkit.PathExists("parameter.data")

			step := defkit.NewWorkflowStep("webhook").
				WithImports("vela/kube", "encoding/json").
				Params(data).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					dataValue := defkit.NewArrayElement().
						SetIf(noData, "read", defkit.Reference("kube.#Read & {}")).
						SetIf(noData, "value", defkit.Reference("json.Marshal(read.$returns.value)")).
						SetIf(hasData, "value", defkit.Reference("json.Marshal(parameter.data)"))
					tpl.Set("data", dataValue)
				})

			cue := step.ToCue()
			Expect(strings.Count(cue, "if parameter.data == _|_ {")).To(Equal(2))
			Expect(cue).To(ContainSubstring("read: kube.#Read & {}"))
			Expect(cue).To(ContainSubstring("value: json.Marshal(read.$returns.value)"))
		})
	})

	Context("Labels", func() {
		It("should set and get labels", func() {
			step := defkit.NewWorkflowStep("check-metrics").
				Labels(map[string]string{"catalog": "Delivery"})
			Expect(step.GetLabels()).To(HaveKeyWithValue("catalog", "Delivery"))
		})

		It("should render labels in CUE output", func() {
			step := defkit.NewWorkflowStep("check-metrics").
				Description("Verify metrics").
				Category("Application Delivery").
				Labels(map[string]string{"catalog": "Delivery"})

			cue := step.ToCue()
			Expect(cue).To(ContainSubstring(`"catalog": "Delivery"`))
		})

		It("should render multiple labels sorted alphabetically", func() {
			step := defkit.NewWorkflowStep("test").
				Description("Test").
				Labels(map[string]string{"z-label": "last", "a-label": "first"})

			cue := step.ToCue()
			aIdx := strings.Index(cue, `"a-label"`)
			zIdx := strings.Index(cue, `"z-label"`)
			Expect(aIdx).To(BeNumerically("<", zIdx))
		})
	})

	Context("TemplateBody", func() {
		It("should set and get raw template body", func() {
			step := defkit.NewWorkflowStep("test").
				TemplateBody(`check: metrics.#PromCheck & {}`)
			Expect(step.HasRawTemplateBody()).To(BeTrue())
			Expect(step.GetRawTemplateBody()).To(Equal(`check: metrics.#PromCheck & {}`))
		})

		It("should return false when no template body set", func() {
			step := defkit.NewWorkflowStep("test")
			Expect(step.HasRawTemplateBody()).To(BeFalse())
			Expect(step.GetRawTemplateBody()).To(BeEmpty())
		})

		It("should embed raw template body in generated CUE", func() {
			step := defkit.NewWorkflowStep("check-metrics").
				Description("Verify metrics").
				WithImports("vela/metrics").
				Params(defkit.String("query")).
				TemplateBody("check: metrics.#PromCheck & {\n\t$params: query: parameter.query\n}")

			cue := step.ToCue()
			Expect(cue).To(ContainSubstring("check: metrics.#PromCheck & {"))
			Expect(cue).To(ContainSubstring("$params: query: parameter.query"))
			Expect(cue).To(ContainSubstring("parameter:"))
			Expect(cue).To(ContainSubstring("query: string"))
		})

		It("should handle empty lines in template body", func() {
			step := defkit.NewWorkflowStep("test").
				Description("Test").
				TemplateBody("line1: true\n\nline2: false")

			cue := step.ToCue()
			Expect(cue).To(ContainSubstring("line1: true"))
			Expect(cue).To(ContainSubstring("line2: false"))
		})
	})

	Context("WithFullParameter", func() {
		It("should generate $params: parameter for builtin", func() {
			step := defkit.NewWorkflowStep("suspend").
				Description("Suspend workflow").
				Params(defkit.String("message").Optional()).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.Builtin("suspend", "builtin.#Suspend").
						WithFullParameter().
						Build()
				})

			cue := step.ToCue()
			Expect(cue).To(ContainSubstring("suspend: builtin.#Suspend & {"))
			Expect(cue).To(ContainSubstring("$params: parameter"))
		})

		It("should use WithFullParameter over WithParams when both are set", func() {
			step := defkit.NewWorkflowStep("suspend").
				Description("Suspend workflow").
				Params(defkit.String("message").Optional()).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.Builtin("suspend", "builtin.#Suspend").
						WithFullParameter().
						WithParams(map[string]defkit.Value{
							"message": defkit.Reference("parameter.message"),
						}).
						Build()
				})

			cue := step.ToCue()
			Expect(cue).To(ContainSubstring("$params: parameter"))
			Expect(cue).NotTo(ContainSubstring("$params: {"))
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

	Context("Status Block CUE Render", func() {
		It("should render statusDetails in CUE template block", func() {
			cue := defkit.NewWorkflowStep("x").StatusDetails("foo").ToCue()
			Expect(cue).To(ContainSubstring("status:"))
			Expect(cue).To(ContainSubstring("details:"))
		})

		It("should render customStatus in CUE template block", func() {
			cue := defkit.NewWorkflowStep("x").CustomStatus("message: \"ok\"").ToCue()
			Expect(cue).To(ContainSubstring("status:"))
			Expect(cue).To(ContainSubstring("customStatus:"))
		})

		It("should render healthPolicy in CUE template block", func() {
			cue := defkit.NewWorkflowStep("x").HealthPolicy("isHealth: true").ToCue()
			Expect(cue).To(ContainSubstring("status:"))
			Expect(cue).To(ContainSubstring("healthPolicy:"))
		})

		It("should omit status block when none set", func() {
			cue := defkit.NewWorkflowStep("x").ToCue()
			Expect(cue).NotTo(ContainSubstring("status:"))
		})

		It("should render all three status fields together", func() {
			cue := defkit.NewWorkflowStep("x").
				CustomStatus("message: \"ok\"").
				HealthPolicy("isHealth: true").
				StatusDetails("phase: \"running\"").
				ToCue()
			Expect(cue).To(ContainSubstring("status:"))
			Expect(cue).To(ContainSubstring("customStatus:"))
			Expect(cue).To(ContainSubstring("healthPolicy:"))
			Expect(cue).To(ContainSubstring("details:"))
		})
	})

	Context("Annotations", func() {
		It("should store and return annotations", func() {
			step := defkit.NewWorkflowStep("deploy").
				Annotations(map[string]string{"owner": "team-a", "env": "prod"})
			Expect(step.GetAnnotations()).To(HaveKeyWithValue("owner", "team-a"))
			Expect(step.GetAnnotations()).To(HaveKeyWithValue("env", "prod"))
		})

		It("should render sorted annotation keys in CUE", func() {
			cue := defkit.NewWorkflowStep("deploy").
				Annotations(map[string]string{"b": "2", "a": "1"}).
				ToCue()
			Expect(cue).To(ContainSubstring(`annotations: {`))
			aIdx := strings.Index(cue, `"a": "1"`)
			bIdx := strings.Index(cue, `"b": "2"`)
			Expect(aIdx).To(BeNumerically(">=", 0))
			Expect(bIdx).To(BeNumerically(">=", 0))
			Expect(aIdx).To(BeNumerically("<", bIdx))
		})

		It("should render empty annotations block in CUE when not set", func() {
			cue := defkit.NewWorkflowStep("deploy").ToCue()
			Expect(cue).To(ContainSubstring("annotations: {"))
		})

		It("should render user annotations alongside category", func() {
			cue := defkit.NewWorkflowStep("deploy").
				Annotations(map[string]string{"z": "1"}).
				Category("Delivery").
				ToCue()
			Expect(cue).To(ContainSubstring(`"z": "1"`))
			Expect(cue).To(ContainSubstring(`"category": "Delivery"`))
		})

		It("should merge user annotations in ToYAML without overriding description", func() {
			step := defkit.NewWorkflowStep("deploy").
				Description("My Step").
				Annotations(map[string]string{
					"owner": "team-a",
				})
			yamlBytes, err := step.ToYAML()
			Expect(err).NotTo(HaveOccurred())
			yaml := string(yamlBytes)
			Expect(yaml).To(ContainSubstring("owner: team-a"))
			Expect(yaml).To(ContainSubstring("My Step"))
		})

		It("should not allow user annotation to override description in ToYAML", func() {
			step := defkit.NewWorkflowStep("deploy").
				Description("Actual Description").
				Annotations(map[string]string{
					"definition.oam.dev/description": "Not This",
				})
			yamlBytes, err := step.ToYAML()
			Expect(err).NotTo(HaveOccurred())
			yaml := string(yamlBytes)
			Expect(yaml).To(ContainSubstring("Actual Description"))
		})
	})

	Context("SetGuardedBlock", func() {
		It("should preserve nested indentation for multi-line values", func() {
			step := defkit.NewWorkflowStep("notifier").
				Description("test").
				WithImports("vela/http", "vela/kube").
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.SetGuardedBlock(
						defkit.PathExists("parameter.channel"),
						"action",
						defkit.NewArrayElement().
							SetIf(defkit.PathExists("parameter.channel.url"), "post1",
								defkit.HTTPPost(defkit.Reference("parameter.channel.url")).
									Body(defkit.Reference("json.Marshal(parameter.channel.msg)")).
									Header("Content-Type", "application/json"),
							).
							SetIf(defkit.PathExists("parameter.channel.secret"), "read",
								defkit.KubeRead("v1", "Secret").
									Name(defkit.Reference("parameter.channel.secret.name")).
									Namespace(defkit.Reference("context.namespace")),
							),
					)
				})

			cue := step.ToCue()

			// The HTTPDo block should be indented deeper than the if block
			Expect(cue).To(ContainSubstring("action: {"))
			Expect(cue).To(ContainSubstring("if parameter.channel != _|_ {"))

			// Verify nested indentation: $params must be indented inside http.#HTTPDo
			lines := strings.Split(cue, "\n")
			var httpDoLine, paramsLine, methodLine int
			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "post1: http.#HTTPDo") {
					httpDoLine = i
				}
				if strings.HasPrefix(trimmed, "$params: {") && httpDoLine > 0 && paramsLine == 0 {
					paramsLine = i
				}
				if strings.HasPrefix(trimmed, "method: \"POST\"") && paramsLine > 0 && methodLine == 0 {
					methodLine = i
				}
			}
			Expect(httpDoLine).To(BeNumerically(">", 0), "should find post1: http.#HTTPDo line")
			Expect(paramsLine).To(BeNumerically(">", httpDoLine), "should find $params after http.#HTTPDo")
			Expect(methodLine).To(BeNumerically(">", paramsLine), "should find method after $params")

			// Verify each level is indented one more tab than its parent
			httpDoIndent := len(lines[httpDoLine]) - len(strings.TrimLeft(lines[httpDoLine], "\t"))
			paramsIndent := len(lines[paramsLine]) - len(strings.TrimLeft(lines[paramsLine], "\t"))
			methodIndent := len(lines[methodLine]) - len(strings.TrimLeft(lines[methodLine], "\t"))
			Expect(paramsIndent).To(Equal(httpDoIndent + 1))
			Expect(methodIndent).To(Equal(paramsIndent + 1))
		})

		It("should handle simple non-block values", func() {
			step := defkit.NewWorkflowStep("simple").
				Description("test").
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.SetGuardedBlock(
						defkit.PathExists("parameter.x"),
						"result",
						defkit.Reference("parameter.x"),
					)
				})

			cue := step.ToCue()
			Expect(cue).To(ContainSubstring("result: {"))
			Expect(cue).To(ContainSubstring("if parameter.x != _|_ {"))
			Expect(cue).To(ContainSubstring("parameter.x"))
		})

		It("should produce correct kube.#Read indentation inside guarded block", func() {
			step := defkit.NewWorkflowStep("reader").
				Description("test").
				WithImports("vela/kube").
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.SetGuardedBlock(
						defkit.PathExists("parameter.target"),
						"fetch",
						defkit.NewArrayElement().
							Set("read", defkit.KubeRead("v1", "Secret").
								Name(defkit.Reference("parameter.target.name")).
								Namespace(defkit.Reference("context.namespace")),
							),
					)
				})

			cue := step.ToCue()
			lines := strings.Split(cue, "\n")

			// Find the kube.#Read line and metadata line
			var readLine, metadataLine, nameLine int
			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "read: kube.#Read") {
					readLine = i
				}
				if strings.HasPrefix(trimmed, "metadata: {") && readLine > 0 && metadataLine == 0 {
					metadataLine = i
				}
				if strings.HasPrefix(trimmed, "name:") && metadataLine > 0 && nameLine == 0 {
					nameLine = i
				}
			}
			Expect(readLine).To(BeNumerically(">", 0))
			Expect(metadataLine).To(BeNumerically(">", readLine))
			Expect(nameLine).To(BeNumerically(">", metadataLine))

			// Verify progressive indentation
			readIndent := len(lines[readLine]) - len(strings.TrimLeft(lines[readLine], "\t"))
			metaIndent := len(lines[metadataLine]) - len(strings.TrimLeft(lines[metadataLine], "\t"))
			nameIndent := len(lines[nameLine]) - len(strings.TrimLeft(lines[nameLine], "\t"))
			Expect(metaIndent).To(BeNumerically(">", readIndent))
			Expect(nameIndent).To(BeNumerically(">", metaIndent))
		})
	})

	Context("Builtin action params ordering", func() {
		It("should render direct params sorted alphabetically in CUE", func() {
			ws := defkit.NewWorkflowStep("test").
				Description("test step").
				Params(defkit.String("name").Required()).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.Builtin("app", "op.#ShareCloudResource").
						WithDirectFields().
						WithParams(map[string]defkit.Value{
							"zeta":  defkit.Reference("parameter.name"),
							"alpha": defkit.Reference("context.namespace"),
							"mid":   defkit.Reference("context.name"),
						}).
						Build()
				})

			cue := ws.ToCue()

			alphaIdx := strings.Index(cue, "alpha:")
			midIdx := strings.Index(cue, "mid:")
			zetaIdx := strings.Index(cue, "zeta:")
			Expect(alphaIdx).To(BeNumerically(">=", 0))
			Expect(midIdx).To(BeNumerically(">=", 0))
			Expect(zetaIdx).To(BeNumerically(">=", 0))
			Expect(alphaIdx).To(BeNumerically("<", midIdx))
			Expect(midIdx).To(BeNumerically("<", zetaIdx))
		})

		It("should render $params wrapper params sorted alphabetically in CUE", func() {
			ws := defkit.NewWorkflowStep("test").
				Description("test step").
				Params(defkit.String("name").Required()).
				Template(func(tpl *defkit.WorkflowStepTemplate) {
					tpl.Builtin("deploy", "kube.#Apply").
						WithParams(map[string]defkit.Value{
							"zValue": defkit.Reference("parameter.name"),
							"aValue": defkit.Reference("context.namespace"),
						}).
						Build()
				})

			cue := ws.ToCue()

			aIdx := strings.Index(cue, "aValue:")
			zIdx := strings.Index(cue, "zValue:")
			Expect(aIdx).To(BeNumerically(">=", 0))
			Expect(zIdx).To(BeNumerically(">=", 0))
			Expect(aIdx).To(BeNumerically("<", zIdx))
		})

		It("should produce identical CUE across 20 runs", func() {
			build := func() string {
				return defkit.NewWorkflowStep("deploy").
					Description("deploy step").
					Labels(map[string]string{"z": "3", "a": "1", "m": "2"}).
					Params(defkit.String("name").Required()).
					Template(func(tpl *defkit.WorkflowStepTemplate) {
						tpl.Builtin("app", "op.#ShareCloudResource").
							WithDirectFields().
							WithParams(map[string]defkit.Value{
								"zeta":  defkit.Reference("parameter.name"),
								"alpha": defkit.Reference("context.namespace"),
								"mid":   defkit.Reference("context.name"),
							}).
							Build()
					}).
					ToCue()
			}

			first := build()
			for i := 0; i < 20; i++ {
				Expect(build()).To(Equal(first), "CUE output differed on iteration %d", i)
			}
		})
	})
})
