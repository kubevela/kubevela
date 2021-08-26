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
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

type workflow struct {
	app     *oamcore.Application
	cli     client.Client
	dagMode bool
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
	if len(taskRunners) == 0 {
		return true, false, nil
	}

	if w.app.Status.Workflow == nil || w.app.Status.Workflow.AppRevision != rev {
		w.app.Status.Workflow = &common.WorkflowStatus{
			AppRevision: rev,
			Steps:       []common.WorkflowStepStatus{},
		}
	}

	wfStatus := w.app.Status.Workflow

	allTasksDone := w.allDone(taskRunners)

	if wfStatus.Terminated {
		done = true
		if !allTasksDone {
			w.app.Status.Phase = common.ApplicationWorkflowTerminated
		}
		return
	}

	w.app.Status.Phase = common.ApplicationRunningWorkflow

	if allTasksDone {
		done = true
		return
	}

	if wfStatus.Suspend {
		pause = true
		w.app.Status.Phase = common.ApplicationWorkflowSuspending
		return
	}

	var (
		wfCtx wfContext.Context
	)

	wfCtx, gerr = w.makeContext(rev)
	if gerr != nil {
		return
	}

	var terminated bool
	if w.dagMode {
		terminated, pause, gerr = w.runAsDag(wfCtx, taskRunners)
	} else {
		terminated, pause, gerr = w.run(wfCtx, taskRunners[wfStatus.StepIndex:])
	}

	if !terminated {
		done = w.allDone(taskRunners)
	}
	return
}

func (w *workflow) allDone(taskRunners []wfTypes.TaskRunner) bool {
	status := w.app.Status.Workflow
	for _, t := range taskRunners {
		done := false
		for _, ss := range status.Steps {
			if ss.Name == t.Name() {
				done = ss.Phase == common.WorkflowStepPhaseSucceeded
				break
			}
		}
		if !done {
			return false
		}
	}
	return true
}

func (w *workflow) makeContext(rev string) (wfCtx wfContext.Context, err error) {
	wfStatus := w.app.Status.Workflow
	if wfStatus.ContextBackend != nil {
		wfCtx, err = wfContext.LoadContext(w.cli, w.app.Namespace, rev)
		if err != nil {
			err = errors.WithMessage(err, "load context")
		}
		return
	}
	wfCtx, err = wfContext.NewContext(w.cli, w.app.Namespace, rev)
	if err != nil {
		err = errors.WithMessage(err, "new context")
		return
	}

	if err = w.setMetadataToContext(wfCtx); err != nil {
		return
	}
	if err = wfCtx.Commit(); err != nil {
		return
	}
	wfStatus.ContextBackend = wfCtx.StoreRef()
	return
}

func (w *workflow) setMetadataToContext(wfCtx wfContext.Context) error {
	metadata, err := value.NewValue(string(util.MustJSONMarshal(w.app.ObjectMeta)), nil)
	if err != nil {
		return err
	}
	return wfCtx.SetVar(metadata, wfTypes.ContextKeyMetadata)
}

func (w *workflow) runAsDag(wfCtx wfContext.Context, taskRunners []wfTypes.TaskRunner) (terminated bool, pause bool, gerr error) {
	status := w.app.Status.Workflow
	var (
		todoTasks    []wfTypes.TaskRunner
		pendingTasks []wfTypes.TaskRunner
	)
	done := true
	for _, tRunner := range taskRunners {
		ready := false
		for _, ss := range status.Steps {
			if ss.Name == tRunner.Name() {
				ready = ss.Phase == common.WorkflowStepPhaseSucceeded
				break
			}
		}
		if !ready {
			done = false
			if tRunner.Pending(wfCtx) {
				pendingTasks = append(pendingTasks, tRunner)
				continue
			}
			todoTasks = append(todoTasks, tRunner)
		}
	}
	if done {
		return
	}

	if len(todoTasks) > 0 {
		var terminated bool
		terminated, pause, gerr = w.run(wfCtx, todoTasks)
		if terminated {
			done = true
			return
		}
		if gerr != nil || pause {
			return
		}
		if len(pendingTasks) > 0 {
			return w.runAsDag(wfCtx, pendingTasks)
		}
	}
	return

}

func (w *workflow) run(wfCtx wfContext.Context, taskRunners []wfTypes.TaskRunner) (terminated bool, pause bool, gerr error) {
	wfStatus := w.app.Status.Workflow
	for _, runner := range taskRunners {
		status, operation, err := runner.Run(wfCtx)
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

		if status.Phase != common.WorkflowStepPhaseSucceeded {
			if w.dagMode {
				continue
			}
			return
		}

		if err := wfCtx.Commit(); err != nil {
			gerr = errors.WithMessage(err, "commit workflow context")
			return
		}

		wfStatus.StepIndex++

		if operation != nil {
			wfStatus.Terminated = operation.Terminated
			wfStatus.Suspend = operation.Suspend
		}

		if wfStatus.Terminated {
			terminated = true
			return
		}
		if wfStatus.Suspend {
			pause = true
			w.app.Status.Phase = common.ApplicationWorkflowSuspending
			return
		}
	}
	return false, false, nil
}
