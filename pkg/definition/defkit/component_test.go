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

var _ = Describe("ComponentDefinition", func() {

	Context("NewComponent", func() {
		It("should create a component with name", func() {
			c := defkit.NewComponent("webservice")
			Expect(c.GetName()).To(Equal("webservice"))
			Expect(c.GetParams()).To(BeEmpty())
		})
	})

	Context("Component Configuration", func() {
		It("should set description", func() {
			c := defkit.NewComponent("webservice").
				Description("Describes long-running, scalable, containerized services")
			Expect(c.GetDescription()).To(Equal("Describes long-running, scalable, containerized services"))
		})

		It("should set workload type", func() {
			c := defkit.NewComponent("webservice").
				Workload("apps/v1", "Deployment")
			Expect(c.GetWorkload().APIVersion()).To(Equal("apps/v1"))
			Expect(c.GetWorkload().Kind()).To(Equal("Deployment"))
		})

		It("should add parameters", func() {
			image := defkit.String("image").Required()
			replicas := defkit.Int("replicas").Default(1)
			c := defkit.NewComponent("webservice").
				Params(image, replicas)
			Expect(c.GetParams()).To(HaveLen(2))
			Expect(c.GetParams()[0].Name()).To(Equal("image"))
			Expect(c.GetParams()[1].Name()).To(Equal("replicas"))
		})

		It("should set template function", func() {
			var templateCalled bool
			c := defkit.NewComponent("webservice").
				Template(func(tpl *defkit.Template) {
					templateCalled = true
				})
			Expect(c.GetTemplate()).NotTo(BeNil())
			c.GetTemplate()(defkit.NewTemplate())
			Expect(templateCalled).To(BeTrue())
		})

		It("should set autodetect workload mode", func() {
			c := defkit.NewComponent("test").
				AutodetectWorkload()
			Expect(c.GetWorkload().IsAutodetect()).To(BeTrue())
		})
	})

	Context("Template Operations", func() {
		It("should create a new template", func() {
			tpl := defkit.NewTemplate()
			Expect(tpl).NotTo(BeNil())
			Expect(tpl.GetOutput()).To(BeNil())
			Expect(tpl.GetOutputs()).To(BeEmpty())
		})

		It("should set primary output", func() {
			tpl := defkit.NewTemplate()
			r := defkit.NewResource("apps/v1", "Deployment")
			tpl.Output(r)
			Expect(tpl.GetOutput()).To(Equal(r))
		})

		It("should return existing output when called without args", func() {
			tpl := defkit.NewTemplate()
			r := defkit.NewResource("apps/v1", "Deployment")
			tpl.Output(r)
			Expect(tpl.Output()).To(Equal(r))
		})

		It("should set auxiliary outputs", func() {
			tpl := defkit.NewTemplate()
			svc := defkit.NewResource("v1", "Service")
			cm := defkit.NewResource("v1", "ConfigMap")
			tpl.Outputs("service", svc)
			tpl.Outputs("config", cm)
			Expect(tpl.GetOutputs()).To(HaveLen(2))
			Expect(tpl.Outputs("service")).To(Equal(svc))
			Expect(tpl.Outputs("config")).To(Equal(cm))
		})

		It("should support patch with strategy", func() {
			tpl := defkit.NewTemplate()
			tpl.PatchStrategy("retainKeys").
				Patch().Set("spec.replicas", defkit.Lit(1))

			Expect(tpl.HasPatch()).To(BeTrue())
			Expect(tpl.GetPatchStrategy()).To(Equal("retainKeys"))
			Expect(tpl.GetPatch()).NotTo(BeNil())
		})

		It("should support patch without strategy", func() {
			tpl := defkit.NewTemplate()
			tpl.Patch().Set("spec.replicas", defkit.Lit(1))

			Expect(tpl.HasPatch()).To(BeTrue())
			Expect(tpl.GetPatchStrategy()).To(BeEmpty())
		})
	})

	Context("Template Helper Registration", func() {
		It("should register struct array helpers", func() {
			tpl := defkit.NewTemplate()
			volumeMounts := defkit.Object("volumeMounts")
			tpl.StructArrayHelper("volumesArray", volumeMounts).
				Field("pvc", defkit.FieldMap{"name": defkit.FieldRef("name")}).
				Build()

			Expect(tpl.GetStructArrayHelpers()).To(HaveLen(1))
			Expect(tpl.GetStructArrayHelpers()[0].HelperName()).To(Equal("volumesArray"))
		})

		It("should register concat helpers", func() {
			tpl := defkit.NewTemplate()
			volumeMounts := defkit.Object("volumeMounts")
			structHelper := tpl.StructArrayHelper("volumesArray", volumeMounts).
				Field("pvc", defkit.FieldMap{"name": defkit.FieldRef("name")}).
				Field("configMap", defkit.FieldMap{"name": defkit.FieldRef("name")}).
				Build()

			tpl.ConcatHelper("volumesList", structHelper).
				Fields("pvc", "configMap").
				Build()

			Expect(tpl.GetConcatHelpers()).To(HaveLen(1))
			Expect(tpl.GetConcatHelpers()[0].HelperName()).To(Equal("volumesList"))
		})

		It("should register dedupe helpers", func() {
			tpl := defkit.NewTemplate()
			volumeMounts := defkit.Object("volumeMounts")
			structHelper := tpl.StructArrayHelper("volumesArray", volumeMounts).
				Field("pvc", defkit.FieldMap{"name": defkit.FieldRef("name")}).
				Build()

			concatHelper := tpl.ConcatHelper("volumesList", structHelper).
				Fields("pvc").
				Build()

			tpl.DedupeHelper("uniqueVolumes", concatHelper).
				ByKey("name").
				Build()

			Expect(tpl.GetDedupeHelpers()).To(HaveLen(1))
			Expect(tpl.GetDedupeHelpers()[0].HelperName()).To(Equal("uniqueVolumes"))
		})
	})

	Context("PatchResource", func() {
		It("should create patch resource with Set operations", func() {
			patch := defkit.NewPatchResource()
			patch.Set("spec.replicas", defkit.Lit(3))
			patch.Set("metadata.labels.app", defkit.Lit("myapp"))

			Expect(patch.Ops()).To(HaveLen(2))
		})

		It("should add SetIf operation", func() {
			cpu := defkit.String("cpu")
			patch := defkit.NewPatchResource()
			patch.SetIf(cpu.IsSet(), "spec.resources.limits.cpu", cpu)

			Expect(patch.Ops()).To(HaveLen(1))
			setIfOp, ok := patch.Ops()[0].(*defkit.SetIfOp)
			Expect(ok).To(BeTrue())
			Expect(setIfOp.Path()).To(Equal("spec.resources.limits.cpu"))
		})

		It("should add SpreadIf operation", func() {
			labels := defkit.Object("labels")
			patch := defkit.NewPatchResource()
			patch.SpreadIf(labels.IsSet(), "metadata.labels", labels)

			Expect(patch.Ops()).To(HaveLen(1))
			spreadIfOp, ok := patch.Ops()[0].(*defkit.SpreadIfOp)
			Expect(ok).To(BeTrue())
			Expect(spreadIfOp.Path()).To(Equal("metadata.labels"))
		})

		It("should add ForEach operation", func() {
			labels := defkit.Object("labels")
			patch := defkit.NewPatchResource()
			patch.ForEach(labels, "metadata.labels")

			Expect(patch.Ops()).To(HaveLen(1))
			forEachOp, ok := patch.Ops()[0].(*defkit.ForEachOp)
			Expect(ok).To(BeTrue())
			Expect(forEachOp.Path()).To(Equal("metadata.labels"))
			Expect(forEachOp.Source()).To(Equal(labels))
		})

		It("should handle If/EndIf blocks", func() {
			enabled := defkit.Bool("enabled")
			replicas := defkit.Int("replicas")
			cond := defkit.Eq(enabled, defkit.Lit(true))

			patch := defkit.NewPatchResource()
			patch.If(cond).
				Set("spec.replicas", replicas).
				EndIf()

			Expect(patch.Ops()).To(HaveLen(1))
			ifBlock, ok := patch.Ops()[0].(*defkit.IfBlock)
			Expect(ok).To(BeTrue())
			Expect(ifBlock.Cond()).To(Equal(cond))
			Expect(ifBlock.Ops()).To(HaveLen(1))
		})

		It("should handle SetIf within If block", func() {
			enabled := defkit.Bool("enabled")
			cpu := defkit.String("cpu")
			outerCond := defkit.Eq(enabled, defkit.Lit(true))

			patch := defkit.NewPatchResource()
			patch.If(outerCond).
				SetIf(cpu.IsSet(), "spec.resources.limits.cpu", cpu).
				EndIf()

			Expect(patch.Ops()).To(HaveLen(1))
			ifBlock := patch.Ops()[0].(*defkit.IfBlock)
			Expect(ifBlock.Ops()).To(HaveLen(1))
		})

		It("should handle SpreadIf within If block", func() {
			enabled := defkit.Bool("enabled")
			labels := defkit.Object("labels")
			outerCond := defkit.Eq(enabled, defkit.Lit(true))

			patch := defkit.NewPatchResource()
			patch.If(outerCond).
				SpreadIf(labels.IsSet(), "metadata.labels", labels).
				EndIf()

			Expect(patch.Ops()).To(HaveLen(1))
			ifBlock := patch.Ops()[0].(*defkit.IfBlock)
			Expect(ifBlock.Ops()).To(HaveLen(1))
		})

		It("should handle ForEach within If block", func() {
			enabled := defkit.Bool("enabled")
			labels := defkit.Object("labels")
			outerCond := defkit.Eq(enabled, defkit.Lit(true))

			patch := defkit.NewPatchResource()
			patch.If(outerCond).
				ForEach(labels, "metadata.labels").
				EndIf()

			Expect(patch.Ops()).To(HaveLen(1))
			ifBlock := patch.Ops()[0].(*defkit.IfBlock)
			Expect(ifBlock.Ops()).To(HaveLen(1))
		})

		It("should add PatchKey operation", func() {
			container := defkit.NewArrayElement().
				Set("name", defkit.Lit("nginx")).
				Set("image", defkit.Lit("nginx:latest"))

			patch := defkit.NewPatchResource()
			patch.PatchKey("spec.template.spec.containers", "name", container)

			Expect(patch.Ops()).To(HaveLen(1))
			patchKeyOp, ok := patch.Ops()[0].(*defkit.PatchKeyOp)
			Expect(ok).To(BeTrue())
			Expect(patchKeyOp.Path()).To(Equal("spec.template.spec.containers"))
			Expect(patchKeyOp.Key()).To(Equal("name"))
			Expect(patchKeyOp.Elements()).To(HaveLen(1))
		})

		It("should handle PatchKey within If block", func() {
			enabled := defkit.Bool("enabled")
			container := defkit.NewArrayElement().Set("name", defkit.Lit("nginx"))
			outerCond := defkit.Eq(enabled, defkit.Lit(true))

			patch := defkit.NewPatchResource()
			patch.If(outerCond).
				PatchKey("spec.containers", "name", container).
				EndIf()

			Expect(patch.Ops()).To(HaveLen(1))
			ifBlock := patch.Ops()[0].(*defkit.IfBlock)
			Expect(ifBlock.Ops()).To(HaveLen(1))
		})

		It("should add Passthrough operation", func() {
			patch := defkit.NewPatchResource()
			patch.Passthrough()

			Expect(patch.Ops()).To(HaveLen(1))
			_, ok := patch.Ops()[0].(*defkit.PassthroughOp)
			Expect(ok).To(BeTrue())
		})
	})

	Context("Template OutputsIf method", func() {
		It("should add conditional auxiliary resource", func() {
			enabled := defkit.Bool("enabled")
			cond := defkit.Eq(enabled, defkit.Lit(true))

			tpl := defkit.NewTemplate()
			tpl.OutputsIf(cond, "service",
				defkit.NewResource("v1", "Service").
					Set("metadata.name", defkit.VelaCtx().Name()),
			)

			outputs := tpl.GetOutputs()
			Expect(outputs).To(HaveKey("service"))
			Expect(outputs["service"].Kind()).To(Equal("Service"))
		})
	})

	Context("ContextOutputRef", func() {
		It("should create context output reference", func() {
			ref := defkit.ContextOutput()
			Expect(ref).NotTo(BeNil())
			Expect(ref.Path()).To(Equal("context.output"))
		})

		It("should access nested field", func() {
			ref := defkit.ContextOutput().Field("spec.template")
			Expect(ref.Path()).To(Equal("context.output.spec.template"))
		})

		It("should chain Field calls", func() {
			ref := defkit.ContextOutput().Field("spec").Field("template")
			Expect(ref.Path()).To(Equal("context.output.spec.template"))
		})

		It("should create HasPath condition", func() {
			cond := defkit.ContextOutput().HasPath("spec.template")
			Expect(cond).NotTo(BeNil())

			pathCond, ok := cond.(*defkit.ContextPathExistsCondition)
			Expect(ok).To(BeTrue())
			Expect(pathCond.BasePath()).To(Equal("context.output"))
			Expect(pathCond.FieldPath()).To(Equal("spec.template"))
			Expect(pathCond.FullPath()).To(Equal("context.output.spec.template"))
		})

		It("should create IsSet condition", func() {
			cond := defkit.ContextOutput().IsSet()
			Expect(cond).NotTo(BeNil())

			pathCond, ok := cond.(*defkit.ContextPathExistsCondition)
			Expect(ok).To(BeTrue())
			Expect(pathCond.FullPath()).To(Equal("context.output"))
		})

		It("should use ContextOutput in patch condition", func() {
			patch := defkit.NewPatchResource()
			hasTemplate := defkit.ContextOutput().HasPath("spec.template")

			patch.If(hasTemplate).
				Set("spec.template.metadata.labels.app", defkit.Lit("test")).
				EndIf()

			Expect(patch.Ops()).To(HaveLen(1))
		})
	})

	Context("RenderedResource Data method", func() {
		It("should return rendered resource data", func() {
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", defkit.VelaCtx().Name()).
							Set("spec.replicas", defkit.Lit(3)),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().WithName("myapp"),
			)

			data := rendered.Data()
			Expect(data).NotTo(BeNil())
			Expect(data["metadata"].(map[string]any)["name"]).To(Equal("myapp"))
		})
	})

	Context("DefName and DefType", func() {
		It("should return correct definition name", func() {
			c := defkit.NewComponent("mycomp")
			Expect(c.DefName()).To(Equal("mycomp"))
		})

		It("should return component definition type", func() {
			c := defkit.NewComponent("test")
			Expect(c.DefType()).To(Equal(defkit.DefinitionTypeComponent))
		})
	})

	Context("Component Param method", func() {
		It("should add a single parameter using Param method", func() {
			image := defkit.String("image")
			c := defkit.NewComponent("test").
				Param(image)
			Expect(c.GetParams()).To(HaveLen(1))
			Expect(c.GetParams()[0].Name()).To(Equal("image"))
		})

		It("should chain multiple Param calls", func() {
			c := defkit.NewComponent("test").
				Param(defkit.String("image")).
				Param(defkit.Int("replicas")).
				Param(defkit.Bool("enabled"))
			Expect(c.GetParams()).To(HaveLen(3))
		})
	})

	Context("Component Helper method", func() {
		It("should add a helper definition with a param", func() {
			probeParam := defkit.Struct("probe").Fields(
				defkit.Field("path", defkit.ParamTypeString),
				defkit.Field("port", defkit.ParamTypeInt).Default(8080),
			)
			c := defkit.NewComponent("test").
				Helper("HealthProbe", probeParam)
			helpers := c.GetHelperDefinitions()
			Expect(helpers).To(HaveLen(1))
			Expect(helpers[0].GetName()).To(Equal("HealthProbe"))
			Expect(helpers[0].HasParam()).To(BeTrue())
			Expect(helpers[0].GetParam()).To(Equal(probeParam))
		})

		It("should return empty schema when using param", func() {
			probeParam := defkit.Struct("probe")
			c := defkit.NewComponent("test").
				Helper("Probe", probeParam)
			helpers := c.GetHelperDefinitions()
			Expect(helpers[0].GetSchema()).To(BeEmpty())
		})
	})

	Context("Component RawCUE method", func() {
		It("should set raw CUE and bypass template generation", func() {
			rawCue := `"webservice": {
	type: "component"
	template: {
		output: { apiVersion: "apps/v1", kind: "Deployment" }
	}
}`
			c := defkit.NewComponent("webservice").RawCUE(rawCue)

			Expect(c.GetRawCUE()).To(Equal(rawCue))
			Expect(c.HasRawCUE()).To(BeTrue())
			Expect(c.ToCue()).To(Equal(rawCue))
		})
	})

	Context("Component WithImports method", func() {
		It("should add imports to the component", func() {
			c := defkit.NewComponent("test").
				WithImports("strconv", "strings")
			Expect(c.GetImports()).To(ConsistOf("strconv", "strings"))
		})

		It("should accumulate imports with multiple calls", func() {
			c := defkit.NewComponent("test").
				WithImports("strconv").
				WithImports("strings", "list")
			Expect(c.GetImports()).To(HaveLen(3))
		})
	})

	Context("Component ToYAML method", func() {
		It("should generate valid YAML manifest", func() {
			c := defkit.NewComponent("webservice").
				Description("Web service component").
				Workload("apps/v1", "Deployment").
				Params(defkit.String("image").Required())

			yamlBytes, err := c.ToYAML()
			Expect(err).NotTo(HaveOccurred())

			yaml := string(yamlBytes)
			Expect(yaml).To(ContainSubstring("kind: ComponentDefinition"))
			Expect(yaml).To(ContainSubstring("name: webservice"))
		})
	})

	Context("Component ToCueWithImports method", func() {
		It("should generate CUE with import block", func() {
			c := defkit.NewComponent("test").
				Description("Test").
				Workload("apps/v1", "Deployment")

			cue := c.ToCueWithImports("strconv", "strings")
			Expect(cue).To(ContainSubstring(`import (`))
			Expect(cue).To(ContainSubstring(`"strconv"`))
			Expect(cue).To(ContainSubstring(`"strings"`))
		})
	})

	Context("Component ToParameterSchema method", func() {
		It("should generate parameter schema CUE", func() {
			c := defkit.NewComponent("test").
				Params(
					defkit.String("image").Required(),
					defkit.Int("replicas").Default(1),
				)

			schema := c.ToParameterSchema()
			Expect(schema).To(ContainSubstring("image:"))
			Expect(schema).To(ContainSubstring("replicas:"))
		})
	})

	Context("ToCue Generation", func() {
		It("should generate complete CUE definition", func() {
			c := defkit.NewComponent("webservice").
				Description("Web service component").
				Workload("apps/v1", "Deployment").
				Params(
					defkit.String("image").Required().Description("Container image"),
					defkit.Int("replicas").Default(1),
				)

			cue := c.ToCue()

			Expect(cue).To(ContainSubstring(`webservice: {`))
			Expect(cue).To(ContainSubstring(`type: "component"`))
			Expect(cue).To(ContainSubstring(`description: "Web service component"`))
			Expect(cue).To(ContainSubstring(`apiVersion: "apps/v1"`))
			Expect(cue).To(ContainSubstring(`kind:`))
			Expect(cue).To(ContainSubstring(`"Deployment"`))
			Expect(cue).To(ContainSubstring(`parameter: {`))
			Expect(cue).To(ContainSubstring(`image: string`))
			Expect(cue).To(ContainSubstring(`replicas: *1 | int`))
		})

		It("should generate CUE with template output", func() {
			c := defkit.NewComponent("test").
				Description("Test component").
				Workload("apps/v1", "Deployment").
				Params(defkit.String("name")).
				Template(func(tpl *defkit.Template) {
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", defkit.ParamRef("name")),
					)
				})

			cue := c.ToCue()

			Expect(cue).To(ContainSubstring(`output:`))
			Expect(cue).To(ContainSubstring(`metadata:`))
			Expect(cue).To(ContainSubstring(`name: parameter.name`))
		})

		It("should generate CUE with status", func() {
			c := defkit.NewComponent("stateful").
				Description("Stateful component").
				Workload("apps/v1", "Deployment").
				CustomStatus("message: \"Running\"").
				HealthPolicy("isHealth: true")

			cue := c.ToCue()

			Expect(cue).To(ContainSubstring(`status:`))
			Expect(cue).To(ContainSubstring(`customStatus:`))
			Expect(cue).To(ContainSubstring(`healthPolicy:`))
		})
	})

	Context("Full Component Example", func() {
		It("should build a complete webservice component", func() {
			image := defkit.String("image").Required().Description("Container image")
			replicas := defkit.Int("replicas").Default(1)
			port := defkit.Int("port").Default(80)

			c := defkit.NewComponent("webservice").
				Description("Web service component").
				Workload("apps/v1", "Deployment").
				Params(image, replicas, port).
				Template(func(tpl *defkit.Template) {
					vela := defkit.VelaCtx()
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", vela.Name()).
							Set("spec.replicas", replicas).
							Set("spec.template.spec.containers[0].image", image).
							Set("spec.template.spec.containers[0].ports[0].containerPort", port),
					)
					tpl.Outputs("service",
						defkit.NewResource("v1", "Service").
							Set("metadata.name", vela.Name()).
							Set("spec.ports[0].port", port),
					)
				})

			Expect(c.GetName()).To(Equal("webservice"))
			Expect(c.GetParams()).To(HaveLen(3))
			Expect(c.GetWorkload().Kind()).To(Equal("Deployment"))

			// Execute template to verify it builds correctly
			tpl := defkit.NewTemplate()
			c.GetTemplate()(tpl)
			Expect(tpl.GetOutput()).NotTo(BeNil())
			Expect(tpl.GetOutput().Kind()).To(Equal("Deployment"))
			Expect(tpl.GetOutput().Ops()).To(HaveLen(4))
			Expect(tpl.GetOutputs()).To(HaveKey("service"))
		})
	})
})
