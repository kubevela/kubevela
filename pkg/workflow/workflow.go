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
	"math"
	"sync"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/recorder"
	wfTasks "github.com/oam-dev/kubevela/pkg/workflow/tasks"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

var (
	// DisableRecorder optimize workflow by disable recorder
	DisableRecorder = false
	// StepStatusCache cache the step status
	StepStatusCache sync.Map
	// MaxWorkflowWaitBackoffTime is the max time to wait before reconcile wait workflow again
	MaxWorkflowWaitBackoffTime = 60
	// MaxWorkflowFailedBackoffTime is the max time to wait before reconcile failed workflow again
	MaxWorkflowFailedBackoffTime = 300
)

const (
	// minWorkflowBackoffWaitTime is the min time to wait before reconcile workflow again
	minWorkflowBackoffWaitTime = 1
	// backoffTimeCoefficient is the coefficient of time to wait before reconcile workflow again
	backoffTimeCoefficient = 0.05

	// MessageFailedAfterRetries is the message of failed after retries
	MessageFailedAfterRetries = "The workflow suspends automatically because the failed times of steps have reached the limit"
	// MessageInitializingWorkflow is the message of initializing workflow
	MessageInitializingWorkflow = "Initializing workflow"
)

type workflow struct {
	app     *oamcore.Application
	cli     client.Client
	wfCtx   wfContext.Context
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
	revAndSpecHash, err := ComputeWorkflowRevisionHash(appRev.Name, w.app)
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
			StartTime:   metav1.Now(),
		}
		w.app.Status.Workflow.Message = MessageInitializingWorkflow
		if w.dagMode {
			w.app.Status.Workflow.Mode = common.WorkflowModeDAG
		}
		// clean recorded resources info.
		w.app.Status.Services = nil
		w.app.Status.AppliedResources = nil

		// clean conditions after render
		var reservedConditions []condition.Condition
		for i, cond := range w.app.Status.Conditions {
			condTpy, err := common.ParseApplicationConditionType(string(cond.Type))
			if err == nil {
				if condTpy <= common.RenderCondition {
					reservedConditions = append(reservedConditions, w.app.Status.Conditions[i])
				}
			}
		}
		w.app.Status.Conditions = reservedConditions
		StepStatusCache.Delete(fmt.Sprintf("%s-%s", w.app.Name, w.app.Namespace))
		wfContext.CleanupMemoryStore(w.app.Name, w.app.Namespace)
		return common.WorkflowStateInitializing, nil
	}

	wfStatus := w.app.Status.Workflow
	cacheKey := fmt.Sprintf("%s-%s", w.app.Name, w.app.Namespace)

	if wfStatus.Finished {
		StepStatusCache.Delete(cacheKey)
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
		wfStatus.Message = string(common.WorkflowStateExecuting)
		return common.WorkflowStateExecuting, err
	}
	w.wfCtx = wfCtx

	if cacheValue, ok := StepStatusCache.Load(cacheKey); ok {
		// handle cache resource
		if len(wfStatus.Steps) < cacheValue.(int) {
			return common.WorkflowStateSkipping, nil
		}
	}

	e := &engine{
		status:     wfStatus,
		dagMode:    w.dagMode,
		monitorCtx: ctx,
		app:        w.app,
		wfCtx:      wfCtx,
	}

	err = e.run(taskRunners)
	if err != nil {
		ctx.Error(err, "run steps")
		StepStatusCache.Store(cacheKey, len(wfStatus.Steps))
		wfStatus.Message = string(common.WorkflowStateExecuting)
		return common.WorkflowStateExecuting, err
	}

	e.checkWorkflowStatusMessage(wfStatus)
	StepStatusCache.Store(cacheKey, len(wfStatus.Steps))
	if wfStatus.Terminated {
		wfContext.CleanupMemoryStore(e.app.Name, e.app.Namespace)
		return common.WorkflowStateTerminated, nil
	}
	if wfStatus.Suspend {
		wfContext.CleanupMemoryStore(e.app.Name, e.app.Namespace)
		return common.WorkflowStateSuspended, nil
	}
	if w.allDone(taskRunners) {
		wfStatus.Message = string(common.WorkflowStateSucceeded)
		return common.WorkflowStateSucceeded, nil
	}
	wfStatus.Message = string(common.WorkflowStateExecuting)
	return common.WorkflowStateExecuting, nil
}

// Trace record the workflow execute history.
func (w *workflow) Trace() error {
	if DisableRecorder {
		return nil
	}
	data, err := json.Marshal(w.app)
	if err != nil {
		return err
	}
	return recorder.With(w.cli, w.app).Save("", data).Limit(10).Error()
}

