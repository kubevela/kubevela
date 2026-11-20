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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

// yamlDefinition is a definition that renders both the CUE file and the
// custom resource.
type yamlDefinition interface {
	ToCue() string
	ToYAML() ([]byte, error)
}

// objectFromYAML parses the output of ToYAML.
func objectFromYAML(def yamlDefinition) map[string]any {
	bs, err := def.ToYAML()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	obj := map[string]any{}
	ExpectWithOffset(1, yaml.Unmarshal(bs, &obj)).To(Succeed())
	return obj
}

// objectFromCUE converts the CUE file the way `vela def apply-module` does,
// normalised through YAML so it compares with objectFromYAML.
func objectFromCUE(def yamlDefinition) map[string]any {
	converted := pkgdef.Definition{}
	ExpectWithOffset(1, converted.FromCUEString(def.ToCue(), nil)).To(Succeed())
	bs, err := yaml.Marshal(converted.Object)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	obj := map[string]any{}
	ExpectWithOffset(1, yaml.Unmarshal(bs, &obj)).To(Succeed())
	return obj
}

func templateOf(obj map[string]any) string {
	tpl, found, err := unstructured.NestedString(obj, "spec", "schematic", "cue", "template")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, found).To(BeTrue())
	return tpl
}

var _ = Describe("ToYAML", func() {
	image := defkit.String("image").Required()
	labels := map[string]string{"team": "platform", "tier.oam.dev/level": "gold"}
	annotations := map[string]string{"owner": "team-a"}

	component := defkit.NewComponent("web").Description("A web service").Version("v1.2.0").
		Workload("apps/v1", "Deployment").
		Labels(labels).Annotations(annotations).
		WithImports("strings").
		Params(image).
		HealthPolicy("isHealth: context.output.status.readyReplicas > 0").
		CustomStatus(`message: "ready"`).
		Template(func(tpl *defkit.Template) {
			tpl.Output(defkit.NewResource("apps/v1", "Deployment").
				Set("spec.template.spec.containers[0].image", image))
		})
	trait := defkit.NewTrait("labeler").Description("A trait").Version("v1.0.0").
		AppliesTo("deployments.apps").ConflictsWith("other").PodDisruptive(true).Stage("PostDispatch").
		Labels(labels).Annotations(annotations).
		HealthPolicy("isHealth: true").
		CustomStatus(`message: "ok"`).
		Params(image).
		Template(func(tpl *defkit.Template) {
			tpl.Patch().Set("metadata.labels.a", defkit.Lit("b"))
		})
	policy := defkit.NewPolicy("guard").Description("A policy").Version("v1.0.0").
		ManageHealthCheck().Global().Priority(5).
		Labels(labels).Annotations(annotations).
		Params(image).
		Template(func(tpl *defkit.PolicyTemplate) {
			tpl.Set("enabled", defkit.Lit(true))
		})
	step := defkit.NewWorkflowStep("notify").Description("A step").Version("v1.0.0").
		Category("Notification").Scope("Application").Alias("n").
		Labels(labels).Annotations(annotations).
		Params(image).
		Template(func(tpl *defkit.WorkflowStepTemplate) {
			tpl.Set("ok", defkit.Lit(true))
		})

	DescribeTable("embeds only the template body",
		func(def yamlDefinition, name, header string) {
			tpl := templateOf(objectFromYAML(def))
			Expect(tpl).NotTo(MatchRegexp(`(?m)^template:`))
			Expect(tpl).NotTo(ContainSubstring(name + ": {"))
			Expect(tpl).NotTo(ContainSubstring(header))
			Expect(tpl).NotTo(ContainSubstring("attributes:"))
			Expect(tpl).To(ContainSubstring("parameter: image!: string"))
			Expect(tpl).To(Equal(templateOf(objectFromCUE(def))))
		},
		Entry("component", component, "web", `type: "component"`),
		Entry("trait", trait, "labeler", `type: "trait"`),
		Entry("policy", policy, "guard", `type: "policy"`),
		Entry("workflow step", step, "notify", `type: "workflow-step"`),
	)

	DescribeTable("produces the object the CUE file converts to",
		func(def yamlDefinition) {
			Expect(objectFromYAML(def)).To(Equal(objectFromCUE(def)))
		},
		Entry("component", component),
		Entry("trait", trait),
		Entry("policy", policy),
		Entry("workflow step", step),
		Entry("component with autodetected workload", defkit.NewComponent("auto").AutodetectWorkload().
			Params(image).
			Template(func(tpl *defkit.Template) {
				tpl.Output(defkit.NewResource("v1", "ConfigMap"))
			})),
		Entry("trait with a template block", defkit.NewTrait("block").AppliesTo("*").
			TemplateBlock("patch: metadata: labels: a: \"b\"\nparameter: {}")),
		Entry("workflow step with a template body", defkit.NewWorkflowStep("body").
			TemplateBody("nop: {}\nparameter: {}")),
	)

	It("keeps the imports of a component template", func() {
		Expect(templateOf(objectFromYAML(component))).To(HavePrefix("import (\n\t\"strings\"\n)\noutput: {"))
	})

	It("stores a component's health policy and custom status in spec.status", func() {
		status, found, err := unstructured.NestedMap(objectFromYAML(component), "spec", "status")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(status).To(HaveKeyWithValue("healthPolicy", "isHealth: context.output.status.readyReplicas > 0"))
		Expect(status).To(HaveKeyWithValue("customStatus", `message: "ready"`))
	})

	It("writes labels and prefixes user metadata keys", func() {
		metadata, _, err := unstructured.NestedMap(objectFromYAML(component), "metadata")
		Expect(err).NotTo(HaveOccurred())
		Expect(metadata["labels"]).To(Equal(map[string]any{
			"custom.definition.oam.dev/team": "platform",
			"tier.oam.dev/level":             "gold",
		}))
		Expect(metadata["annotations"]).To(Equal(map[string]any{
			"custom.definition.oam.dev/owner": "team-a",
			"definition.oam.dev/description":  "A web service",
		}))
	})

	It("writes a workflow step's category, scope and alias", func() {
		metadata, _, err := unstructured.NestedMap(objectFromYAML(step), "metadata")
		Expect(err).NotTo(HaveOccurred())
		Expect(metadata["annotations"]).To(HaveKeyWithValue("custom.definition.oam.dev/category", "Notification"))
		Expect(metadata["annotations"]).To(HaveKeyWithValue("definition.oam.dev/alias", "n"))
		Expect(metadata["labels"]).To(HaveKeyWithValue("custom.definition.oam.dev/scope", "Application"))
	})

	It("keeps childResourceKinds and podSpecPath next to the converted spec", func() {
		c := defkit.NewComponent("job").Workload("batch/v1", "Job").
			ChildResourceKind("v1", "Pod", map[string]string{"job-name": "x"}).
			PodSpecPath("spec.template.spec").
			Params(image).
			Template(func(tpl *defkit.Template) {
				tpl.Output(defkit.NewResource("batch/v1", "Job"))
			})
		spec, _, err := unstructured.NestedMap(objectFromYAML(c), "spec")
		Expect(err).NotTo(HaveOccurred())
		Expect(spec).To(HaveKeyWithValue("podSpecPath", "spec.template.spec"))
		Expect(spec).To(HaveKey("childResourceKinds"))
		delete(spec, "podSpecPath")
		delete(spec, "childResourceKinds")
		converted, _, err := unstructured.NestedMap(objectFromCUE(c), "spec")
		Expect(err).NotTo(HaveOccurred())
		Expect(spec).To(Equal(converted))
	})

	It("fails for raw CUE that is not a definition file", func() {
		_, err := defkit.NewComponent("raw").RawCUE("template: {\n\toutput: {}\n\tparameter: {}\n}").ToYAML()
		Expect(err).To(MatchError(ContainSubstring("no metadata found")))
	})
})
