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
	"encoding/json"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

// The guarded conditions (Contains on an array, a length check, IsEmpty)
// render as a chained if, which is valid only as a whole comprehension clause
// or joined to another clause by AND. Under Or or Not, or beside IsEmpty in an
// AND, they must still compile and evaluate, including when the parameter is
// absent.
var _ = Describe("Guarded conditions as operands", func() {
	tags := defkit.StringList("tags")

	DescribeTable("compile and evaluate in an if clause",
		func(cond defkit.Condition, parameter map[string]any, want bool) {
			comp := defkit.NewComponent("c").
				Workload("v1", "ConfigMap").
				Params(defkit.StringList("tags").Optional(), defkit.Bool("on")).
				Template(func(tpl *defkit.Template) {
					tpl.Output(defkit.NewResource("v1", "ConfigMap").SetIf(cond, "data.hit", defkit.Lit("yes")))
				})
			cueStr := comp.ToCue()
			raw, err := json.Marshal(parameter)
			Expect(err).NotTo(HaveOccurred())
			v := cuecontext.New().CompileString(cueStr + "\ntemplate: parameter: " + string(raw))
			Expect(v.Err()).NotTo(HaveOccurred(), cueStr)
			// A condition that errors leaves the field absent without a root error, so the
			// output must evaluate concretely before absence counts as false.
			Expect(v.LookupPath(cue.ParsePath("template.output")).Validate(cue.Concrete(true))).To(Succeed(), cueStr)
			Expect(v.LookupPath(cue.ParsePath("template.output.data.hit")).Exists()).To(Equal(want), cueStr)
		},
		Entry("Or with Contains, matched", defkit.Or(tags.Contains("gpu"), defkit.Bool("on").IsTrue()),
			map[string]any{"tags": []string{"gpu"}, "on": false}, true),
		Entry("Or with Contains, absent parameter", defkit.Or(tags.Contains("gpu"), defkit.Bool("on").IsTrue()),
			map[string]any{"on": false}, false),
		Entry("Or with Contains, other operand", defkit.Or(tags.Contains("gpu"), defkit.Bool("on").IsTrue()),
			map[string]any{"on": true}, true),
		Entry("Not Contains, absent parameter", defkit.Not(tags.Contains("gpu")),
			map[string]any{"on": false}, true),
		Entry("Not Contains, matched", defkit.Not(tags.Contains("gpu")),
			map[string]any{"tags": []string{"gpu"}, "on": false}, false),
		Entry("Not LenGt, absent parameter", defkit.Not(tags.LenGt(0)),
			map[string]any{"on": false}, true),
		Entry("And with IsEmpty, absent parameter", defkit.And(tags.IsEmpty(), defkit.Bool("on").IsTrue()),
			map[string]any{"on": true}, true),
		Entry("And with IsEmpty, empty", defkit.And(tags.IsEmpty(), defkit.Bool("on").IsTrue()),
			map[string]any{"tags": []string{}, "on": true}, true),
		Entry("And with IsEmpty, not empty", defkit.And(tags.IsEmpty(), defkit.Bool("on").IsTrue()),
			map[string]any{"tags": []string{"gpu"}, "on": true}, false),
		Entry("AllConditions with IsEmpty, absent parameter", defkit.AllConditions(tags.IsEmpty(), defkit.Bool("on").IsTrue()),
			map[string]any{"on": true}, true),
		Entry("Or of AllConditions with Contains", defkit.Or(defkit.AllConditions(tags.Contains("gpu"), tags.LenGt(1)), defkit.Bool("on").IsTrue()),
			map[string]any{"tags": []string{"gpu", "x"}, "on": false}, true),
		Entry("Or of AllConditions with Contains, other operand", defkit.Or(defkit.AllConditions(tags.Contains("gpu"), tags.LenGt(1)), defkit.Bool("on").IsTrue()),
			map[string]any{"on": true}, true),
		Entry("Or of an And with Contains", defkit.Or(defkit.And(tags.Contains("gpu"), tags.LenGt(1)), defkit.Bool("on").IsTrue()),
			map[string]any{"tags": []string{"gpu", "x"}, "on": false}, true),
	)

	// An If block's condition is combined with each inner SetIf's condition
	// at render time.
	DescribeTable("compile and evaluate as an If block's condition",
		func(cond defkit.Condition, parameter map[string]any, want bool) {
			comp := defkit.NewComponent("c").
				Workload("v1", "ConfigMap").
				Params(defkit.StringList("tags").Optional(), defkit.Bool("on")).
				Template(func(tpl *defkit.Template) {
					tpl.Output(defkit.NewResource("v1", "ConfigMap").
						If(cond).SetIf(defkit.Bool("on").IsTrue(), "data.hit", defkit.Lit("yes")).EndIf())
				})
			cueStr := comp.ToCue()
			raw, err := json.Marshal(parameter)
			Expect(err).NotTo(HaveOccurred())
			v := cuecontext.New().CompileString(cueStr + "\ntemplate: parameter: " + string(raw))
			Expect(v.Err()).NotTo(HaveOccurred(), cueStr)
			// A condition that errors leaves the field absent without a root error, so the
			// output must evaluate concretely before absence counts as false.
			Expect(v.LookupPath(cue.ParsePath("template.output")).Validate(cue.Concrete(true))).To(Succeed(), cueStr)
			Expect(v.LookupPath(cue.ParsePath("template.output.data.hit")).Exists()).To(Equal(want), cueStr)
		},
		Entry("IsEmpty, absent parameter", tags.IsEmpty(), map[string]any{"on": true}, true),
		Entry("IsEmpty, not empty", tags.IsEmpty(), map[string]any{"tags": []string{"gpu"}, "on": true}, false),
	)
})