func (w *workflow) GetBackoffWaitTime() time.Duration {
	nextTime, ok := w.wfCtx.GetValueInMemory(wfTypes.ContextKeyNextExecuteTime)
	if !ok {
		return time.Second
	}
	unix, ok := nextTime.(int64)
	if !ok {
		return time.Second
	}
	next := time.Unix(unix, 0)
	if next.After(time.Now()) {
		return time.Until(next)
	}

	return time.Second
}

func (w *workflow) HandleSuspendWait(ctx monitorContext.Context) (doWaiting bool, durationWaiting time.Duration, errRet error) {
	ctx.Info("handle suspend wait")
	for i, stepStatus := range w.app.Status.Workflow.Steps {
		if !w.isWaitSuspendStep(stepStatus) {
			continue
		}

		step := w.getWorkflowStepByName(stepStatus.Name)
		if step.Name == "" {
			errRet = fmt.Errorf("failed to get workflow step by name: %s", stepStatus.Name)
			return
		}

		d, wd, err := wfTasks.GetSuspendStepDurationWaiting(step)
		if err != nil {
			ctx.Error(err, "failed to get suspend step wait duration")
			errRet = err
			return
		}

		if d {
			doWaiting = d
			remainingDuration := wd - time.Since(stepStatus.FirstExecuteTime.Time)
			if remainingDuration <= 0 {
				w.app.Status.Workflow.Steps[i].Phase = common.WorkflowStepPhaseSucceeded
			}

			if remainingDuration > 0 && (durationWaiting > remainingDuration || durationWaiting <= 0) {
				suspendState := fmt.Sprintf("durationWaiting(%s)", wd.String())
				if w.app.Status.Workflow.SuspendState != suspendState {
					w.app.Status.Workflow.SuspendState = suspendState
				}
				durationWaiting = remainingDuration
			}
		}

		if !w.dagMode {
			return
		}
	}

	return doWaiting, durationWaiting, errRet
}

func (w *workflow) isWaitSuspendStep(status common.WorkflowStepStatus) bool {
	return status.Type == wfTypes.WorkflowStepTypeSuspend && status.Phase == common.WorkflowStepPhaseRunning
}

