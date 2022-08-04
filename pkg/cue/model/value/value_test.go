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

package value

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/oam-dev/kubevela/pkg/cue/model/sets"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func TestValueFill(t *testing.T) {
	src := `
object: {
	x: int
    y: string
    z: {
       provider: string
       do: string
    }
}
`
	testVal1, err := NewValue(src, nil, "")
	assert.NilError(t, err)
	err = testVal1.FillObject(12, "object", "x")
	assert.NilError(t, err)
	err = testVal1.FillObject("y_string", "object", "y")
	assert.NilError(t, err)
	z, err := testVal1.MakeValue(`
z: {
	provider: string
	do: "apply"
}
`)
	z.FillObject("kube", "z", "provider")
	assert.NilError(t, err)
	err = testVal1.FillObject(z, "object")
	assert.NilError(t, err)
	val1String, err := testVal1.String()
	assert.NilError(t, err)

	expectedValString := `object: {
	x: 12
	y: "y_string"
	z: {
		provider: "kube"
		do:       "apply"
	}
}
`
	assert.Equal(t, val1String, expectedValString)

	testVal2, err := NewValue(src, nil, "")
	assert.NilError(t, err)
	err = testVal2.FillRaw(expectedValString)
	assert.NilError(t, err)
	val2String, err := testVal1.String()
	assert.NilError(t, err)
	assert.Equal(t, val2String, expectedValString)
}

func TestStepByFields(t *testing.T) {
	testCases := []struct {
		base     string
		expected string
	}{
		{base: `
step1: {}
step2: {prefix: step1.value}
step3: {prefix: step2.value}
step4: {prefix: step3.value}
if step4.value > 100 {
        step5: {prefix: step4.value}
} 
`,
			expected: `step1: {
	value: 100
}
step2: {
	prefix: 100
	value:  101
}
step3: {
	prefix: 101
	value:  102
}
step5: {
	prefix: 103
	value:  104
}
step4: {
	prefix: 102
	value:  103
}
`},

		{base: `
step1: {}
step2: {prefix: step1.value}
if step2.value > 100 {
        step2_3: {prefix: step2.value}
}
step3: {prefix: step2.value}
`,
			expected: `step1: {
	value: 100
}
step2: {
	prefix: 100
	value:  101
}
step2_3: {
	prefix: 101
	value:  102
}
step3: {
	prefix: 101
	value:  103
}
`},
	}

	for _, tCase := range testCases {
		val, err := NewValue(tCase.base, nil, "")
		assert.NilError(t, err)
		number := 99
		err = val.StepByFields(func(_ string, in *Value) (bool, error) {
			number++
			return false, in.FillObject(map[string]interface{}{
				"value": number,
			})
		})
		assert.NilError(t, err)
		str, err := val.String()
		assert.NilError(t, err)
		assert.Equal(t, str, tCase.expected)
	}

	caseSkip := `
step1: "1"
step2: "2"
step3: "3"
`
	val, err := NewValue(caseSkip, nil, "")
	assert.NilError(t, err)
	inc := 0
	err = val.StepByFields(func(_ string, in *Value) (bool, error) {
		inc++
		s, err := in.CueValue().String()
		assert.NilError(t, err)
		if s == "2" {
			return true, nil
		}
		return false, nil
	})
	assert.NilError(t, err)
	assert.Equal(t, inc, 2)

	inc = 0
	err = val.StepByFields(func(_ string, in *Value) (bool, error) {
		inc++
		s, err := in.CueValue().String()
		assert.NilError(t, err)
		if s == "2" {
			return false, errors.New("mock error")
		}
		return false, nil
	})
	assert.Equal(t, err != nil, true)
	assert.Equal(t, inc, 2)

	inc = 0
	err = val.StepByFields(func(_ string, in *Value) (bool, error) {
		inc++
		s, err := in.CueValue().String()
		assert.NilError(t, err)
		if s == "2" {
			v, err := NewValue("v: 33", nil, "")
			assert.NilError(t, err)
			*in = *v
		}
		return false, nil
	})
	assert.Equal(t, err != nil, true)
	assert.Equal(t, inc, 2)

}

