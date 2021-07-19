package value

import (
	"encoding/json"
	"testing"

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
		err = val.StepByFields(func(in *Value) (bool, error) {
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
}
