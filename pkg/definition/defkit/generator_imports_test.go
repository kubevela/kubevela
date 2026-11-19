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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

// generate renders one definition named name with a reused generator. When
// needsStrings is true the definition has a String("s").MinLen(3) param, so
// strings.MinRunes is emitted and "strings" has to be imported; otherwise it
// needs no import at all.
type generate func(needsStrings bool, name string) string

// sharesNoImports registers the import isolation specs for one generator.
// newGen returns a generate func bound to a single new generator created with
// the given explicit imports, so every call of the returned func reuses it.
func sharesNoImports(newGen func(explicit ...string) generate) {
	importCount := func(cue, pkg string) int {
		return strings.Count(cue, "\t\""+pkg+"\"\n")
	}

	It("should not leak detected imports into a later definition", func() {
		gen := newGen()

		first := gen(true, "a")
		Expect(importCount(first, "strings")).To(Equal(1))

		second := gen(false, "b")
		Expect(second).NotTo(ContainSubstring("import ("))
		Expect(second).NotTo(ContainSubstring(`"strings"`))
	})

	It("should generate the same CUE whether the generator is reused or fresh", func() {
		reused := newGen()
		reusedFirst := reused(true, "a")
		reusedSecond := reused(false, "b")

		freshFirst := newGen()(true, "a")
		freshSecond := newGen()(false, "b")

		Expect(reusedFirst).To(Equal(freshFirst))
		Expect(reusedSecond).To(Equal(freshSecond))
	})

	It("should keep explicit WithImports across every Generate call", func() {
		gen := newGen("strconv")

		first := gen(true, "a")
		second := gen(false, "b")
		third := gen(false, "c")

		Expect(importCount(first, "strconv")).To(Equal(1))
		Expect(importCount(second, "strconv")).To(Equal(1))
		Expect(importCount(third, "strconv")).To(Equal(1))
		Expect(second).NotTo(ContainSubstring(`"strings"`))
	})

	It("should keep an explicit import that is also detected exactly once", func() {
		gen := newGen("strings")

		first := gen(true, "a")
		Expect(importCount(first, "strings")).To(Equal(1))

		// A second definition still sees the explicit import and nothing else.
		second := gen(false, "b")
		Expect(second).To(HavePrefix("import (\n\t\"strings\"\n)\n"))
	})
}

var _ = Describe("Sibling generator import isolation", func() {
	param := func(needsStrings bool) defkit.Param {
		if needsStrings {
			return defkit.String("s").MinLen(3)
		}
		return defkit.String("s")
	}
	trait := func(needsStrings bool, name string) *defkit.TraitDefinition {
		return defkit.NewTrait(name).AppliesTo("deployments.apps").Params(param(needsStrings))
	}

	Describe("PolicyCUEGenerator", func() {
		sharesNoImports(func(explicit ...string) generate {
			gen := defkit.NewPolicyCUEGenerator().WithImports(explicit...)
			return func(needsStrings bool, name string) string {
				return gen.GenerateFullDefinition(defkit.NewPolicy(name).Params(param(needsStrings)))
			}
		})
	})

	Describe("WorkflowStepCUEGenerator", func() {
		sharesNoImports(func(explicit ...string) generate {
			gen := defkit.NewWorkflowStepCUEGenerator().WithImports(explicit...)
			return func(needsStrings bool, name string) string {
				return gen.GenerateFullDefinition(defkit.NewWorkflowStep(name).Params(param(needsStrings)))
			}
		})
	})

	Describe("TraitCUEGenerator.GenerateFullDefinition", func() {
		sharesNoImports(func(explicit ...string) generate {
			gen := defkit.NewTraitCUEGenerator().WithImports(explicit...)
			return func(needsStrings bool, name string) string {
				return gen.GenerateFullDefinition(trait(needsStrings, name))
			}
		})
	})

	// GenerateDefinitionWithRawTemplate detects nothing itself, so it should
	// emit exactly the explicit imports, even after GenerateFullDefinition
	// detected some on the same generator. A definition that needs "strings"
	// goes through GenerateFullDefinition and a clean one through the raw path.
	Describe("TraitCUEGenerator.GenerateDefinitionWithRawTemplate", func() {
		const raw = "patch: {}\nparameter: {}"
		sharesNoImports(func(explicit ...string) generate {
			gen := defkit.NewTraitCUEGenerator().WithImports(explicit...)
			return func(needsStrings bool, name string) string {
				if needsStrings {
					return gen.GenerateFullDefinition(trait(true, name))
				}
				return gen.GenerateDefinitionWithRawTemplate(trait(false, name), raw)
			}
		})
	})
})
