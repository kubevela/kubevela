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

package docgen

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("a list parameter with a size bound", func() {
	// Written with escapes: each line of the table header ends in a space.
	const header = " Name | Description | Type | Required | Default | Immutable \n" +
		" ---- | ----------- | ---- | -------- | ------- | --------- \n"
	// What the table shows for a value CUE evaluates to bottom.
	const bottom = "_&#124;_"

	// table renders the parameter table of a template the way vela show does.
	table := func(template string) string {
		GinkgoHelper()
		_, err := parser.ParseFile("fixture.cue", template)
		Expect(err).NotTo(HaveOccurred(), "the fixture is not valid CUE")
		value := cuecontext.New().CompileString(template).LookupPath(cue.ParsePath("parameter"))
		Expect(value.Exists()).To(BeTrue(), "the fixture has no parameter block")
		out, _, err := (&MarkdownReference{}).parseParameters("", value, "list", 0, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).NotTo(ContainSubstring("Unsupported Composition Type"))
		return out
	}

	DescribeTable("is shown as the list it is",
		func(template string, rows ...string) {
			out := table(template)
			for _, row := range rows {
				Expect(out).To(ContainSubstring(row))
			}
			Expect(out).NotTo(ContainSubstring(bottom))
		},
		Entry("a required list of structs with a minimum",
			`import "list"

parameter: {
	attributeDefinitions!: list.MinItems(1) & [...{attributeName!: string, attributeType!: "S" | "N" | "B"}]
}`,
			header+` attributeDefinitions |  | [[]attributeDefinitions](#attributedefinitions) | true |  |  `,
			"#### attributeDefinitions\n\n"+header+" attributeName |  | string | true |  |  \n attributeType |  | \"S\" or \"N\" or \"B\" | true |  |  ",
		),
		Entry("a required list of a definition with a minimum and a maximum",
			`import "list"

#KeySchemaElement: {
	attributeName!: string
	keyType!:       "HASH" | "RANGE"
}
parameter: {
	keySchema!: list.MinItems(1) & list.MaxItems(2) & [...#KeySchemaElement]
}`,
			header+` keySchema |  | [[]keySchema](#keyschema) | true |  |  `,
			"#### keySchema\n\n"+header+" attributeName |  | string | true |  |  \n keyType |  | \"HASH\" or \"RANGE\" | true |  |  ",
		),
		Entry("an optional list written before its bounds",
			`import "list"

parameter: {
	tags?: [...{key: string}] & list.MaxItems(3) & list.MinItems(1)
}`,
			header+` tags |  | [[]tags](#tags) | false |  |  `,
			"#### tags\n\n"+header+" key |  | string | true |  |  ",
		),
		Entry("a list of scalars with a minimum",
			`import "list"

parameter: {
	names: list.MinItems(2) & [...string]
}`,
			header+` names |  | []string | true |  |  `,
		),
		Entry("a list with only a maximum",
			`import "list"

parameter: {
	names: list.MaxItems(2) & [...string]
}`,
			header+` names |  | []string | true |  |  `,
		),
		Entry("a list nested in a struct",
			`import "list"

parameter: {
	table: indexes!: list.MinItems(1) & [...{name: string}]
}`,
			` indexes |  | [[]indexes](#indexes) | true |  |  `,
			"##### indexes\n\n"+header+" name |  | string | true |  |  ",
		),
		Entry("the list package imported under another name",
			`import l "list"

parameter: {
	names: l.MinItems(1) & [...string]
}`,
			header+` names |  | []string | true |  |  `,
		),
	)

	It("still shows bounds that no list can satisfy as bottom", func() {
		out := table(`import "list"

parameter: {
	names: list.MinItems(3) & list.MaxItems(2) & [...string]
}`)
		Expect(out).To(ContainSubstring(header + ` names |  | ` + bottom + ` | true |  |  `))
		Expect(out).NotTo(ContainSubstring("[]string"))
	})

	Describe("looking through the bound to the list", func() {
		const (
			// What CUE reports for an open list under a minimum size.
			tooShort = "does not satisfy list.MinItems"
			// What CUE reports for operands that cannot be unified.
			conflict = "conflicting values"
			// The fixture compiles without an error.
			valid = ""
		)

		// field compiles one field on its own: a conflict would otherwise make
		// the struct around it bottom. Most fixtures are bottom on purpose, so
		// each one names the error it is expected to carry. A typo, such as a
		// function that does not exist, carries a different error and fails
		// here instead of passing as a value that is "not a list".
		field := func(imports, expr, wantErr string) cue.Value {
			GinkgoHelper()
			src := imports + "\n\nfield: " + expr
			_, err := parser.ParseFile("fixture.cue", src)
			Expect(err).NotTo(HaveOccurred(), "the fixture is not valid CUE")
			value := cuecontext.New().CompileString(src).LookupPath(cue.ParsePath("field"))
			Expect(value.Exists()).To(BeTrue(), "the fixture has no field")
			if wantErr == valid {
				Expect(value.Err()).NotTo(HaveOccurred(), "the fixture does not compile")
			} else {
				Expect(value.Err()).To(MatchError(ContainSubstring(wantErr)), "the fixture is bottom for another reason than intended")
			}
			return value
		}

		DescribeTable("finds the list",
			func(imports, expr string) {
				got, ok := listBehindSizeValidator(field(imports, expr, tooShort))
				Expect(ok).To(BeTrue())
				Expect(got.IncompleteKind()).To(Equal(cue.ListKind))
				element := got.LookupPath(cue.MakePath(cue.AnyIndex))
				Expect(element.Exists()).To(BeTrue())
				Expect(element.IncompleteKind()).NotTo(Equal(cue.BottomKind))
			},
			Entry("behind a minimum", `import "list"`, `list.MinItems(1) & [...string]`),
			Entry("behind a minimum and a maximum", `import "list"`, `list.MinItems(1) & list.MaxItems(2) & [...string]`),
			Entry("written before the bound", `import "list"`, `[...string] & list.MinItems(1)`),
			Entry("as two lists that agree", `import "list"`, `[...string] & list.MinItems(1) & [...=~"^a"]`),
			Entry("with the package under another name", `import l "list"`, `l.MinItems(1) & l.MaxItems(2) & [...string]`),
		)

		DescribeTable("leaves everything else as it is",
			func(imports, expr, wantErr string) {
				_, ok := listBehindSizeValidator(field(imports, expr, wantErr))
				Expect(ok).To(BeFalse())
			},
			Entry("a list without a bound", ``, `[...string]`, valid),
			Entry("a maximum that an empty list already satisfies", `import "list"`, `list.MaxItems(2) & [...string]`, valid),
			Entry("a concrete list that is too short", `import "list"`, `list.MinItems(2) & [...string] & ["a"]`, tooShort),
			Entry("a minimum above the maximum", `import "list"`, `list.MinItems(3) & list.MaxItems(2) & [...string]`, tooShort),
			Entry("a bound without a list", `import "list"`, `list.MinItems(1)`, valid),
			Entry("another validator of the list package", `import "list"`, `list.UniqueItems() & list.MinItems(1) & [...string]`, tooShort),
			Entry("scalars that conflict", ``, `string & int`, conflict),
			Entry("two lists whose element types conflict", `import "list"`, `list.MinItems(1) & [...string] & [...int]`, tooShort),
			Entry("a function of the same name from another package", `import "strings"`, `strings.MinRunes(1) & [...string]`, conflict),
		)
	})
})
