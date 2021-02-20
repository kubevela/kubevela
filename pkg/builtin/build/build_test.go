package build

import (
	"testing"

	"github.com/bmizerany/assert"

	"github.com/oam-dev/kubevela/pkg/builtin/registry"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestBuild(t *testing.T) {
	errTestCase := []struct {
		expectErr string
		input     map[string]interface{}
	}{
		{
			expectErr: "do task build: json: cannot unmarshal number into Go value of type build.Build",
			input: map[string]interface{}{
				"build": 100,
			},
		},
		{
			expectErr: "do task build: json: cannot unmarshal string into Go value of type build.Build",
			input: map[string]interface{}{
				"build": "test",
			},
		},
		{
			expectErr: "do task build: json: cannot unmarshal array into Go value of type build.Build",
			input: map[string]interface{}{
				"build": []string{"test"},
			},
		},
		{
			expectErr: "do task build: lookup field 'image' : not found",
			input: map[string]interface{}{
				"build": map[string]interface{}{
					"docker": map[string]interface{}{
						"file":    "./vela.yaml",
						"context": "./docker",
					},
					"push": map[string]interface{}{
						"Registry": "test.io",
					},
				},
			},
		},
		{
			expectErr: "do task build: image must be 'string'",
			input: map[string]interface{}{
				"image": 1000,
				"build": map[string]interface{}{
					"docker": map[string]interface{}{
						"file":    "./vela.yaml",
						"context": "./docker",
					},
					"push": map[string]interface{}{
						"Registry": "test.io",
					},
				},
			},
		},
	}

	for _, tcase := range errTestCase {
		_, err := registry.Run(tcase.input, cmdutil.IOStreams{})
		assert.Equal(t, err != nil, true)
		assert.Equal(t, tcase.expectErr, err.Error())
	}

}
