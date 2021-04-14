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

package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

var mock map[string]interface{}

func mockTask(ctx CallCtx, params interface{}) error {
	ppp := params.(map[string]interface{})
	for k, v := range ppp {
		mock[k] = v
	}
	return nil
}

func mockTask1(ctx CallCtx, params interface{}) error {
	ppp := params.(map[string]interface{})
	for k, v := range ppp {
		mock[k] = v
	}
	return nil
}

func TestDoTasks(t *testing.T) {
	RegisterTask("mock", mockTask)
	RegisterTask("mock1", mockTask1)

	testCases := map[string]struct {
		expect     map[string]interface{}
		input      map[string]interface{}
		expectMock map[string]interface{}
	}{
		"one task exec with new value added": {
			expect: map[string]interface{}{
				"image": "testImage",
			},
			input: map[string]interface{}{
				"image": "testImage",
				"mock": map[string]interface{}{
					"prams": "testValues",
				},
			},
			expectMock: map[string]interface{}{
				"prams": "testValues",
			},
		},

		"No task exec No changes": {
			expect: map[string]interface{}{
				"image": "testImage",
				"cmd":   []string{"sleep", "1000"},
			},
			expectMock: map[string]interface{}{},
			input: map[string]interface{}{
				"image": "testImage",
				"cmd":   []string{"sleep", "1000"},
			}},
		"two tasks exec with new value added": {
			expect: map[string]interface{}{
				"image": "testImage",
			},
			expectMock: map[string]interface{}{
				"prams":  "testValues",
				"prams1": "testValues1",
			},
			input: map[string]interface{}{
				"image": "testImage",
				"mock": map[string]interface{}{
					"prams": "testValues",
				},
				"mock1": map[string]interface{}{
					"prams1": "testValues1",
				},
			}},
	}

	for _, tcase := range testCases {
		mock = make(map[string]interface{})
		ret, _ := Run(tcase.input, cmdutil.IOStreams{})
		assert.Equal(t, tcase.expect, ret)
		assert.Equal(t, tcase.expectMock, mock)
	}
}

func TestRegisterTask(t *testing.T) {
	RegisterTask("mock", mockTask)
	RegisterTask("mock1", mockTask)
	RegisterTask("mock2", mockTask)
	RegisterTask("mock", mockTask)

	expectTasks := []string{
		"mock",
		"mock1",
		"mock2",
	}
	tasks := GetTasks()
	for _, eTask := range expectTasks {
		if _, ok := tasks[eTask]; !ok {
			t.Errorf("task %s not register", eTask)
		}
	}
}
