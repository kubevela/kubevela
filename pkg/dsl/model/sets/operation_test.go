package sets

import (
	"testing"

	"github.com/bmizerany/assert"
)

func TestPatch(t *testing.T) {
	base := `
containers: [{name: "x1"},{name: "x2", age: 13, envs: [{namex: "OPS1",value: string},...]},...]
`

	testcases := []struct {
		patchBody    string
		expectErr    bool
		expectResult string
	}{
		{
			patchBody: `
// +patchKey=name
containers: [{name: "x2",
// +patchKey=namex
envs: [{namex: "OPS",value: "OAM"}]}]

`,
			expectResult: "// +patchKey=name\ncontainers: [{\n\tname: \"x1\"\n}, {\n\tname: \"x2\"\n\tage:  13\n\t// +patchKey=namex\n\tenvs: [{\n\t\tnamex: \"OPS1\"\n\t\tvalue: string\n\t}, {\n\t\tnamex: \"OPS\"\n\t\tvalue: \"OAM\"\n\t}, ...]\n}, ...]\n",
		},
		{
			patchBody: `
// +patchKey=name
containers: [{
name: "x2",
envs: [{namex: "OPS",value: "OAM"}]}]

`,
			expectResult: "",
			expectErr:    true,
		},
		{
			patchBody: `
// +patchKey=name
containers: [{
name: "x2",
envs: [{namex: "OPS1",value: "OAM"}]}]

`,
			expectResult: "// +patchKey=name\ncontainers: [{\n\tname: \"x1\"\n}, {\n\tname: \"x2\"\n\tage:  13\n\tenvs: [{\n\t\tnamex: \"OPS1\"\n\t\tvalue: \"OAM\"\n\t}]\n}, ...]\n",
		},
	}

	for _, tcase := range testcases {
		v, err := StrategyUnify(base, tcase.patchBody)
		assert.Equal(t, err != nil, tcase.expectErr, v)
		assert.Equal(t, v, tcase.expectResult)
	}
}
