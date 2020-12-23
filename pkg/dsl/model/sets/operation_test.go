package sets

import (
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"github.com/bmizerany/assert"
)

func TestPatch(t *testing.T) {

	testCase := []struct {
		base   string
		patch  string
		result string
	}{
		{
			base:  `containers: [{name: "x1"},{name: "x2"},...]`,
			patch: `containers: [{name: "x1"},{name: "x2",image: "pause:0.1"}]`,
			result: `containers: [{
	name: "x1"
}, {
	name:  "x2"
	image: "pause:0.1"
}]
`,
		},

		{
			base:   `containers: [{name: "x1"},{name: "x2"},...]`,
			patch:  `containers: [{name: "x2"},{name: "x1"}]`,
			result: "_|_\n",
		},

		{
			base:   `containers: [{name: _|_},{name: "x2"},...]`,
			patch:  `containers: [{name: _|_},{name: "x2"}]`,
			result: "_|_\n",
		},

		{
			base: `containers: [{name: "x1"},{name: "x2"},...]`,
			patch: `
// +patchKey=name
containers: [{name: "x2"},{name: "x1"}]`,
			result: `// +patchKey=name
containers: [{
	name: "x1"
}, {
	name: "x2"
}, ...]
`,
		},

		{
			base: `containers: [{name: "x1"},{name: "x2"},...]`,
			patch: `
// +patchKey=name
containers: [{name: "x4"},{name: "x3"},{name: "x1"}]`,
			result: `// +patchKey=name
containers: [{
	name: "x1"
}, {
	name: "x2"
}, {
	name: "x4"
}, {
	name: "x3"
}, ...]
`,
		},

		{
			base: `containers: [{name: "x1"},{name: "x2"},...]`,
			patch: `
// +patchKey=name
containers: [{noname: "x3"},{name: "x1"}]`,
			result: "_|_\n",
		},
		{
			base: `containers: [{name: "x1"},{name: "x2", envs:[ {name: "OPS",value: string},...]},...]`,
			patch: `
// +patchKey=name
containers: [{name: "x2", envs: [{name: "OPS", value: "OAM"}]}]`,
			result: `// +patchKey=name
containers: [{
	name: "x1"
}, {
	name: "x2"
	envs: [{
		name:  "OPS"
		value: "OAM"
	}, ...]
}, ...]
`,
		},

		{
			base: `containers: [{name: "x1"},{name: "x2", envs:[ {name: "OPS",value: string},...]},...]`,
			patch: `
// +patchKey=name
containers: [{name: "x2", envs: [{name: "USER", value: "DEV"},{name: "OPS", value: "OAM"}]}]`,
			result: `// +patchKey=name
containers: [{
	name: "x1"
}, {
	name: "x2"
	envs: [{
		name:  "OPS"
		value: "OAM"
	}, {
		name:  "USER"
		value: "DEV"
	}, ...]
}, ...]
`,
		},

		{
			base: `containers: [{name: "x1"},{name: "x2", envs:[ {key: "OPS",value: string},...]},...]`,
			patch: `
// +patchKey=name
containers: [{name: "x2", 
// +patchKey=key
envs: [{key: "USER", value: "DEV"},{key: "OPS", value: "OAM"}]}]`,
			result: `// +patchKey=name
containers: [{
	name: "x1"
}, {
	name: "x2"
	// +patchKey=key
	envs: [{
		key:   "OPS"
		value: "OAM"
	}, {
		key:   "USER"
		value: "DEV"
	}, ...]
}, ...]
`,
		},
	}

	for i, tcase := range testCase {
		v, _ := StrategyUnify(tcase.base, tcase.patch)
		assert.Equal(t, v, tcase.result, fmt.Sprintf("testPatch for case(no:%d)", i))
	}
}

func TestParseCommentTags(t *testing.T) {
	temp := `
// +patchKey=name
// +testKey1=testValue1
	// +testKey2=testValue2
// +testKey3 =testValue3
//    +testKey4 = testValue4
// invalid=x
// +invalid=x y
// +invalid
x: null
`

	var r cue.Runtime
	inst, err := r.Compile("-", temp)
	if err != nil {
		t.Error(err)
		return
	}
	ms := findCommentTag(inst.Lookup("x").Doc())
	assert.Equal(t, ms, map[string]string{
		"patchKey": "name",
		"testKey1": "testValue1",
		"testKey2": "testValue2",
		"testKey3": "testValue3",
		"testKey4": "testValue4",
	})
}
