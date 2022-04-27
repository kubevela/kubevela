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

package tasks

import (
	"context"
	"testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	"github.com/pkg/errors"
	"gotest.tools/assert"

	"github.com/oam-dev/kubevela/pkg/cue/process"
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
	}
	pCtx := process.NewContext(process.ContextData{
		AppName:         "myapp",
		CompName:        "mycomp",
		Namespace:       "default",
		AppRevisionName: "myapp-v1",
	})
	discover := &taskDiscover{
		builtins: map[string]types.TaskGenerator{
			"suspend":   suspend,
			"stepGroup": stepGroup,
		},
		remoteTaskDiscover: custom.NewTaskLoader(loadTemplate, nil, nil, 0, pCtx),
	}

	_, err := discover.GetTaskGenerator(context.Background(), "suspend")
	assert.NilError(t, err)
	_, err = discover.GetTaskGenerator(context.Background(), "stepGroup")
	assert.NilError(t, err)
	_, err = discover.GetTaskGenerator(context.Background(), "foo")
	assert.NilError(t, err)
	_, err = discover.GetTaskGenerator(context.Background(), "crazy")
	assert.NilError(t, err)
	_, err = discover.GetTaskGenerator(context.Background(), "fly")
	assert.Equal(t, err.Error(), makeErr("fly").Error())

}

func TestSuspendStep(t *testing.T) {
	discover := &taskDiscover{
		builtins: map[string]types.TaskGenerator{
			"suspend": suspend,
		},
	}
	gen, err := discover.GetTaskGenerator(context.Background(), "suspend")
	assert.NilError(t, err)
	runner, err := gen(v1beta1.WorkflowStep{Name: "test"}, &types.GeneratorOptions{ID: "124"})
	assert.NilError(t, err)
	assert.Equal(t, runner.Name(), "test")
	assert.Equal(t, runner.Pending(nil), false)
	status, act, err := runner.Run(nil, nil)
	assert.NilError(t, err)
	assert.Equal(t, act.Suspend, true)
	assert.Equal(t, status.ID, "124")
	assert.Equal(t, status.Name, "test")
	assert.Equal(t, status.Phase, common.WorkflowStepPhaseSucceeded)
}

func TestStepGroupStep(t *testing.T) {
	discover := &taskDiscover{
		builtins: map[string]types.TaskGenerator{
			"stepGroup": stepGroup,
		},
	}
	gen, err := discover.GetTaskGenerator(context.Background(), "stepGroup")
	assert.NilError(t, err)
	runner, err := gen(v1beta1.WorkflowStep{Name: "test"}, &types.GeneratorOptions{ID: "124"})
	assert.NilError(t, err)
	assert.Equal(t, runner.Name(), "test")
	assert.Equal(t, runner.Pending(nil), false)
	status, act, err := runner.Run(nil, nil)
	assert.NilError(t, err)
	assert.Equal(t, act.Suspend, false)
	assert.Equal(t, status.ID, "124")
	assert.Equal(t, status.Name, "test")
	assert.Equal(t, status.Phase, common.WorkflowStepPhaseSucceeded)
}
