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

package workflow

import (
	"context"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

type workflow struct {
	app *oamcore.Application
	cli client.Client
}

// NewWorkflow returns a Workflow implementation.
func NewWorkflow(app *oamcore.Application, cli client.Client) Workflow {
	return &workflow{
		app: app,
		cli: cli,
	}
}

// ExecuteSteps process workflow step in order.
func (w *workflow) ExecuteSteps(ctx context.Context, rev string, taskRunners []wfTypes.TaskRunner) (done bool, pause bool, gerr error) {
	if w.app.Spec.Workflow == nil {
		return true, false, nil
	}

	steps := w.app.Spec.Workflow.Steps
	if len(steps) == 0 {
		return true, false, nil
	}

	if w.app.Status.Workflow == nil || w.app.Status.Workflow.AppRevision != rev {
		w.app.Status.Workflow = &common.WorkflowStatus{
			AppRevision: rev,
			Steps:       []common.WorkflowStepStatus{},
		}
	}

	wfStatus := w.app.Status.Workflow

	if wfStatus.Terminated {
		done = true
		return
	}

	w.app.Status.Phase = common.ApplicationRunningWorkflow

	if len(taskRunners) <= wfStatus.StepIndex {
		done = true
		return
	}

	if wfStatus.Suspend {
		pause = true
		return
	}

	var (
		wfCtx wfContext.Context
	)

	if wfStatus.ContextBackend != nil {
		wfCtx, gerr = wfContext.LoadContext(w.cli, w.app.Namespace, rev)
		if gerr != nil {
			gerr = errors.WithMessage(gerr, "load context")
			return
		}
	} else {
		wfCtx, gerr = wfContext.NewContext(w.cli, w.app.Namespace, rev)
		if gerr != nil {
			gerr = errors.WithMessage(gerr, "new context")
			return
		}
		wfStatus.ContextBackend = wfCtx.StoreRef()
	}

	for _, run := range taskRunners[wfStatus.StepIndex:] {
		status, operation, err := run(wfCtx)
		if err != nil {
			gerr = err
			return
		}

		var conditionUpdated bool
		for i := range wfStatus.Steps {
			if wfStatus.Steps[i].Name == status.Name {
				wfStatus.Steps[i] = status
				conditionUpdated = true
				break
			}
		}

		if !conditionUpdated {
			wfStatus.Steps = append(wfStatus.Steps, status)
		}

		if operation != nil {
			wfStatus.Terminated = operation.Terminated
			wfStatus.Suspend = operation.Suspend
		}

		if status.Phase != common.WorkflowStepPhaseSucceeded {
			return
		}

		if err := wfCtx.Commit(); err != nil {
			gerr = errors.WithMessage(err, "commit workflow context")
			return
		}
		wfStatus.StepIndex++

		if wfStatus.Terminated {
			done = true
			return
		}
		if wfStatus.Suspend {
			pause = true
			return
		}
	}
	return true, false, nil // all steps done
}
