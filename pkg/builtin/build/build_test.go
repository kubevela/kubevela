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

package build

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
