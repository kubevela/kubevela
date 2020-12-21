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
		patchBody string
		expectErr bool
	}{
		{
			patchBody: `
// +patchKey=name
containers: [{name: "x2",
// +patchKey=namex
envs: [{namex: "OPS",value: "OAM"}]}]

`,
		},
		{
			patchBody: `
// +patchKey=name
containers: [{
name: "x2",
envs: [{namex: "OPS",value: "OAM"}]}]

`,
			expectErr: true,
		},
		{
			patchBody: `
// +patchKey=name
containers: [{
name: "x2",
envs: [{namex: "OPS1",value: "OAM"}]}]

`,
		},
	}

	for _, tcase := range testcases {
		v, err := StrategyUnify(base, tcase.patchBody)
		assert.Equal(t, err != nil, tcase.expectErr, v)
	}
}