func (w *workflow) getWorkflowStepByName(name string) oamcore.WorkflowStep {
	for _, s := range w.app.Spec.Workflow.Steps {
		if s.Name == name {
			return s
		}
	}

	return oamcore.WorkflowStep{}
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

func (e *engine) getBackoffWaitTime() int {
	// the default value of min times reaches the max workflow backoff wait time
	minTimes := 15
	found := false
	for _, step := range e.status.Steps {
		if v, ok := e.wfCtx.GetValueInMemory(wfTypes.ContextPrefixBackoffTimes, step.ID); ok {
			found = true
			times, ok := v.(int)
			if !ok {
				times = 0
			}
			if times < minTimes {
				minTimes = times
			}
		}
	}
	if !found {
		return minWorkflowBackoffWaitTime
	}

	interval := int(math.Pow(2, float64(minTimes)) * backoffTimeCoefficient)
	if interval < minWorkflowBackoffWaitTime {
		return minWorkflowBackoffWaitTime
	}
	maxWorkflowBackoffWaitTime := e.getMaxBackoffWaitTime()
	if interval > maxWorkflowBackoffWaitTime {
		return maxWorkflowBackoffWaitTime
	}
	return interval
}

func (e *engine) getMaxBackoffWaitTime() int {
	for _, step := range e.status.Steps {
		if step.Phase == common.WorkflowStepPhaseFailed {
			return MaxWorkflowFailedBackoffTime
		}
	}
	return MaxWorkflowWaitBackoffTime
}

func (e *engine) setNextExecuteTime() {
	interval := e.getBackoffWaitTime()
	lastExecuteTime, ok := e.wfCtx.GetValueInMemory(wfTypes.ContextKeyLastExecuteTime)
	if !ok {
		e.monitorCtx.Error(fmt.Errorf("failed to get last execute time"), "application", e.app.Name)
	}

	last, ok := lastExecuteTime.(int64)
	if !ok {
		e.monitorCtx.Error(fmt.Errorf("failed to parse last execute time to int64"), "lastExecuteTime", lastExecuteTime)
	}

	next := last + int64(interval)
	e.wfCtx.SetValueInMemory(next, wfTypes.ContextKeyNextExecuteTime)
	if err := e.wfCtx.Commit(); err != nil {
		e.monitorCtx.Error(err, "failed to commit next execute time", "nextExecuteTime", next)
	}
}

func (e *engine) runAsDAG(taskRunners []wfTypes.TaskRunner) error {
	var (
		todoTasks    []wfTypes.TaskRunner
		pendingTasks []wfTypes.TaskRunner
	)
	wfCtx := e.wfCtx
	done := true
	for _, tRunner := range taskRunners {
		ready := false
		var stepID string
		for _, ss := range e.status.Steps {
			if ss.Name == tRunner.Name() {
				stepID = ss.ID
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
		} else {
			wfCtx.DeleteValueInMemory(wfTypes.ContextPrefixBackoffTimes, stepID)
		}
	}
	if done {
		return nil
	}

	if len(todoTasks) > 0 {
		err := e.steps(todoTasks)
		if err != nil {
			return err
		}

		if e.needStop() {
			return nil
		}

		if len(pendingTasks) > 0 {
			return e.runAsDAG(pendingTasks)
		}
	}
	return nil

}

func (e *engine) run(taskRunners []wfTypes.TaskRunner) error {
	var err error
	if e.dagMode {
		err = e.runAsDAG(taskRunners)
	} else {
		err = e.steps(e.todoByIndex(taskRunners))
	}

	e.setNextExecuteTime()
	return err
}

func (e *engine) checkWorkflowStatusMessage(wfStatus *common.WorkflowStatus) {
	if !e.waiting && e.failedAfterRetries {
		e.status.Message = MessageFailedAfterRetries
		return
	}

	if wfStatus.Terminated {
		e.status.Message = string(common.WorkflowStateTerminated)
	}
	if wfStatus.Suspend {
		e.status.Message = string(common.WorkflowStateSuspended)
	}
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

func (e *engine) steps(taskRunners []wfTypes.TaskRunner) error {
	wfCtx := e.wfCtx
	for _, runner := range taskRunners {
		status, operation, err := runner.Run(wfCtx, &wfTypes.TaskRunOptions{
			GetTracer: func(id string, stepStatus oamcore.WorkflowStep) monitorContext.Context {
				return e.monitorCtx.Fork(id, monitorContext.DurationMetric(func(v float64) {
					metrics.StepDurationHistogram.WithLabelValues("application", stepStatus.Type).Observe(v)
				}))
			},
		})
		if err != nil {
			return err
		}

		e.updateStepStatus(status)

		e.failedAfterRetries = e.failedAfterRetries || operation.FailedAfterRetries
		e.waiting = e.waiting || operation.Waiting
		if status.Phase == common.WorkflowStepPhaseSucceeded || (status.Phase == common.WorkflowStepPhaseRunning && status.Type == wfTypes.WorkflowStepTypeSuspend) {
			wfCtx.DeleteValueInMemory(wfTypes.ContextPrefixBackoffTimes, status.ID)
			if err := wfCtx.Commit(); err != nil {
				return errors.WithMessage(err, "commit workflow context")
			}

			e.finishStep(operation)
			if e.needStop() {
				return nil
			}
			continue
		}

		wfCtx.IncreaseCountValueInMemory(wfTypes.ContextPrefixBackoffTimes, status.ID)
		if err := wfCtx.Commit(); err != nil {
			return errors.WithMessage(err, "commit workflow context")
		}
		if e.isDag() {
			continue
		}
		e.checkFailedAfterRetries()
		return nil
	}
	return nil
}

type engine struct {
	dagMode            bool
	failedAfterRetries bool
	waiting            bool
	status             *common.WorkflowStatus
	monitorCtx         monitorContext.Context
	wfCtx              wfContext.Context
	app                *oamcore.Application
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

	e.wfCtx.SetValueInMemory(now.Unix(), wfTypes.ContextKeyLastExecuteTime)
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

func (e *engine) checkFailedAfterRetries() {
	if !e.waiting && e.failedAfterRetries {
		e.status.Suspend = true
	}
}

func (e *engine) needStop() bool {
	e.checkFailedAfterRetries()
	return e.status.Suspend || e.status.Terminated
}

// ComputeWorkflowRevisionHash compute workflow revision.
func ComputeWorkflowRevisionHash(rev string, app *oamcore.Application) (string, error) {
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

// IsFailedAfterRetry check if application is hang due to FailedAfterRetry
func IsFailedAfterRetry(app *oamcore.Application) bool {
	return app.Status.Workflow != nil && app.Status.Workflow.Message == MessageFailedAfterRetries
}
