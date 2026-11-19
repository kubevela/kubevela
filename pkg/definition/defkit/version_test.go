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
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("Version field on all definition types", func() {

	Context("Task 1: GetVersion round-trip", func() {
		It("NewTrait Version round-trip", func() {
			Expect(defkit.NewTrait("t").Version("1.0").GetVersion()).To(Equal("1.0"))
		})

		It("NewComponent Version round-trip", func() {
			Expect(defkit.NewComponent("c").Version("2.0").GetVersion()).To(Equal("2.0"))
		})

		It("NewPolicy empty Version", func() {
			Expect(defkit.NewPolicy("p").Version("").GetVersion()).To(Equal(""))
		})

		It("NewTrait default GetVersion is empty string", func() {
			Expect(defkit.NewTrait("t").GetVersion()).To(Equal(""))
		})

		It("Version is chainable on TraitDefinition", func() {
			_ = defkit.NewTrait("t").Version("1.0").Description("d")
		})

		It("Version is chainable on ComponentDefinition", func() {
			_ = defkit.NewComponent("c").Version("1.0").Description("d")
		})

		It("Version is chainable on PolicyDefinition", func() {
			_ = defkit.NewPolicy("p").Version("1.0").Description("d")
		})

		It("Version is chainable on WorkflowStepDefinition", func() {
			_ = defkit.NewWorkflowStep("w").Version("1.0").Description("d")
		})
	})

	Context("Task 2: CUE render — version field conditional", func() {
		It("NewTrait with Version produces version in ToCue", func() {
			cue := defkit.NewTrait("t").Version("1.0").ToCue()
			Expect(cue).To(MatchRegexp(`version:\s+"1\.0"`))
		})

		It("NewTrait without Version omits version in ToCue", func() {
			cue := defkit.NewTrait("t").ToCue()
			Expect(cue).NotTo(ContainSubstring("version:"))
		})

		It("NewComponent with Version produces version in ToCue", func() {
			cue := defkit.NewComponent("c").Version("2.0").ToCue()
			Expect(cue).To(ContainSubstring(`version: "2.0"`))
		})

		It("NewComponent without Version omits version in ToCue", func() {
			cue := defkit.NewComponent("c").ToCue()
			Expect(cue).NotTo(ContainSubstring("version:"))
		})

		It("NewPolicy with Version produces version in ToCue", func() {
			cue := defkit.NewPolicy("p").Version("1.0").ToCue()
			Expect(cue).To(ContainSubstring(`version: "1.0"`))
		})

		It("NewWorkflowStep with Version produces version in ToCue", func() {
			cue := defkit.NewWorkflowStep("w").Version("1.0").ToCue()
			Expect(cue).To(ContainSubstring(`version: "1.0"`))
		})

		It("NewWorkflowStep without Version omits version in ToCue", func() {
			cue := defkit.NewWorkflowStep("w").ToCue()
			Expect(cue).NotTo(ContainSubstring("version:"))
		})

		It("NewPolicy without Version omits version in ToCue", func() {
			cue := defkit.NewPolicy("p").ToCue()
			Expect(cue).NotTo(ContainSubstring("version:"))
		})
	})

	Context("Task 2: ToYAML spec.version conditional", func() {
		It("TraitDefinition with Version sets spec.version in ToYAML", func() {
			yamlBytes, err := defkit.NewTrait("t").Version("1.0").ToYAML()
			Expect(err).NotTo(HaveOccurred())
			var obj map[string]any
			Expect(yaml.Unmarshal(yamlBytes, &obj)).To(Succeed())
			spec := obj["spec"].(map[string]any)
			Expect(spec["version"]).To(Equal("1.0"))
		})

		It("TraitDefinition without Version omits spec.version in ToYAML", func() {
			yamlBytes, err := defkit.NewTrait("t").ToYAML()
			Expect(err).NotTo(HaveOccurred())
			var obj map[string]any
			Expect(yaml.Unmarshal(yamlBytes, &obj)).To(Succeed())
			spec := obj["spec"].(map[string]any)
			Expect(spec).NotTo(HaveKey("version"))
		})

		It("ComponentDefinition with Version sets spec.version in ToYAML", func() {
			yamlBytes, err := defkit.NewComponent("c").Version("1.0").ToYAML()
			Expect(err).NotTo(HaveOccurred())
			var obj map[string]any
			Expect(yaml.Unmarshal(yamlBytes, &obj)).To(Succeed())
			spec := obj["spec"].(map[string]any)
			Expect(spec["version"]).To(Equal("1.0"))
		})

		It("PolicyDefinition with Version sets spec.version in ToYAML", func() {
			yamlBytes, err := defkit.NewPolicy("p").Version("1.0").ToYAML()
			Expect(err).NotTo(HaveOccurred())
			var obj map[string]any
			Expect(yaml.Unmarshal(yamlBytes, &obj)).To(Succeed())
			spec := obj["spec"].(map[string]any)
			Expect(spec["version"]).To(Equal("1.0"))
		})

		It("WorkflowStepDefinition with Version sets spec.version in ToYAML", func() {
			yamlBytes, err := defkit.NewWorkflowStep("w").Version("1.0").ToYAML()
			Expect(err).NotTo(HaveOccurred())
			var obj map[string]any
			Expect(yaml.Unmarshal(yamlBytes, &obj)).To(Succeed())
			spec := obj["spec"].(map[string]any)
			Expect(spec["version"]).To(Equal("1.0"))
		})

		It("ComponentDefinition without Version omits spec.version in ToYAML", func() {
			yamlBytes, err := defkit.NewComponent("c").ToYAML()
			Expect(err).NotTo(HaveOccurred())
			var obj map[string]any
			Expect(yaml.Unmarshal(yamlBytes, &obj)).To(Succeed())
			spec := obj["spec"].(map[string]any)
			Expect(spec).NotTo(HaveKey("version"))
		})

		It("PolicyDefinition without Version omits spec.version in ToYAML", func() {
			yamlBytes, err := defkit.NewPolicy("p").ToYAML()
			Expect(err).NotTo(HaveOccurred())
			var obj map[string]any
			Expect(yaml.Unmarshal(yamlBytes, &obj)).To(Succeed())
			spec := obj["spec"].(map[string]any)
			Expect(spec).NotTo(HaveKey("version"))
		})

		It("WorkflowStepDefinition without Version omits spec.version in ToYAML", func() {
			yamlBytes, err := defkit.NewWorkflowStep("w").ToYAML()
			Expect(err).NotTo(HaveOccurred())
			var obj map[string]any
			Expect(yaml.Unmarshal(yamlBytes, &obj)).To(Succeed())
			spec := obj["spec"].(map[string]any)
			Expect(spec).NotTo(HaveKey("version"))
		})
	})
})
