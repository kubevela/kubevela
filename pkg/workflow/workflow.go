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

// NewDAGWorkflow returns a DAG mode Workflow.
func NewDAGWorkflow(app *oamcore.Application, cli client.Client) Workflow {
	return &workflow{
		app:     app,
		cli:     cli,
		dagMode: true,
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

	e := &engine{
		status:  wfStatus,
		dagMode: w.dagMode,
	}

	gerr = e.run(wfCtx, taskRunners)

	if wfStatus.Suspend {
		pause = true
		w.app.Status.Phase = common.ApplicationWorkflowSuspending
	}

	if wfStatus.Terminated {
		done = true
		w.app.Status.Phase = common.ApplicationWorkflowTerminated
	} else {
		done = w.allDone(taskRunners)
	}
	return done, pause, gerr
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

func (e *engine) runAsDAG(wfCtx wfContext.Context, taskRunners []wfTypes.TaskRunner) error {
	var (
		todoTasks    []wfTypes.TaskRunner
		pendingTasks []wfTypes.TaskRunner
	)
	done := true
	for _, tRunner := range taskRunners {
		ready := false
		for _, ss := range e.status.Steps {
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
		return nil
	}

	if len(todoTasks) > 0 {
		err := e.steps(wfCtx, todoTasks)
		if err != nil {
			return err
		}
		if e.needStop() {
			return nil
		}

		if len(pendingTasks) > 0 {
			return e.runAsDAG(wfCtx, pendingTasks)
		}
	}
	return nil

}

func (e *engine) run(wfCtx wfContext.Context, taskRunners []wfTypes.TaskRunner) error {
	if e.dagMode {
		return e.runAsDAG(wfCtx, taskRunners)
	} else {
		return e.steps(wfCtx, taskRunners[e.status.StepIndex:])
	}
}

func (e *engine) steps(wfCtx wfContext.Context, taskRunners []wfTypes.TaskRunner) error {
	for _, runner := range taskRunners {
		status, operation, err := runner.Run(wfCtx, &wfTypes.TaskRunOptions{
			RunSteps: func(isDag bool, runners ...wfTypes.TaskRunner) (*common.WorkflowStatus, error) {
				stepsEngine := &engine{
					dagMode: isDag,
				}
				stepStatus := e.getStepStatus(runner.Name())

				if stepStatus != nil {
					stepsEngine.status.StepIndex = stepStatus.SubSteps.StepIndex
					stepsEngine.status.Steps = stepStatus.SubSteps.Steps
				}
				err := stepsEngine.run(wfCtx, runners)
				return stepsEngine.status, err
			},
		})
		if err != nil {
			return err
		}

		e.updateStepStatus(status)

		if status.Phase != common.WorkflowStepPhaseSucceeded {
			if e.isDag() {
				continue
			}
			return nil
		}

		if err := wfCtx.Commit(); err != nil {
			return errors.WithMessage(err, "commit workflow context")
		}

		e.finishStep(operation)
		if e.needStop() {
			return nil
		}
	}
	return nil
}

type engine struct {
	dagMode bool
	status  *common.WorkflowStatus
}

func (e *engine) getStepStatus(name string) *common.WorkflowStepStatus {
	for i := range e.status.Steps {
		if e.status.Steps[i].Name == name {
			return &e.status.Steps[i]
			break
		}
	}
	return nil
}

func (e *engine) isDag() bool {
	return e.dagMode
}

func (e *engine) finishStep(operation *wfTypes.Operation) {
	e.status.StepIndex++
	if operation != nil {
		e.status.Suspend = operation.Suspend
		e.status.Terminated = operation.Terminated
	}
}

func (e *engine) updateStepStatus(status common.WorkflowStepStatus) {
	var conditionUpdated bool
	for i := range e.status.Steps {
		if e.status.Steps[i].Name == status.Name {
			e.status.Steps[i] = status
			conditionUpdated = true
			break
		}
	}
	if !conditionUpdated {
		e.status.Steps = append(e.status.Steps, status)
	}
}

func (e *engine) needStop() bool {
	return e.status.Suspend == true || e.status.Terminated == true
}
