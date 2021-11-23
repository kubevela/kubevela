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
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/recorder"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

type workflow struct {
	app     *oamcore.Application
	cli     client.Client
	dagMode bool
}

// NewWorkflow returns a Workflow implementation.
func NewWorkflow(app *oamcore.Application, cli client.Client, mode common.WorkflowMode) Workflow {
	dagMode := false
	if mode == common.WorkflowModeDAG {
		dagMode = true
	}
	return &workflow{
		app:     app,
		cli:     cli,
		dagMode: dagMode,
	}
}

// ExecuteSteps process workflow step in order.
func (w *workflow) ExecuteSteps(ctx monitorContext.Context, appRev *oamcore.ApplicationRevision, taskRunners []wfTypes.TaskRunner) (common.WorkflowState, error) {
	revAndSpecHash, err := computeAppRevisionHash(appRev.Name, w.app)
	if err != nil {
		return common.WorkflowStateExecuting, err
	}
	ctx.AddTag("workflow_version", revAndSpecHash)
	if len(taskRunners) == 0 {
		return common.WorkflowStateFinished, nil
	}

	if w.app.Status.Workflow == nil || w.app.Status.Workflow.AppRevision != revAndSpecHash {
		ctx.Info("Restart Workflow")
		status := w.app.Status.Workflow
		if status != nil && !status.Finished {
			status.Terminated = true
			return common.WorkflowStateTerminated, nil
		}
		w.app.Status.Workflow = &common.WorkflowStatus{
			AppRevision: revAndSpecHash,
			Mode:        common.WorkflowModeStep,
			StartTime:   metav1.NewTime(time.Now()),
		}
		if w.dagMode {
			w.app.Status.Workflow.Mode = common.WorkflowModeDAG
		}
		// clean recorded resources info.
		w.app.Status.Services = nil
		w.app.Status.AppliedResources = nil
	}

	wfStatus := w.app.Status.Workflow
	if wfStatus.Finished {
		return common.WorkflowStateFinished, nil
	}
	if wfStatus.Terminated {
		return common.WorkflowStateTerminated, nil
	}
	if wfStatus.Suspend {
		return common.WorkflowStateSuspended, nil
	}
	allTasksDone := w.allDone(taskRunners)
	if allTasksDone {
		return common.WorkflowStateSucceeded, nil
	}

	wfCtx, err := w.makeContext(w.app.Name)
	if err != nil {
		ctx.Error(err, "make context")
		return common.WorkflowStateExecuting, err
	}

	e := &engine{
		status:     wfStatus,
		dagMode:    w.dagMode,
		monitorCtx: ctx,
		app:        w.app,
	}

	err = e.run(wfCtx, taskRunners)
	if err != nil {
		ctx.Error(err, "run steps")
		return common.WorkflowStateExecuting, err
	}
	if wfStatus.Terminated {
		return common.WorkflowStateTerminated, nil
	}
	if wfStatus.Suspend {
		return common.WorkflowStateSuspended, nil
	}
	if w.allDone(taskRunners) {
		return common.WorkflowStateSucceeded, nil
	}
	return common.WorkflowStateExecuting, nil
}

// Trace record the workflow execute history.
func (w *workflow) Trace() error {
	data, err := json.Marshal(w.app)
	if err != nil {
		return err
	}
	return recorder.With(w.cli, w.app).Save("", data).Limit(10).Error()
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

func (w *workflow) makeContext(appName string) (wfCtx wfContext.Context, err error) {
	wfStatus := w.app.Status.Workflow
	if wfStatus.ContextBackend != nil {
		wfCtx, err = wfContext.LoadContext(w.cli, w.app.Namespace, appName)
		if err != nil {
			err = errors.WithMessage(err, "load context")
		}
		return
	}

	wfCtx, err = wfContext.NewContext(w.cli, w.app.Namespace, appName, w.app.GetUID())

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
	copierMeta := w.app.ObjectMeta.DeepCopy()
	copierMeta.ManagedFields = nil
	copierMeta.Finalizers = nil
	copierMeta.OwnerReferences = nil
	metadata, err := value.NewValue(string(util.MustJSONMarshal(copierMeta)), nil, "")
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
	}

	return e.steps(wfCtx, e.todoByIndex(taskRunners))
}

func (e *engine) todoByIndex(taskRunners []wfTypes.TaskRunner) []wfTypes.TaskRunner {
	index := 0
	for _, t := range taskRunners {
		for _, ss := range e.status.Steps {
			if ss.Name == t.Name() {
				if ss.Phase == common.WorkflowStepPhaseSucceeded {
					index++
				}
				break
			}
		}
	}
	return taskRunners[index:]
}

func (e *engine) steps(wfCtx wfContext.Context, taskRunners []wfTypes.TaskRunner) error {
	for _, runner := range taskRunners {
		status, operation, err := runner.Run(wfCtx, &wfTypes.TaskRunOptions{
			GetTracer: func(id string, stepStatus oamcore.WorkflowStep) monitorContext.Context {
				return e.monitorCtx.Fork(id, monitorContext.DurationMetric(func(v float64) {
					metrics.StepDurationSummary.WithLabelValues(e.app.Namespace+"/"+e.app.Name, e.status.AppRevision, stepStatus.Name, stepStatus.Type).Observe(v)
				}))
			},
		})
		if err != nil {
			return err
		}

		e.updateStepStatus(status)

		if err := wfCtx.Commit(); err != nil {
			return errors.WithMessage(err, "commit workflow context")
		}

		if status.Phase != common.WorkflowStepPhaseSucceeded {
			if e.isDag() {
				continue
			}
			return nil
		}

		e.finishStep(operation)
		if e.needStop() {
			return nil
		}
	}
	return nil
}

type engine struct {
	dagMode    bool
	status     *common.WorkflowStatus
	monitorCtx monitorContext.Context
	app        *oamcore.Application
}

func (e *engine) isDag() bool {
	return e.dagMode
}

func (e *engine) finishStep(operation *wfTypes.Operation) {
	if operation != nil {
		e.status.Suspend = operation.Suspend
		e.status.Terminated = operation.Terminated
	}
}

func (e *engine) updateStepStatus(status common.WorkflowStepStatus) {
	var (
		conditionUpdated bool
		now              = metav1.NewTime(time.Now())
	)
	status.LastExecuteTime = now
	for i := range e.status.Steps {
		if e.status.Steps[i].Name == status.Name {
			status.FirstExecuteTime = e.status.Steps[i].FirstExecuteTime
			e.status.Steps[i] = status
			conditionUpdated = true
			break
		}
	}
	if !conditionUpdated {
		status.FirstExecuteTime = now
		e.status.Steps = append(e.status.Steps, status)
	}
}

func (e *engine) needStop() bool {
	return e.status.Suspend || e.status.Terminated
}

func computeAppRevisionHash(rev string, app *oamcore.Application) (string, error) {
	version := ""
	if annos := app.Annotations; annos != nil {
		version = annos[oam.AnnotationPublishVersion]
	}
	if version == "" {
		specHash, err := utils.ComputeSpecHash(app.Spec)
		if err != nil {
			return "", err
		}
		version = fmt.Sprintf("%s:%s", rev, specHash)
	}
	return version, nil
}
