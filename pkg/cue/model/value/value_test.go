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
	"testing"

	"github.com/pkg/errors"
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
	testVal1, err := NewValue(src, nil)
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

	testVal2, err := NewValue(src, nil)
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
        step5: "end"
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
step4: {
	prefix: 102
	value:  103
}
step5: "end"
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
		val, err := NewValue(tCase.base, nil)
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
	val, err := NewValue(caseSkip, nil)
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
			v, err := NewValue("v: 33", nil)
			assert.NilError(t, err)
			*in = *v
		}
		return false, nil
	})
	assert.Equal(t, err != nil, true)
	assert.Equal(t, inc, 2)

}

func TestStepWithTag(t *testing.T) {
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
        step5: "end"
} 
`,
			expected: `step1: {
	value: 100
} @step(1)
step2: {
	prefix: 100 @step(3)
	value:  101
} @step(2)
step3: {
	prefix: 101 @step(5)
	value:  102
} @step(4)
step4: {
	prefix: 102 @step(7)
	value:  103
} @step(6)
step5: "end" @step(8)
`}, {base: `
step1: {}
step2: {prefix: step1.value}
if step2.value > 100 {
        step2_3: {prefix: step2.value}
}
step3: {prefix: step2.value}
step4: {prefix: step3.value}
if step4.value > 100 {
        step5: "end"
} 
`,
			expected: `step1: {
	value: 100
} @step(1)
step2: {
	prefix: 100 @step(3)
	value:  101
} @step(2)
step2_3: {
	prefix: 101 @step(5)
	value:  102
} @step(4)
step3: {
	prefix: 101 @step(7)
	value:  103
} @step(6)
step4: {
	prefix: 103 @step(9)
	value:  104
} @step(8)
step5: "end" @step(10)
`}, {base: `
step2: {prefix: step1.value} @step(2)
step1: {} @step(1)
step3: {prefix: step2.value} @step(4)
if step2.value > 100 {
        step2_3: {prefix: step2.value} @step(3)
}
`,
			expected: `step2: {
	prefix: 100 @step(1)
	value:  101
} @step(2)
step1: {
	value: 100
} @step(1)
step3: {
	prefix: 101 @step(2)
	value:  103
} @step(4)
step2_3: {
	prefix: 101 @step(3)
	value:  102
} @step(3)
`},

		{base: `
step2: {prefix: step1.value} 
step1: {} @step(-1)
if step2.value > 100 {
        step2_3: {prefix: step2.value}
}
step3: {prefix: step2.value}
`,
			expected: `step2: {
	prefix: 100 @step(2)
	value:  101
} @step(1)
step1: {
	value: 100
} @step(-1)
step2_3: {
	prefix: 101 @step(4)
	value:  102
} @step(3)
step3: {
	prefix: 101 @step(6)
	value:  103
} @step(5)
`}}

	for _, tCase := range testCases {
		val, err := NewValue(tCase.base, nil, TagFieldOrder)
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

	val, err := NewValue(case1, nil)
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
	val, err = NewValue(caseIncomplete, nil)
	assert.NilError(t, err)
	err = val.UnmarshalTo(&out)
	assert.Equal(t, err != nil, true)
}

func TestStepByList(t *testing.T) {
	base := `[{step: 1},{step: 2}]`
	v, err := NewValue(base, nil)
	assert.NilError(t, err)
	var i int64
	err = v.StepByList(func(name string, in *Value) (bool, error) {
		i++
		num, err := in.CueValue().Lookup("step").Int64()
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

	notListV, err := NewValue(`{}`, nil)
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
	val, err := NewValue(caseError, nil)
	assert.Equal(t, err != nil, true)
	assert.Equal(t, val == nil, true)

	val, err = NewValue(":", nil)
	assert.Equal(t, err != nil, true)
	assert.Equal(t, val == nil, true)

	// Test make error by Fill with wrong cue format.
	caseOk := `
provider: "kube"
do: "apply"
`
	val, err = NewValue(caseOk, nil)
	assert.NilError(t, err)
	originCue := val.CueValue()

	_, err = val.MakeValue(caseError)
	assert.Equal(t, err != nil, true)
	_, err = val.MakeValue(":")
	assert.Equal(t, err != nil, true)
	err = val.FillRaw(caseError)
	assert.Equal(t, err != nil, true)
	assert.Equal(t, originCue, val.CueValue())
	cv, err := NewValue(caseOk, nil)
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
	assert.Equal(t, err != nil, true)
	assert.Equal(t, originCue, val.CueValue())

	_, err = val.LookupValue("abc")
	assert.Equal(t, err != nil, true)

	providerValue, err := val.LookupValue("provider")
	assert.NilError(t, err)
	err = providerValue.StepByFields(func(_ string, in *Value) (bool, error) {
		return false, nil
	})
	assert.Equal(t, err != nil, true)
}

func TestValueError(t *testing.T) {
	caseOk := `
provider: "kube"
do: "apply"
`
	val, err := NewValue(caseOk, nil)
	assert.NilError(t, err)
	err = val.FillRaw(`
provider: "conflict"`)
	assert.NilError(t, err)
	assert.Equal(t, val.Error() != nil, true)

	val, err = NewValue(caseOk, nil)
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
	val, err := NewValue(caseSrc, nil)
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
