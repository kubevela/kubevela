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

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("Policy imports", func() {
	// compiles reports whether the generated template compiles with the imports
	// generated for it, as the controller compiles it.
	compiles := func(p *defkit.PolicyDefinition) error {
		cueStr := p.ToCue()
		var imports string
		if strings.HasPrefix(cueStr, "import (") {
			imports = cueStr[:strings.Index(cueStr, ")\n")+2]
		}
		v := cuecontext.New().CompileString(imports + defkitTemplateOnly(cueStr) + "\ncontext: {}")
		return v.Err()
	}
	importCount := func(p *defkit.PolicyDefinition, pkg string) int {
		return strings.Count(p.ToCue(), "\t\""+pkg+"\"\n")
	}

	DescribeTable("adds the imports a policy needs",
		func(p *defkit.PolicyDefinition, pkg string) {
			Expect(importCount(p, pkg)).To(Equal(1), p.ToCue())
			Expect(compiles(p)).To(Succeed(), p.ToCue())
		},
		Entry("a parameter constraint",
			defkit.NewPolicy("p").Params(defkit.String("team").MinLen(3)), "strings"),
		Entry("a context schema constraint",
			defkit.NewPolicy("p").ApplicationScoped().
				CtxSchema(defkit.String("team").MinLen(3)).
				Template(func(tpl *defkit.PolicyTemplate) {
					tpl.Output().Ctx("team", defkit.Lit("payments"))
				}), "strings"),
		Entry("an output value",
			defkit.NewPolicy("p").ApplicationScoped().
				Template(func(tpl *defkit.PolicyTemplate) {
					tpl.Output().Ctx("team", defkit.StringsToLower(defkit.Lit("Payments")))
				}), "strings"),
		Entry("a computed field",
			defkit.NewPolicy("p").
				Template(func(tpl *defkit.PolicyTemplate) {
					tpl.Set("team", defkit.StringsToLower(defkit.Lit("Payments")))
				}), "strings"),
		Entry("a parameter validator's message",
			defkit.NewPolicy("p").Params(defkit.Object("governance").WithFields(defkit.String("name")).
				Validators(defkit.ValidateValue(defkit.StringsToUpper(defkit.Lit("name required"))).
					WithName("_validateName").
					FailWhen(defkit.LocalField("name").Eq("")))), "strings"),
		Entry("an import also asked for with WithImports",
			defkit.NewPolicy("p").WithImports("strings").Params(defkit.String("team").MinLen(3)), "strings"),
	)

	// enabledPolicy is an Application-scoped policy enabled by cond.
	enabledPolicy := func(cond defkit.Condition, params ...defkit.Param) *defkit.PolicyDefinition {
		return defkit.NewPolicy("p").ApplicationScoped().
			Params(params...).
			Template(func(tpl *defkit.PolicyTemplate) {
				tpl.Enabled(cond)
				tpl.Output().Label("gpu", defkit.Lit("true"))
			})
	}

	DescribeTable("adds the imports the enabled condition needs",
		func(p *defkit.PolicyDefinition, pkg string) {
			Expect(importCount(p, pkg)).To(Equal(1), p.ToCue())
			Expect(compiles(p)).To(Succeed(), p.ToCue())
		},
		Entry("an array Contains",
			enabledPolicy(defkit.StringList("tags").Contains("gpu"), defkit.StringList("tags")), "list"),
		Entry("a string Contains",
			enabledPolicy(defkit.String("team").Contains("pay"), defkit.String("team")), "strings"),
		Entry("a string StartsWith",
			enabledPolicy(defkit.String("team").StartsWith("pay"), defkit.String("team")), "strings"),
		Entry("a string EndsWith",
			enabledPolicy(defkit.String("team").EndsWith("pay"), defkit.String("team")), "strings"),
		Entry("a comparison of a function call",
			enabledPolicy(defkit.Eq(defkit.StringsToLower(defkit.Lit("GPU")), defkit.Lit("gpu"))), "strings"),
		Entry("a comparison with a function call on the right",
			enabledPolicy(defkit.Eq(defkit.Lit("gpu"), defkit.StringsToLower(defkit.Lit("GPU")))), "strings"),
		Entry("a function call under And and Not",
			enabledPolicy(defkit.And(defkit.Not(defkit.Ne(defkit.StringsToLower(defkit.Lit("GPU")), defkit.Lit("gpu"))))), "strings"),
		Entry("a function call under AllConditions",
			enabledPolicy(defkit.AllConditions(defkit.Eq(defkit.StringsToLower(defkit.Lit("GPU")), defkit.Lit("gpu")))), "strings"),
		Entry("a function call under Or",
			enabledPolicy(defkit.Or(defkit.Eq(defkit.StringsToLower(defkit.Lit("GPU")), defkit.Lit("gpu")))), "strings"),
		Entry("the length of a function call",
			enabledPolicy(defkit.LenGt(defkit.StringsToLower(defkit.Lit("GPU")), 0)), "strings"),
		Entry("exposed ports of a function call",
			enabledPolicy(defkit.HasExposedPorts(defkit.ListConcat(defkit.Lit([]any{[]any{}})))), "list"),
		Entry("a regex match on a function call",
			enabledPolicy(defkit.RegexMatch(defkit.StringsToLower(defkit.Lit("GPU")), "^gpu$")), "strings"),
	)

	// The guarded forms (Contains on an array, a length check) render as a
	// chained if, which is only valid as a comprehension clause. As the value
	// of config.enabled they must still evaluate, including when the parameter
	// is absent.
	DescribeTable("renders a guarded enabled condition as a value",
		func(cond defkit.Condition, param defkit.Param, parameterJSON string, want bool) {
			p := enabledPolicy(cond, param)
			cueStr := p.ToCue()
			var imports string
			if strings.HasPrefix(cueStr, "import (") {
				imports = cueStr[:strings.Index(cueStr, ")\n")+2]
			}
			v := cuecontext.New().CompileString(imports + defkitTemplateOnly(cueStr) + "\ncontext: {}\nparameter: " + parameterJSON)
			Expect(v.Err()).NotTo(HaveOccurred(), cueStr)
			got, err := v.LookupPath(cue.ParsePath("config.enabled")).Bool()
			Expect(err).NotTo(HaveOccurred(), cueStr)
			Expect(got).To(Equal(want), cueStr)
		},
		Entry("Contains, present", defkit.StringList("tags").Contains("gpu"), defkit.StringList("tags"), `{tags: ["gpu"]}`, true),
		Entry("Contains, missing value", defkit.StringList("tags").Contains("gpu"), defkit.StringList("tags"), `{tags: ["cpu"]}`, false),
		Entry("Contains, absent parameter", defkit.StringList("tags").Contains("gpu"), defkit.StringList("tags"), `{}`, false),
		Entry("LenGt, long enough", defkit.StringList("tags").LenGt(0), defkit.StringList("tags"), `{tags: ["gpu"]}`, true),
		Entry("LenGt, absent parameter", defkit.StringList("tags").LenGt(0), defkit.StringList("tags"), `{}`, false),
		Entry("IsEmpty, empty", defkit.StringList("tags").IsEmpty(), defkit.StringList("tags"), `{tags: []}`, true),
		Entry("And with Contains", defkit.And(defkit.StringList("tags").Contains("gpu"), defkit.StringList("tags").LenGt(1)),
			defkit.StringList("tags"), `{tags: ["gpu", "x"]}`, true),
		Entry("And with Contains, one false", defkit.And(defkit.StringList("tags").Contains("gpu"), defkit.StringList("tags").LenGt(1)),
			defkit.StringList("tags"), `{tags: ["gpu"]}`, false),
		Entry("Not Contains, absent parameter", defkit.Not(defkit.StringList("tags").Contains("gpu")),
			defkit.StringList("tags"), `{}`, true),
	)

	It("adds no import a policy does not need", func() {
		p := defkit.NewPolicy("p").Params(defkit.String("team"))
		Expect(p.ToCue()).NotTo(ContainSubstring("import"))
	})

	It("writes the imports of computed fields in the same order every time", func() {
		gen := func() string {
			return defkit.NewPolicy("ordered").ApplicationScoped().Template(func(tpl *defkit.PolicyTemplate) {
				tpl.Set("a", defkit.StrconvFormatInt(defkit.Lit(1), 10))
				tpl.Set("b", defkit.StringsToLower(defkit.Lit("X")))
				tpl.Set("c", defkit.StrconvFormatInt(defkit.Lit(2), 10))
				tpl.Set("d", defkit.StringsToUpper(defkit.Lit("y")))
			}).ToCue()
		}
		first := gen()
		Expect(first).To(ContainSubstring("\"strconv\"\n\t\"strings\""))
		for i := 0; i < 50; i++ {
			Expect(gen()).To(Equal(first))
		}
	})
})