func TestStepWithTag(t *testing.T) {
	// comment all the tests with comprehension with if for now,
	// refer to issue: https://github.com/cue-lang/cue/issues/1826
	testCases := []struct {
		base     string
		expected string
	}{
		{base: `
step1: {}
step2: {prefix: step1.value}
step3: {prefix: step2.value}
step4: {prefix: step3.value}
step5: {
	value:  *100|int
}
`,
			expected: `step1: {
	value: 100
} @step(1)
step2: {
	prefix: 100
	value:  101
} @step(2)
step3: {
	prefix: 101
	value:  102
} @step(3)
step4: {
	prefix: 102
	value:  103
} @step(4)
step5: {
	value: 104
} @step(5)
`}, {base: `
step1: {}
step2: {prefix: step1.value}
step2_3: {prefix: step2.value}
step3: {prefix: step2.value}
step4: {prefix: step3.value}
`,
			expected: `step1: {
	value: 100
} @step(1)
step2: {
	prefix: 100
	value:  101
} @step(2)
step2_3: {
	prefix: 101
	value:  102
} @step(3)
step3: {
	prefix: 101
	value:  103
} @step(4)
step4: {
	prefix: 103
	value:  104
} @step(5)
`}, {base: `
step2: {prefix: step1.value} @step(2)
step1: {} @step(1)
step3: {prefix: step2.value} @step(4)
step2_3: {prefix: step2.value} @step(3)
`,
			expected: `step2: {
	prefix: 100
	value:  101
} @step(2)
step1: {
	value: 100
} @step(1)
step3: {
	prefix: 101
	value:  103
} @step(4)
step2_3: {
	prefix: 101
	value:  102
} @step(3)
`},

		{base: `
step2: {prefix: step1.value} 
step1: {} @step(-1)
step2_3: {prefix: step2.value}
step3: {prefix: step2.value}
`,
			expected: `step2: {
	prefix: 100
	value:  101
} @step(1)
step1: {
	value: 100
} @step(-1)
step2_3: {
	prefix: 101
	value:  102
} @step(2)
step3: {
	prefix: 101
	value:  103
} @step(3)
`}}

	for i, tCase := range testCases {
		r := require.New(t)
		val, err := NewValue(tCase.base, nil, "", TagFieldOrder)
		r.NoError(err)
		number := 99
		err = val.StepByFields(func(name string, in *Value) (bool, error) {
			number++
			return false, in.FillObject(map[string]interface{}{
				"value": number,
			})
		})
		r.NoError(err)
		str, err := sets.ToString(val.CueValue())
		r.NoError(err)
		r.Equal(str, tCase.expected, fmt.Sprintf("testPatch for case(no:%d) %s", i, str))
	}
}

func TestUnmarshal(t *testing.T) {
	case1 := `
provider: "kube"
do: "apply"
`
	out := struct {
		Provider string `json:"provider"`
		Do       string `json:"do"`
	}{}

	val, err := NewValue(case1, nil, "")
	assert.NilError(t, err)
	err = val.UnmarshalTo(&out)
	assert.NilError(t, err)
	assert.Equal(t, out.Provider, "kube")
	assert.Equal(t, out.Do, "apply")

	bt, err := val.CueValue().MarshalJSON()
	assert.NilError(t, err)
	expectedJson, err := json.Marshal(out)
	assert.NilError(t, err)
	assert.Equal(t, string(bt), string(expectedJson))

	caseIncomplete := `
provider: string
do: string
`
	val, err = NewValue(caseIncomplete, nil, "")
	assert.NilError(t, err)
	err = val.UnmarshalTo(&out)
	assert.Equal(t, err != nil, true)
}

func TestStepByList(t *testing.T) {
	base := `[{step: 1},{step: 2}]`
	v, err := NewValue(base, nil, "")
	assert.NilError(t, err)
	var i int64
	err = v.StepByList(func(name string, in *Value) (bool, error) {
		i++
		num, err := in.CueValue().LookupPath(FieldPath("step")).Int64()
		assert.NilError(t, err)
		assert.Equal(t, num, i)
		return false, nil
	})
	assert.NilError(t, err)

	i = 0
	err = v.StepByList(func(_ string, _ *Value) (bool, error) {
		i++
		return true, nil
	})
	assert.NilError(t, err)
	assert.Equal(t, i, int64(1))

	i = 0
	err = v.StepByList(func(_ string, _ *Value) (bool, error) {
		i++
		return false, errors.New("mock error")
	})
	assert.Equal(t, err.Error(), "mock error")
	assert.Equal(t, i, int64(1))

	notListV, err := NewValue(`{}`, nil, "")
	assert.NilError(t, err)
	err = notListV.StepByList(func(_ string, _ *Value) (bool, error) {
		return false, nil
	})
	assert.Equal(t, err != nil, true)
}

