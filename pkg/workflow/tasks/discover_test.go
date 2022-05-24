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

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks/custom"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

func TestDiscover(t *testing.T) {
	r := require.New(t)
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
			"stepGroup": StepGroup,
		},
		remoteTaskDiscover: custom.NewTaskLoader(loadTemplate, nil, nil, 0, pCtx),
	}

	_, err := discover.GetTaskGenerator(context.Background(), "suspend")
	r.NoError(err)
	_, err = discover.GetTaskGenerator(context.Background(), "stepGroup")
	r.NoError(err)
	_, err = discover.GetTaskGenerator(context.Background(), "foo")
	r.NoError(err)
	_, err = discover.GetTaskGenerator(context.Background(), "crazy")
	r.NoError(err)
	_, err = discover.GetTaskGenerator(context.Background(), "fly")
	r.Equal(err.Error(), makeErr("fly").Error())

}

func TestSuspendStep(t *testing.T) {
	r := require.New(t)
	discover := &taskDiscover{
		builtins: map[string]types.TaskGenerator{
			"suspend": suspend,
		},
	}
	gen, err := discover.GetTaskGenerator(context.Background(), "suspend")
	r.NoError(err)
	runner, err := gen(v1beta1.WorkflowStep{
		Name:      "test",
		DependsOn: []string{"depend"},
	}, &types.GeneratorOptions{ID: "124"})
	r.NoError(err)
	r.Equal(runner.Name(), "test")

	// test pending
	r.Equal(runner.Pending(nil, nil), true)
	ss := map[string]common.StepStatus{
		"depend": {
			Phase: common.WorkflowStepPhaseSucceeded,
		},
	}
	r.Equal(runner.Pending(nil, ss), false)

	// test skip
	status, skip := runner.Skip(common.WorkflowStepPhaseFailedAfterRetries, nil)
	r.Equal(skip, true)
	r.Equal(status.Phase, common.WorkflowStepPhaseSkipped)
	r.Equal(status.Reason, custom.StatusReasonSkip)
	runner2, err := gen(v1beta1.WorkflowStep{
		If:   "always",
		Name: "test",
	}, &types.GeneratorOptions{ID: "124"})
	_, skip = runner2.Skip(common.WorkflowStepPhaseFailedAfterRetries, nil)
	r.Equal(skip, false)

	// test run
	status, act, err := runner.Run(nil, nil)
	r.NoError(err)
	r.Equal(act.Suspend, true)
	r.Equal(status.ID, "124")
	r.Equal(status.Name, "test")
	r.Equal(status.Phase, common.WorkflowStepPhaseSucceeded)
}

type testEngine struct {
	stepStatus common.WorkflowStepStatus
	operation  *types.Operation
}

func (e *testEngine) Run(taskRunners []types.TaskRunner, dag bool) error {
	return nil
}

func (e *testEngine) GetStepStatus(stepName string) common.WorkflowStepStatus {
	return e.stepStatus
}

func (e *testEngine) SetParentRunner(name string) {
}

func (e *testEngine) GetOperation() *types.Operation {
	return e.operation
}

