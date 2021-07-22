package sets

import (
	"testing"

	"cuelang.org/go/cue"
	"gotest.tools/assert"
)

func TestToString(t *testing.T) {
	testCases := []struct {
		s        string
		expected string
	}{
		{
			s: `
foo: int
lacy: string
`,
			expected: `foo:  int
lacy: string
`},
		{
			s: ` import "strconv"
foo: strconv.Atoi("100")
lacy: string
`,
			expected: `foo:  100
lacy: string
`},
		{
			s: ` 
if true {
	foo: int
}
lacy: string
`,
			expected: `foo:  int
lacy: string
`},
		{
			s: ` 
foo: int
if foo>5{
lacy: "=5"
}
`,
			expected: `foo: int
if foo > 5 {
	lacy: "=5"
}
`},
	}
	var r cue.Runtime
	for _, tcase := range testCases {
		inst, err := r.Compile("-", tcase.s)
		assert.NilError(t, err)
		str, err := ToString(inst.Value())
		assert.NilError(t, err)
		assert.Equal(t, str, tcase.expected)
	}
}
