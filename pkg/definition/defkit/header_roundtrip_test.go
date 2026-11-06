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

	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

// fromCUE parses generated CUE the way `vela def apply-module` does and
// returns the resulting spec.
func fromCUE(cueStr string) map[string]any {
	def := pkgdef.Definition{}
	ExpectWithOffset(1, def.FromCUEString(cueStr, nil)).To(Succeed(), cueStr)
	spec, _, err := unstructured.NestedMap(def.Object, "spec")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return spec
}

var _ = Describe("Definition header round trip", func() {
	image := defkit.String("image")

	DescribeTable("version reaches spec.version",
		func(def interface{ ToCue() string }) {
			Expect(fromCUE(def.ToCue())).To(HaveKeyWithValue("version", "v1.2.0"))
		},
		Entry("component", defkit.NewComponent("c").Version("v1.2.0").
			Params(image).
			Template(func(tpl *defkit.Template) {
				tpl.Output(defkit.NewResource("v1", "ConfigMap"))
			})),
		Entry("trait", defkit.NewTrait("t").Version("v1.2.0").
			Params(image).
			Template(func(tpl *defkit.Template) {
				tpl.Patch().Set("metadata.labels.a", defkit.Lit("b"))
			})),
		Entry("trait with a template block", defkit.NewTrait("t").Version("v1.2.0").
			TemplateBlock("patch: metadata: labels: a: \"b\"\nparameter: {}")),
		Entry("policy", defkit.NewPolicy("p").Version("v1.2.0").
			Params(image).
			Template(func(tpl *defkit.PolicyTemplate) {
				tpl.Set("enabled", defkit.Lit(true))
			})),
		Entry("workflow step", defkit.NewWorkflowStep("w").Version("v1.2.0").
			Params(image).
			Template(func(tpl *defkit.WorkflowStepTemplate) {
				tpl.Set("ok", defkit.Lit(true))
			})),
	)

	It("writes a policy's manageHealthCheck into spec", func() {
		p := defkit.NewPolicy("p").ManageHealthCheck().
			Params(image).
			Template(func(tpl *defkit.PolicyTemplate) {
				tpl.Set("enabled", defkit.Lit(true))
			})
		Expect(fromCUE(p.ToCue())).To(HaveKeyWithValue("manageHealthCheck", true))
	})
})