func TestStepGroupStep(t *testing.T) {
	r := require.New(t)
	discover := &taskDiscover{
		builtins: map[string]types.TaskGenerator{
			"stepGroup": StepGroup,
		},
	}
	genSub, err := discover.GetTaskGenerator(context.Background(), "stepGroup")
	r.NoError(err)
	subRunner, err := genSub(v1beta1.WorkflowStep{Name: "sub"}, &types.GeneratorOptions{ID: "1"})
	r.NoError(err)
	gen, err := discover.GetTaskGenerator(context.Background(), "stepGroup")
	r.NoError(err)
	runner, err := gen(v1beta1.WorkflowStep{
		Name:      "test",
		DependsOn: []string{"depend"},
	}, &types.GeneratorOptions{ID: "124", SubTaskRunners: []types.TaskRunner{subRunner}})
	r.NoError(err)
	r.Equal(runner.Name(), "test")

	// test pending
	r.Equal(runner.Pending(nil, nil), true)
	ss := map[string]common.StepStatus{
		"depend": {
			Phase: common.WorkflowStepPhaseSucceeded,
		},
	}
	r.Equal(runner.Pending(nil, ss), false)

	// test skip
	stepStatus := make(map[string]common.StepStatus)
	status, skip := runner.Skip(common.WorkflowStepPhaseFailedAfterRetries, stepStatus)
	r.Equal(skip, false)
	r.Equal(stepStatus["test"].Phase, common.WorkflowStepPhaseSkipped)
	r.Equal(status.Phase, common.WorkflowStepPhaseSkipped)
	r.Equal(status.Reason, custom.StatusReasonSkip)
	runner2, err := gen(v1beta1.WorkflowStep{
		If:   "always",
		Name: "test",
	}, &types.GeneratorOptions{ID: "124"})
	_, skip = runner2.Skip(common.WorkflowStepPhaseFailedAfterRetries, stepStatus)
	r.Equal(skip, false)

	// test run
	testCases := []struct {
		name          string
		engine        *testEngine
		expectedPhase common.WorkflowStepPhase
	}{
		{
			name: "running1",
			engine: &testEngine{
				stepStatus: common.WorkflowStepStatus{},
				operation:  &types.Operation{},
			},
			expectedPhase: common.WorkflowStepPhaseRunning,
		},
		{
			name: "running2",
			engine: &testEngine{
				stepStatus: common.WorkflowStepStatus{
					SubStepsStatus: []common.WorkflowSubStepStatus{
						{
							StepStatus: common.StepStatus{
								Phase: common.WorkflowStepPhaseRunning,
							},
						},
					},
				},
				operation: &types.Operation{},
			},
			expectedPhase: common.WorkflowStepPhaseRunning,
		},
		{
			name: "stop",
			engine: &testEngine{
				stepStatus: common.WorkflowStepStatus{
					SubStepsStatus: []common.WorkflowSubStepStatus{
						{
							StepStatus: common.StepStatus{
								Phase: common.WorkflowStepPhaseStopped,
							},
						},
						{
							StepStatus: common.StepStatus{
								Phase: common.WorkflowStepPhaseFailed,
							},
						},
					},
				},
				operation: &types.Operation{},
			},
			expectedPhase: common.WorkflowStepPhaseStopped,
		},
		{
			name: "fail",
			engine: &testEngine{
				stepStatus: common.WorkflowStepStatus{
					SubStepsStatus: []common.WorkflowSubStepStatus{
						{
							StepStatus: common.StepStatus{
								Phase: common.WorkflowStepPhaseFailed,
							},
						},
						{
							StepStatus: common.StepStatus{
								Phase: common.WorkflowStepPhaseSucceeded,
							},
						},
					},
				},
				operation: &types.Operation{},
			},
			expectedPhase: common.WorkflowStepPhaseFailed,
		},
		{
			name: "success",
			engine: &testEngine{
				stepStatus: common.WorkflowStepStatus{
					SubStepsStatus: []common.WorkflowSubStepStatus{
						{
							StepStatus: common.StepStatus{
								Phase: common.WorkflowStepPhaseSucceeded,
							},
						},
					},
				},
				operation: &types.Operation{},
			},
			expectedPhase: common.WorkflowStepPhaseSucceeded,
		},
		{
			name: "operation",
			engine: &testEngine{
				stepStatus: common.WorkflowStepStatus{
					SubStepsStatus: []common.WorkflowSubStepStatus{
						{
							StepStatus: common.StepStatus{
								Phase: common.WorkflowStepPhaseSucceeded,
							},
						},
					},
				},
				operation: &types.Operation{
					Suspend:            true,
					Terminated:         true,
					FailedAfterRetries: true,
					Waiting:            true,
				},
			},
			expectedPhase: common.WorkflowStepPhaseSucceeded,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status, act, err := runner.Run(nil, &types.TaskRunOptions{
				Engine: tc.engine,
			})
			r.NoError(err)
			r.Equal(status.ID, "124")
			r.Equal(status.Name, "test")
			r.Equal(act.Suspend, tc.engine.operation.Suspend)
			r.Equal(status.Phase, tc.expectedPhase)
		})
	}
}