func TestValue(t *testing.T) {

	// Test NewValue with wrong cue format.
	caseError := `
provider: xxx
`
	val, err := NewValue(caseError, nil, "")
	assert.NilError(t, err)
	assert.Equal(t, val.Error() != nil, true)

	val, err = NewValue(":", nil, "")
	assert.Equal(t, err != nil, true)
	assert.Equal(t, val == nil, true)

	// Test make error by Fill with wrong cue format.
	caseOk := `
provider: "kube"
do: "apply"
`
	val, err = NewValue(caseOk, nil, "")
	assert.NilError(t, err)
	originCue := val.CueValue()

	_, err = val.MakeValue(caseError)
	assert.Equal(t, err != nil, true)
	_, err = val.MakeValue(":")
	assert.Equal(t, err != nil, true)
	err = val.FillRaw(caseError)
	assert.Equal(t, err != nil, true)
	assert.Equal(t, originCue, val.CueValue())
	cv, err := NewValue(caseOk, nil, "")
	assert.NilError(t, err)
	err = val.FillObject(cv)
	assert.Equal(t, err != nil, true)
	assert.Equal(t, originCue, val.CueValue())

	// Test make error by Fill with cue eval error.
	caseClose := `
close({provider: int})
`
	err = val.FillRaw(caseClose)
	assert.Equal(t, err != nil, true)
	assert.Equal(t, originCue, val.CueValue())
	cv, err = val.MakeValue(caseClose)
	assert.NilError(t, err)
	err = val.FillObject(cv)
	assert.NilError(t, err)
	assert.Equal(t, val.Error() != nil, true)

	_, err = val.LookupValue("abc")
	assert.Equal(t, err != nil, true)

	providerValue, err := val.LookupValue("provider")
	assert.NilError(t, err)
	err = providerValue.StepByFields(func(_ string, in *Value) (bool, error) {
		return false, nil
	})
	assert.Equal(t, err != nil, true)

	openSt := `
#X: {...}
x: #X & {
   name: "xxx"
   age: 12
}
`
	val, err = NewValue(openSt, nil, "")
	assert.NilError(t, err)
	x, _ := val.LookupValue("x")
	xs, _ := x.String()
	_, err = val.MakeValue(xs)
	assert.NilError(t, err)
}

func TestValueError(t *testing.T) {
	caseOk := `
provider: "kube"
do: "apply"
`
	val, err := NewValue(caseOk, nil, "")
	assert.NilError(t, err)
	err = val.FillRaw(`
provider: "conflict"`)
	assert.Equal(t, err != nil, true)

	val, err = NewValue(caseOk, nil, "")
	assert.NilError(t, err)
	err = val.FillObject(map[string]string{
		"provider": "abc",
	})
	assert.NilError(t, err)
	assert.Equal(t, val.Error() != nil, true)
}

func TestField(t *testing.T) {
	caseSrc := `
name: "foo"
#name: "fly"
#age: 100
bottom: _|_
`
	val, err := NewValue(caseSrc, nil, "")
	assert.NilError(t, err)

	name, err := val.Field("name")
	assert.NilError(t, err)
	nameValue, err := name.String()
	assert.NilError(t, err)
	assert.Equal(t, nameValue, "foo")

	dname, err := val.Field("#name")
	assert.NilError(t, err)
	nameValue, err = dname.String()
	assert.NilError(t, err)
	assert.Equal(t, nameValue, "fly")

	_, err = val.Field("age")
	assert.Equal(t, err != nil, true)

	_, err = val.Field("bottom")
	assert.Equal(t, err != nil, true)
}

