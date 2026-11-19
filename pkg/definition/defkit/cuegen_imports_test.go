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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("CUEGenerator import isolation", func() {
	// needsStrings has a length constraint, so strings.MinRunes is emitted and
	// "strings" has to be imported.
	needsStrings := func(name string) *defkit.ComponentDefinition {
		return defkit.NewComponent(name).
			Workload("apps/v1", "Deployment").
			Params(defkit.String("s").MinLen(3))
	}
	// clean needs no import at all.
	clean := func(name string) *defkit.ComponentDefinition {
		return defkit.NewComponent(name).
			Workload("apps/v1", "Deployment").
			Params(defkit.String("s"))
	}
	importCount := func(cue, pkg string) int {
		return strings.Count(cue, "\t\""+pkg+"\"\n")
	}

	It("should not leak detected imports into a later definition", func() {
		gen := defkit.NewCUEGenerator()

		first := gen.GenerateFullDefinition(needsStrings("a"))
		Expect(first).To(ContainSubstring("import ("))
		Expect(first).To(ContainSubstring(`"strings"`))

		second := gen.GenerateFullDefinition(clean("b"))
		Expect(second).NotTo(ContainSubstring("import ("))
		Expect(second).NotTo(ContainSubstring(`"strings"`))
	})

	It("should generate the same CUE whether the generator is reused or fresh", func() {
		reused := defkit.NewCUEGenerator()
		reusedFirst := reused.GenerateFullDefinition(needsStrings("a"))
		reusedSecond := reused.GenerateFullDefinition(clean("b"))

		freshFirst := defkit.NewCUEGenerator().GenerateFullDefinition(needsStrings("a"))
		freshSecond := defkit.NewCUEGenerator().GenerateFullDefinition(clean("b"))

		Expect(reusedFirst).To(Equal(freshFirst))
		Expect(reusedSecond).To(Equal(freshSecond))
	})

	It("should keep explicit WithImports across every Generate call", func() {
		gen := defkit.NewCUEGenerator().WithImports("strconv")

		first := gen.GenerateFullDefinition(clean("a"))
		second := gen.GenerateFullDefinition(clean("b"))

		Expect(importCount(first, "strconv")).To(Equal(1))
		Expect(importCount(second, "strconv")).To(Equal(1))
	})

	It("should keep an explicit import that is also detected exactly once", func() {
		gen := defkit.NewCUEGenerator().WithImports("strings")

		cue := gen.GenerateFullDefinition(needsStrings("a"))
		Expect(importCount(cue, "strings")).To(Equal(1))

		// A second definition still sees the explicit import and nothing else.
		second := gen.GenerateFullDefinition(clean("b"))
		Expect(importCount(second, "strings")).To(Equal(1))
	})

	It("should emit explicit imports before detected ones", func() {
		cue := defkit.NewCUEGenerator().
			WithImports("strconv").
			GenerateFullDefinition(needsStrings("a"))

		Expect(cue).To(HavePrefix("import (\n\t\"strconv\"\n\t\"strings\"\n)\n"))
	})
})
