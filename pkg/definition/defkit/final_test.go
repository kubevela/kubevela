/*
Copyright 2026 The KubeVela Authors.

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
	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
)

var _ = Describe("Resource.Final", func() {
	secureWeb := func() *defkit.ComponentDefinition {
		image := defkit.String("image")
		return defkit.NewComponent("secure-web").
			Workload("apps/v1", "Deployment").
			Params(image).
			Template(func(tpl *defkit.Template) {
				tpl.Output(
					defkit.NewResource("apps/v1", "Deployment").
						Set("spec.template.spec.securityContext.runAsNonRoot", defkit.Lit(true)).
						Set("spec.template.spec.containers[0].image", image).
						Final("spec.template.spec.securityContext"),
				)
			})
	}

	It("records a +final directive on the path", func() {
		r := defkit.NewResource("apps/v1", "Deployment").Final("spec.selector")
		Expect(r.Ops()).To(HaveLen(1))
		op, ok := r.Ops()[0].(*defkit.DirectiveOp)
		Expect(ok).To(BeTrue())
		Expect(op.Path()).To(Equal("spec.selector"))
		Expect(op.GetDirective()).To(Equal("final"))
	})

	It("emits the marker in a form the inheritance check reads", func() {
		// The template as it is stored, after `vela def apply-module` has read
		// the generated definition.
		tpl, found, err := unstructured.NestedString(fromCUE(secureWeb().ToCue()), "schematic", "cue", "template")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(tpl).To(ContainSubstring("// +final"))

		child := inherit.Level{Name: "team-web",
			Template: "output: spec: template: spec: securityContext: runAsUser: 0\n"}
		own := cuecontext.New().CompileString(child.Template)
		Expect(own.Err()).NotTo(HaveOccurred())

		err = inherit.CheckFinal("team-web", child, own,
			[]inherit.Level{{Name: "secure-web", Template: tpl}}, inherit.ComponentSurface)
		Expect(err).To(MatchError(ContainSubstring(
			"component team-web adds to output.spec.template.spec.securityContext, which secure-web marks +final")))
	})
})