func TestProcessScript(t *testing.T) {
	testCases := []struct {
		src    string
		expect string
		err    string
	}{
		{
			src: `parameter: {
 check: "status==\"ready\""
}

wait: {
 status: "ready"
 continue: script(parameter.check)
}`,
			expect: `parameter: {
	check: "status==\"ready\""
}
wait: {
	status:   "ready"
	continue: true
}
`,
		},
		{
			src: `parameter: {
 check: "status==\"ready\""
}

wait: {
 status: "ready"
 continue: script("")
}`,
			expect: ``,
			err:    "script parameter error",
		},
		{
			src: `parameter: {
 check: "status=\"ready\""
}

wait: {
 status: "ready"
 continue: script(parameter.check)
}`,
			expect: ``,
			err:    "script value(status=\"ready\") is invalid CueLang",
		},
	}

	for _, tCase := range testCases {
		v, err := NewValue(tCase.src, nil, "", ProcessScript)
		if tCase.err != "" {
			assert.Equal(t, err.Error(), tCase.err)
			continue
		}
		assert.NilError(t, err)
		s, err := v.String()
		assert.NilError(t, err)
		assert.Equal(t, s, tCase.expect)
	}
}

func TestLookupByScript(t *testing.T) {
	testCases := []struct {
		src    string
		script string
		expect string
	}{
		{
			src: `
traits: {
	ingress: {
		// +patchKey=name
		test: [{name: "main", image: "busybox"}]
	}
}
`,
			script: `traits["ingress"]`,
			expect: `// +patchKey=name
test: [{
	name:  "main"
	image: "busybox"
}]
`,
		},
		{
			src: `
apply: containers: [{name: "main", image: "busybox"}]
`,
			script: `apply.containers[0].image`,
			expect: `"busybox"
`,
		},
		{
			src: `
apply: workload: name: "main"
`,
			script: `
apply.workload.name`,
			expect: `"main"
`,
		},
		{
			src: `
apply: arr: ["abc","def"]
`,
			script: `
import "strings"
strings.Join(apply.arr,".")+"$"`,
			expect: `"abc.def$"
`,
		},
	}

	for _, tCase := range testCases {
		srcV, err := NewValue(tCase.src, nil, "")
		assert.NilError(t, err)
		v, err := srcV.LookupByScript(tCase.script)
		assert.NilError(t, err)
		result, _ := v.String()
		assert.Equal(t, tCase.expect, result)
	}

	errorCases := []struct {
		src    string
		script string
		err    string
	}{
		{
			src: `
   op: string 
   op: "help"
`,
			script: `op(1`,
			err:    "parse script: expected ')', found 'EOF'",
		},
		{
			src: `
   op: string 
   op: "help"
`,
			script: `oss`,
			err:    "failed to lookup value: var(path=oss) not exist",
		},
	}

	for _, tCase := range errorCases {
		srcV, err := NewValue(tCase.src, nil, "")
		assert.NilError(t, err)
		_, err = srcV.LookupByScript(tCase.script)
		assert.Error(t, err, tCase.err)
		assert.Equal(t, err.Error(), tCase.err)
	}
}

func TestGet(t *testing.T) {
	caseOk := `
strKey: "xxx"
intKey: 100
boolKey: true
`
	val, err := NewValue(caseOk, nil, "")
	assert.NilError(t, err)

	str, err := val.GetString("strKey")
	assert.NilError(t, err)
	assert.Equal(t, str, "xxx")
	// err case
	_, err = val.GetInt64("strKey")
	assert.Equal(t, err != nil, true)

	intv, err := val.GetInt64("intKey")
	assert.NilError(t, err)
	assert.Equal(t, intv, int64(100))
	// err case
	_, err = val.GetBool("intKey")
	assert.Equal(t, err != nil, true)

	ok, err := val.GetBool("boolKey")
	assert.NilError(t, err)
	assert.Equal(t, ok, true)
	// err case
	_, err = val.GetString("boolKey")
	assert.Equal(t, err != nil, true)
}

