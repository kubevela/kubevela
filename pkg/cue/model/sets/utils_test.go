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

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/parser"
	"github.com/pkg/errors"
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
			expected: `lacy: string
foo:  int
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
	for _, tcase := range testCases {
		inst := cuecontext.New().CompileString(tcase.s)
		str, err := ToString(inst)
		assert.NilError(t, err)
		assert.Equal(t, str, tcase.expected)
	}
}

func TestOptBytesToString(t *testing.T) {
	testCases := []struct {
		s        string
		expected string
	}{
		{
			s: `
import "encoding/base64"
foo: int
lacy: base64.Decode(null,base64.Encode(null,"abc"))
`,
			expected: `foo:  int
lacy: "abc"
`},
		{
			s: `
foo: int
lacy: 'xxx==vv-'
`,
			expected: `foo:  int
lacy: "xxx==vv-"
`},
		{
			s: `
foo: int
lacy: "123456"
`,
			expected: `foo:  int
lacy: "123456"
`},
		{
			s: `
foo: int
lacy: #"""
abc
123
"""#
`,
			expected: `foo: int
lacy: """
	abc
	123
	"""
`},
	}

	var r = cuecontext.New()
	for _, tcase := range testCases {
		file, err := parser.ParseFile("-", tcase.s)
		assert.NilError(t, err)
		inst := r.BuildFile(file)
		str, err := ToString(inst.Value(), OptBytesToString)
		assert.NilError(t, err)
		assert.Equal(t, str, tcase.expected)
	}
}

func TestPreprocessBuiltinFunc(t *testing.T) {

	doScript := func(values []ast.Node) (ast.Expr, error) {
		for _, v := range values {
			lit, ok := v.(*ast.BasicLit)
			if ok {
				src, _ := literal.Unquote(lit.Value)
				expr, err := parser.ParseExpr("-", src)
				if err != nil {
					return nil, errors.Errorf("script value(%s) format err", src)
				}
				return expr, nil
			}
		}
		return nil, errors.New("script parameter")
	}

	testCases := []struct {
		src        string
		expectJson string
	}{
		{
			src: `
a: "a"
b: "b"
c: script(a)
`,
			expectJson: `{"a":"a","b":"b","c":"a"}`,
		},
		{
			src: `
parameter: {
 continue: "true"
}

wait: {
 continue: script(parameter.continue)
}

`,
			expectJson: `{"parameter":{"continue":"true"},"wait":{"continue":true}}`,
		},
		{
			src: `
parameter: {
 continue: "_status"
}

wait: {
 _status: true 
 continue: script(parameter.continue)
}

`,
			expectJson: `{"parameter":{"continue":"_status"},"wait":{"continue":true}}`,
		},
		{
			src: `
parameter: {
 continue: "_status"
}

wait: {
 _status: true 
 if parameter.continue!=_|_{
	continue: script(parameter["continue"])
 }
}

`,
			expectJson: `{"parameter":{"continue":"_status"},"wait":{"continue":true}}`,
		},
		{
			src: `
parameter: {
 continue: "_status"
}

wait: {
 _status: {
   x: "abc"
 }
 script(parameter["continue"])
}
`,
			expectJson: `{"parameter":{"continue":"_status"},"wait":{"x":"abc"}}`,
		},
	}

	var r = cuecontext.New()
	for _, tCase := range testCases {
		f, err := parser.ParseFile("-", tCase.src)
		assert.NilError(t, err)
		err = PreprocessBuiltinFunc(f, "script", doScript)
		assert.NilError(t, err)
		inst := r.BuildFile(f)
		bt, _ := inst.Value().MarshalJSON()
		assert.Equal(t, string(bt), tCase.expectJson)
	}
}

func TestOpenBasicLit(t *testing.T) {
	f, err := OpenBaiscLit(cuecontext.New().CompileString(`
a: 10
a1: int
b: "foo"
b1: string
c: true
c1: bool
arr: [1,2]
top: _
bottom: _|_
`))
	assert.NilError(t, err)
	val := cuecontext.New().BuildFile(f)
	s, err := toString(val)
	assert.NilError(t, err)
	assert.Equal(t, s, `a:      *10 | _
a1:     int
b:      *"foo" | _
b1:     string
c:      *true | _
c1:     bool
arr:    *[1, 2] | [...]
top:    _
bottom: _|_ // explicit error (_|_ literal) in source
`)
}

func TestListOpen(t *testing.T) {
	f, err := parser.ParseFile("-", `
x: ["a","b"]
y: [...string]
z: []
`)
	assert.NilError(t, err)
	ListOpen(f)

	bt, err := format.Node(f)
	assert.NilError(t, err)
	s := string(bt)
	assert.Equal(t, s, `x: ["a", "b", ...]
y: [...string]
z: []
`)

}
