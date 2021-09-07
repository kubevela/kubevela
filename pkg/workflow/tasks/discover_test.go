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

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pkg/errors"
	"gotest.tools/assert"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
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
	discover := &taskDiscover{
		builtins: map[string]types.TaskGenerator{
			"suspend": suspend,
		},
		remoteTaskDiscover: custom.NewTaskLoader(loadTemplate, nil, nil),
	}

	_, err := discover.GetTaskGenerator(context.Background(), "suspend")
	assert.NilError(t, err)
	_, err = discover.GetTaskGenerator(context.Background(), "foo")
	assert.NilError(t, err)
	_, err = discover.GetTaskGenerator(context.Background(), "crazy")
	assert.NilError(t, err)
	_, err = discover.GetTaskGenerator(context.Background(), "fly")
	assert.Equal(t, err.Error(), makeErr("fly").Error())

}

func TestRegister(t *testing.T) {
	discover := &taskDiscover{
		builtins: map[string]types.TaskGenerator{
			"suspend": suspend,
		}}
	discover.RegisterGenerator("test", func(_ wfContext.Context, options *types.TaskRunOptions, paramValue *value.Value) (common.WorkflowStepStatus, *types.Operation, *value.Value) {
		return common.WorkflowStepStatus{
			Phase: common.WorkflowStepPhaseSucceeded,
		}, nil, nil
	})

	gen, err := discover.GetTaskGenerator(context.Background(), "test")
	assert.NilError(t, err)
	run, err := gen(v1beta1.WorkflowStep{
		Name: "step1",
	}, &types.GeneratorOptions{ID: "abcd"})
	assert.NilError(t, err)
	assert.Equal(t, run.Name(), "step1")
	wfCtx := mockContext(t)
	status, _, err := run.Run(wfCtx, &types.TaskRunOptions{})
	assert.NilError(t, err)
	assert.Equal(t, status.Name, "step1")
	assert.Equal(t, status.Type, "test")
	assert.Equal(t, status.ID, "abcd")
	assert.Equal(t, status.Phase, common.WorkflowStepPhaseSucceeded)
}

func mockContext(t *testing.T) wfContext.Context {
	cli := &test.MockClient{
		MockCreate: func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
			return nil
		},
		MockUpdate: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			return nil
		},
		MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			return nil
		},
	}
	wfCtx, err := wfContext.NewEmptyContext(cli, "default", "v1")
	assert.NilError(t, err)
	return wfCtx
}
