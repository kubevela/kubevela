package tasks

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"gotest.tools/assert"

	"github.com/oam-dev/kubevela/pkg/workflow/tasks/custom"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

func TestDiscover(t *testing.T) {

	makeErr := func(name string) error {
		return errors.Errorf("template %s not found", name)
	}

	loadTemplate := func(ctx context.Context, name string) (string, error) {
		switch name {
		case "foo":
			return "", nil
		case "crazy":
			return "", nil
		default:
			return "", makeErr(name)
		}
		return "", nil
	}
	discover := &taskDiscover{
		builtins: map[string]types.TaskGenerator{
			"suspend": suspend,
		},
		remoteTaskDiscover: custom.NewTaskLoader(loadTemplate, nil, nil),
	}

	_, err := discover.GetTaskGenerator("suspend")
	assert.NilError(t, err)
	_, err = discover.GetTaskGenerator("foo")
	assert.NilError(t, err)
	_, err = discover.GetTaskGenerator("crazy")
	assert.NilError(t, err)
	_, err = discover.GetTaskGenerator("fly")
	assert.Equal(t, err.Error(), makeErr("fly").Error())

}