// An OR operand of an AND keeps its grouping: CUE binds && tighter than ||, so
// `a || b && c` would read as `a || (b && c)`.
var _ = Describe("OR operands of an AND", func() {
	a, b, c := defkit.Bool("a"), defkit.Bool("b"), defkit.Bool("c")
	mode := defkit.String("mode")

	DescribeTable("evaluate with OR grouped",
		func(cond defkit.Condition, parameter map[string]any, want bool) {
			comp := defkit.NewComponent("c").
				Workload("v1", "ConfigMap").
				Params(defkit.Bool("a"), defkit.Bool("b"), defkit.Bool("c"), defkit.String("mode")).
				Template(func(tpl *defkit.Template) {
					tpl.Output(defkit.NewResource("v1", "ConfigMap").SetIf(cond, "data.hit", defkit.Lit("yes")))
				})
			cueStr := comp.ToCue()
			raw, err := json.Marshal(parameter)
			Expect(err).NotTo(HaveOccurred())
			v := cuecontext.New().CompileString(cueStr + "\ntemplate: parameter: " + string(raw))
			Expect(v.Err()).NotTo(HaveOccurred(), cueStr)
			// A condition that errors leaves the field absent without a root error, so the
			// output must evaluate concretely before absence counts as false.
			Expect(v.LookupPath(cue.ParsePath("template.output")).Validate(cue.Concrete(true))).To(Succeed(), cueStr)
			Expect(v.LookupPath(cue.ParsePath("template.output.data.hit")).Exists()).To(Equal(want), cueStr)
		},
		Entry("And(Or(a, b), c) with c false", defkit.And(defkit.Or(a.IsTrue(), b.IsTrue()), c.IsTrue()),
			map[string]any{"a": true, "b": false, "c": false, "mode": "x"}, false),
		Entry("And(Or(a, b), c) with c true", defkit.And(defkit.Or(a.IsTrue(), b.IsTrue()), c.IsTrue()),
			map[string]any{"a": true, "b": false, "c": true, "mode": "x"}, true),
		Entry("And(In, c) with c false", defkit.And(mode.In("x", "y"), c.IsTrue()),
			map[string]any{"a": false, "b": false, "c": false, "mode": "x"}, false),
		Entry("AllConditions(Or(a, b), c) with c false", defkit.AllConditions(defkit.Or(a.IsTrue(), b.IsTrue()), c.IsTrue()),
			map[string]any{"a": true, "b": false, "c": false, "mode": "x"}, false),
	)
})
