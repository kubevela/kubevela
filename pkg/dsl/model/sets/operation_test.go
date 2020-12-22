package sets

import (
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"github.com/bmizerany/assert"
)

func TestPatch(t *testing.T) {


	testCase:= []struct {
		base string
		patch string
		result string
	}{
		{
			base: `containers: [{name: "x1"},{name: "x2"},...]`,
			patch: `containers: [{name: "x2"},{name: "x1"}]`,
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
containers: [{name: "x3"},{name: "x1"}]`,
			result: `// +patchKey=name
containers: [{
	name: "x1"
}, {
	name: "x2"
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
		assert.Equal(t,v,tcase.result,fmt.Sprintf("testPatch for case(no:%d)",i))
	}
}

func TestParseCommentTags(t *testing.T) {
	temp := `
// +patchKey=name
// +test1=value1
// invalid=x
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
		"test1":    "value1",
	})
}
