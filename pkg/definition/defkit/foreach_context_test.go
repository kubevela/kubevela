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

var _ = Describe("Workflow step context", func() {
	vela := defkit.VelaCtx()

	It("reads the forEach pass and the running step", func() {
		Expect(vela.Each().Item().Path()).To(Equal("context.each.item"))
		Expect(vela.Each().ItemField("spec.name").Path()).To(Equal("context.each.item.spec.name"))
		Expect(vela.Each().Index().Path()).To(Equal("context.each.index"))
		Expect(vela.StepName().Path()).To(Equal("context.stepName"))
		Expect(vela.StepSessionID().Path()).To(Equal("context.stepSessionID"))
		Expect(vela.WorkflowName().Path()).To(Equal("context.workflowName"))
	})

	It("guards a read of the pass, which is absent outside a forEach", func() {
		w := defkit.NewWorkflowStep("notify").
			Template(func(tpl *defkit.WorkflowStepTemplate) {
				tpl.SetIf(vela.Each().IsSet(), "target", vela.Each().ItemField("name"))
			})
		Expect(w.ToCue()).To(ContainSubstring("if context.each != _|_"))
		Expect(w.ToCue()).To(ContainSubstring("context.each.item.name"))
		Expect(defkit.ValidateGeneratedCUE(w)).To(Succeed())
	})
})
