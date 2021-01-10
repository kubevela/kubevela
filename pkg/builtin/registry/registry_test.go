package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
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
