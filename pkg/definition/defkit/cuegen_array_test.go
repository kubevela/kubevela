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

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("CUEGenerator indexed array rendering", func() {
	multiIndexComponent := func(conditional bool) *defkit.ComponentDefinition {
		hashKey := defkit.Struct("hashKey").WithFields(defkit.Field("name", defkit.ParamTypeString))
		rangeKey := defkit.Struct("rangeKey").Optional().WithFields(defkit.Field("name", defkit.ParamTypeString))
		return defkit.NewComponent("multi-index").
			Description("x").
			Workload("example.com/v1", "Thing").
			Params(hashKey, rangeKey).
			Template(func(tpl *defkit.Template) {
				hk := defkit.Struct("hashKey")
				rk := defkit.Struct("rangeKey")
				r := defkit.NewResource("example.com/v1", "Thing").
					Set("spec.keySchema[0].attributeName", hk.Field("name")).
					Set("spec.keySchema[0].keyType", defkit.Lit("HASH"))
				if conditional {
					r = r.SetIf(rk.IsSet(), "spec.keySchema[1].attributeName", rk.Field("name")).
						SetIf(rk.IsSet(), "spec.keySchema[1].keyType", defkit.Lit("RANGE"))
				} else {
					r = r.Set("spec.keySchema[1].attributeName", rk.Field("name")).
						Set("spec.keySchema[1].keyType", defkit.Lit("RANGE"))
				}
				tpl.Output(r)
			})
	}

	// extractTemplateBody pulls the body of the generated `template: {...}`
	// block so it can be evaluated with a parameter struct prepended.
	extractTemplateBody := func(full string) string {
		idx := strings.Index(full, "template: {")
		Expect(idx).To(BeNumerically(">=", 0))
		body := full[idx+len("template: {"):]
		j := strings.LastIndex(body, "}")
		Expect(j).To(BeNumerically(">=", 0))
		return body[:j]
	}

	It("renders every array index, not only [0]", func() {
		out := multiIndexComponent(false).ToCue()
		Expect(out).To(ContainSubstring(`"HASH"`))
		Expect(out).To(ContainSubstring(`"RANGE"`), "second array element must not be dropped")
	})

	It("renders primitive array elements, not empty structs", func() {
		comp := defkit.NewComponent("ports").
			Description("x").
			Workload("example.com/v1", "Thing").
			Params(defkit.String("p")).
			Template(func(tpl *defkit.Template) {
				tpl.Output(defkit.NewResource("example.com/v1", "Thing").
					Set("spec.ports[0]", defkit.Lit(8080)).
					Set("spec.ports[1]", defkit.Lit(9090)))
			})
		out := comp.ToCue()
		Expect(out).To(ContainSubstring("8080"))
		Expect(out).To(ContainSubstring("9090"))
		Expect(out).NotTo(ContainSubstring("ports: [{"), "primitive elements must not render as structs")
	})

	It("fails generation loudly on sparse array indices", func() {
		comp := defkit.NewComponent("sparse").
			Description("x").
			Workload("example.com/v1", "Thing").
			Params(defkit.String("p")).
			Template(func(tpl *defkit.Template) {
				tpl.Output(defkit.NewResource("example.com/v1", "Thing").
					Set("spec.items[0]", defkit.Lit("a")).
					Set("spec.items[2]", defkit.Lit("c")))
			})
		Expect(func() { comp.ToCue() }).To(PanicWith(ContainSubstring("non-contiguous")))
	})

	It("guards a conditional element and drops it only when the condition is false", func() {
		out := multiIndexComponent(true).ToCue()
		Expect(out).To(ContainSubstring(`"RANGE"`), "conditional second element must not be dropped")
		Expect(out).To(ContainSubstring(`if parameter["rangeKey"] != _|_`))

		tmpl := extractTemplateBody(out)
		ctx := cuecontext.New()
		for params, want := range map[string]int64{
			`parameter: {hashKey: {name: "id"}}`:                         1,
			`parameter: {hashKey: {name: "id"}, rangeKey: {name: "ts"}}`: 2,
		} {
			v := ctx.CompileString(params + "\n" + tmpl)
			Expect(v.Err()).NotTo(HaveOccurred(), "generated template must compile")
			l, err := v.LookupPath(cue.ParsePath("output.spec.keySchema")).Len().Int64()
			Expect(err).NotTo(HaveOccurred())
			Expect(l).To(Equal(want), params)
		}
	})
})
