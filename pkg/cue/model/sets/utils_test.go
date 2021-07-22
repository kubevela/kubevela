/*
Copyright 2021 The KubeVela Authors.

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