func TestImports(t *testing.T) {
	cont := `
context: stepSessionID: "3w9qkdgn5w"`
	v, err := NewValue(`
import (
	"vela/custom"
)

id: custom.context.stepSessionID 

`+cont, nil, cont)
	assert.NilError(t, err)
	id, err := v.GetString("id")
	assert.NilError(t, err)
	assert.Equal(t, id, "3w9qkdgn5w")
}

func TestOpenCompleteValue(t *testing.T) {
	v, err := NewValue(`
x: 10
y: "100"
`, nil, "")
	assert.NilError(t, err)
	err = v.OpenCompleteValue()
	assert.NilError(t, err)
	s, err := v.String()
	assert.NilError(t, err)
	assert.Equal(t, s, `x: *10 | _
y: *"100" | _
`)
}

func TestFillByScript(t *testing.T) {
	testCases := []struct {
		name     string
		raw      string
		path     string
		v        string
		expected string
	}{
		{
			name: "insert array",
			raw:  `a: b: [{x: 100},...]`,
			path: "a.b[1]",
			v:    `{name: "foo"}`,
			expected: `a: {
	b: [{
		x: 100
	}, {
		name: "foo"
	}]
}
`,
		},
		{
			name: "insert nest array ",
			raw:  `a: b: [{x: y:[{name: "key"}]}]`,
			path: "a.b[0].x.y[0].value",
			v:    `"foo"`,
			expected: `a: {
	b: [{
		x: {
			y: [{
				name:  "key"
				value: "foo"
			}]
		}
	}]
}
`,
		},
		{
			name: "insert without array",
			raw:  `a: b: [{x: y:[{name: "key"}]}]`,
			path: "a.c.x",
			v:    `"foo"`,
			expected: `a: {
	b: [{
		x: {
			y: [{
				name: "key"
			}]
		}
	}]
	c: {
		x: "foo"
	}
}
`,
		},
		{
			name: "path with string index",
			raw:  `a: b: [{x: y:[{name: "key"}]}]`,
			path: "a.c[\"x\"]",
			v:    `"foo"`,
			expected: `a: {
	b: [{
		x: {
			y: [{
				name: "key"
			}, ...]
		}
	}, ...]
	c: {
		x: "foo"
	}
}
`,
		},
	}

	for _, tCase := range testCases {
		v, err := NewValue(tCase.raw, nil, "")
		assert.NilError(t, err, tCase.name)
		val, err := v.MakeValue(tCase.v)
		assert.NilError(t, err, tCase.name)
		err = v.FillValueByScript(val, tCase.path)
		assert.NilError(t, err, tCase.name)
		s, err := v.String()
		assert.NilError(t, err, tCase.name)
		assert.Equal(t, s, tCase.expected, tCase.name)
	}

	errCases := []struct {
		name string
		raw  string
		path string
		v    string
		err  string
	}{
		{
			name: "invalid path",
			raw:  `a: b: [{x: 100},...]`,
			path: "a.b[1]+1",
			v:    `{name: "foo"}`,
			err:  "invalid path",
		},
		{
			name: "invalid path [float]",
			raw:  `a: b: [{x: 100},...]`,
			path: "a.b[0.1]",
			v:    `{name: "foo"}`,
			err:  "invalid path",
		},
		{
			name: "invalid value",
			raw:  `a: b: [{x: y:[{name: "key"}]}]`,
			path: "a.b[0].x.y[0].value",
			v:    `foo`,
			err:  "remake value: a.b.x.y.value: reference \"foo\" not found",
		},
		{
			name: "conflict merge",
			raw:  `a: b: [{x: y:[{name: "key"}]}]`,
			path: "a.b[0].x.y[0].name",
			v:    `"foo"`,
			err:  "remake value: a.b.0.x.y.0.name: conflicting values \"foo\" and \"key\"",
		},
		{
			name: "filled value with wrong cue format",
			raw:  `a: b: [{x: y:[{name: "key"}]}]`,
			path: "a.b[0].x.y[0].value",
			v:    `*+-`,
			err:  "remake value: expected operand, found '}'",
		},
	}

	for _, errCase := range errCases {
		v, err := NewValue(errCase.raw, nil, "")
		assert.NilError(t, err, errCase.name)
		err = v.fillRawByScript(errCase.v, errCase.path)
		assert.Equal(t, errCase.err, err.Error(), errCase.name)
	}

}
