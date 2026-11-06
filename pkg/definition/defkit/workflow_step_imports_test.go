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
	"strings"

	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("WorkflowStep imports", func() {
	importCount := func(w *defkit.WorkflowStepDefinition, pkg string) int {
		return strings.Count(w.ToCue(), "\t\""+pkg+"\"\n")
	}
	// compiles reports whether the generated template compiles with the imports
	// generated for it. Only for steps that call no vela builtin.
	compiles := func(w *defkit.WorkflowStepDefinition) error {
		cueStr := w.ToCue()
		var imports string
		if strings.HasPrefix(cueStr, "import (") {
			imports = cueStr[:strings.Index(cueStr, ")\n")+2]
		}
		return cuecontext.New().CompileString(imports + defkitTemplateOnly(cueStr) + "\ncontext: {}").Err()
	}
	tags := defkit.StringList("tags")

	DescribeTable("adds the imports a step needs",
		func(w *defkit.WorkflowStepDefinition, pkg string) {
			Expect(importCount(w, pkg)).To(Equal(1), w.ToCue())
			Expect(compiles(w)).To(Succeed(), w.ToCue())
		},
		Entry("a parameter constraint",
			defkit.NewWorkflowStep("s").Params(defkit.String("team").MinLen(3)), "strings"),
		Entry("a parameter validator's message",
			defkit.NewWorkflowStep("s").Params(defkit.Object("governance").WithFields(defkit.String("name")).
				Validators(defkit.ValidateValue(defkit.StringsToUpper(defkit.Lit("name required"))).
					WithName("_validateName").
					FailWhen(defkit.LocalField("name").Eq("")))), "strings"),
		Entry("a helper definition's constraint",
			defkit.NewWorkflowStep("s").Helper("Team", defkit.Object("team").WithFields(defkit.String("name").MinLen(3))), "strings"),
		Entry("an array helper's field constraint",
			defkit.NewWorkflowStep("s").Helper("Teams", defkit.Array("teams").WithFields(defkit.String("name").MinLen(3))), "strings"),
		Entry("a value",
			defkit.NewWorkflowStep("s").Template(func(tpl *defkit.WorkflowStepTemplate) {
				tpl.Set("team", defkit.StringsToLower(defkit.Lit("Payments")))
			}), "strings"),
		Entry("a conditional value's condition",
			defkit.NewWorkflowStep("s").Params(tags).Template(func(tpl *defkit.WorkflowStepTemplate) {
				tpl.SetIf(tags.Contains("gpu"), "gpu", defkit.Lit(true))
			}), "list"),
		Entry("a conditional value",
			defkit.NewWorkflowStep("s").Params(tags).Template(func(tpl *defkit.WorkflowStepTemplate) {
				tpl.SetIf(tags.IsNotEmpty(), "team", defkit.StringsToLower(defkit.Lit("Payments")))
			}), "strings"),
		Entry("a guarded block's condition",
			defkit.NewWorkflowStep("s").Params(tags).Template(func(tpl *defkit.WorkflowStepTemplate) {
				tpl.SetGuardedBlock(tags.Contains("gpu"), "gpu", defkit.Lit(true))
			}), "list"),
		Entry("a guarded block's value",
			defkit.NewWorkflowStep("s").Params(tags).Template(func(tpl *defkit.WorkflowStepTemplate) {
				tpl.SetGuardedBlock(tags.IsNotEmpty(), "team", defkit.StringsToLower(defkit.Lit("Payments")))
			}), "strings"),
		Entry("an import also asked for with WithImports",
			defkit.NewWorkflowStep("s").WithImports("strings").Params(defkit.String("team").MinLen(3)), "strings"),
	)

	// Builtin calls reference vela packages, so these only count imports.
	DescribeTable("adds the imports a builtin call needs",
		func(w *defkit.WorkflowStepDefinition, pkg string) {
			Expect(importCount(w, pkg)).To(Equal(1), w.ToCue())
		},
		Entry("a builtin parameter",
			defkit.NewWorkflowStep("s").WithImports("vela/builtin").Template(func(tpl *defkit.WorkflowStepTemplate) {
				tpl.Builtin("print", "builtin.#Message").
					WithParams(map[string]defkit.Value{"message": defkit.StringsToLower(defkit.Lit("Hi"))}).Build()
			}), "strings"),
		Entry("a builtin call's condition",
			defkit.NewWorkflowStep("s").WithImports("vela/builtin").Params(tags).Template(func(tpl *defkit.WorkflowStepTemplate) {
				tpl.Builtin("print", "builtin.#Message").
					WithParams(map[string]defkit.Value{"message": defkit.Lit("hi")}).If(tags.Contains("gpu"))
			}), "list"),
		Entry("the builtin's own import, kept beside a detected one",
			defkit.NewWorkflowStep("s").WithImports("vela/builtin").Template(func(tpl *defkit.WorkflowStepTemplate) {
				tpl.Builtin("print", "builtin.#Message").
					WithParams(map[string]defkit.Value{"message": defkit.StringsToLower(defkit.Lit("Hi"))}).Build()
			}), "vela/builtin"),
	)

	// A scalar helper renders as `_`, so its constraint needs no import, and an
	// unused import would not compile.
	It("adds no import for a constraint a helper does not render", func() {
		w := defkit.NewWorkflowStep("s").Helper("Team", defkit.String("team").MinLen(3))
		Expect(w.ToCue()).NotTo(ContainSubstring("import"))
		Expect(compiles(w)).To(Succeed(), w.ToCue())
	})

	It("adds no import for builtin params a full-parameter call does not render", func() {
		w := defkit.NewWorkflowStep("s").WithImports("vela/builtin").Template(func(tpl *defkit.WorkflowStepTemplate) {
			tpl.Builtin("print", "builtin.#Message").
				WithParams(map[string]defkit.Value{"message": defkit.StringsToLower(defkit.Lit("Hi"))}).
				WithFullParameter().Build()
		})
		Expect(importCount(w, "strings")).To(Equal(0), w.ToCue())
	})

	It("adds no import a step does not need", func() {
		w := defkit.NewWorkflowStep("s").Params(defkit.String("team"))
		Expect(w.ToCue()).NotTo(ContainSubstring("import"))
	})
})
