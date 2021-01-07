package registry

import (
	"testing"

	"github.com/bmizerany/assert"

	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

func mockTask(_ CallCtx, _ interface{}) error {
	return nil
}

func TestDoTasks(t *testing.T) {
	RegisterTask("mock", mockTask)
	RegisterTask("mock1", mockTask)

	testCases := []struct {
		expect map[string]interface{}
		input  map[string]interface{}
	}{
		{
			expect: map[string]interface{}{
				"image": "testImage",
			},
			input: map[string]interface{}{
				"image": "testImage",
				"mock": map[string]string{
					"prams": "testValues",
				},
			}},

		{
			expect: map[string]interface{}{
				"image": "testImage",
				"cmd":   []string{"sleep", "1000"},
			},
			input: map[string]interface{}{
				"image": "testImage",
				"cmd":   []string{"sleep", "1000"},
			}},

		{
			expect: map[string]interface{}{
				"image": "testImage",
			},
			input: map[string]interface{}{
				"image": "testImage",
				"mock": map[string]string{
					"prams": "testValues",
				},
				"mock1": map[string]string{
					"prams1": "testValues1",
				},
			}},
	}

	for _, tcase := range testCases {
		ret, _ := Run(tcase.input, cmdutil.IOStreams{})
		assert.Equal(t, tcase.expect, ret)
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
