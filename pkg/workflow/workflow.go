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
	"k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/features"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/debug"
	"github.com/oam-dev/kubevela/pkg/workflow/recorder"
	wfTasks "github.com/oam-dev/kubevela/pkg/workflow/tasks"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks/custom"
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

	// MessageTerminatedFailedAfterRetries is the message of failed after retries
	MessageTerminatedFailedAfterRetries = "The workflow terminates automatically because the failed times of steps have reached the limit"
	// MessageSuspendFailedAfterRetries is the message of failed after retries
	MessageSuspendFailedAfterRetries = "The workflow suspends automatically because the failed times of steps have reached the limit"
	// MessageInitializingWorkflow is the message of initializing workflow
	MessageInitializingWorkflow = "Initializing workflow"
)

type workflow struct {
	app     *oamcore.Application
	cli     client.Client
	wfCtx   wfContext.Context
	rk      resourcekeeper.ResourceKeeper
	dagMode bool
	debug   bool
}

// NewWorkflow returns a Workflow implementation.
func NewWorkflow(app *oamcore.Application, cli client.Client, mode common.WorkflowMode, debug bool, rk resourcekeeper.ResourceKeeper) Workflow {
	dagMode := false
	if mode == common.WorkflowModeDAG {
		dagMode = true
	}
	return &workflow{
		app:     app,
		cli:     cli,
		dagMode: dagMode,
		debug:   debug,
		rk:      rk,
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
		return w.restartWorkflow(ctx, revAndSpecHash)
	}

	wfStatus := w.app.Status.Workflow
	cacheKey := fmt.Sprintf("%s-%s", w.app.Name, w.app.Namespace)

	allTasksDone, allTasksSucceeded := w.allDone(taskRunners)
	if wfStatus.Finished {
		StepStatusCache.Delete(cacheKey)
		return common.WorkflowStateFinished, nil
	}
	if checkWorkflowTerminated(wfStatus, allTasksDone) {
		return common.WorkflowStateTerminated, nil
	}
	if wfStatus.Suspend {
		return common.WorkflowStateSuspended, nil
	}
	if allTasksSucceeded {
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

	e := newEngine(ctx, wfCtx, w, wfStatus)

	err = e.Run(taskRunners, w.dagMode)
	if err != nil {
		ctx.Error(err, "run steps")
		StepStatusCache.Store(cacheKey, len(wfStatus.Steps))
		wfStatus.Message = string(common.WorkflowStateExecuting)
		return common.WorkflowStateExecuting, err
	}

	e.checkWorkflowStatusMessage(wfStatus)
	StepStatusCache.Store(cacheKey, len(wfStatus.Steps))
	allTasksDone, allTasksSucceeded = w.allDone(taskRunners)
	if wfStatus.Terminated {
		e.cleanBackoffTimesForTerminated()
		if checkWorkflowTerminated(wfStatus, allTasksDone) {
			wfContext.CleanupMemoryStore(e.app.Name, e.app.Namespace)
			return common.WorkflowStateTerminated, nil
		}
	}
	if wfStatus.Suspend {
		wfContext.CleanupMemoryStore(e.app.Name, e.app.Namespace)
		return common.WorkflowStateSuspended, nil
	}
	if allTasksSucceeded {
		wfStatus.Message = string(common.WorkflowStateSucceeded)
		return common.WorkflowStateSucceeded, nil
	}
	wfStatus.Message = string(common.WorkflowStateExecuting)
	return common.WorkflowStateExecuting, nil
}

func checkWorkflowTerminated(wfStatus *common.WorkflowStatus, allTasksDone bool) bool {
	return (wfStatus.Terminated && allTasksDone) || (wfStatus.Terminated && wfStatus.Suspend)
}

func (w *workflow) restartWorkflow(ctx monitorContext.Context, revAndSpecHash string) (common.WorkflowState, error) {
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

func newEngine(ctx monitorContext.Context, wfCtx wfContext.Context, w *workflow, wfStatus *common.WorkflowStatus) *engine {
	stepStatus := make(map[string]common.StepStatus)
	for _, ss := range wfStatus.Steps {
		stepStatus[ss.Name] = common.StepStatus{
			Phase:  ss.Phase,
			ID:     ss.ID,
			Reason: ss.Reason,
		}
		for _, sss := range ss.SubStepsStatus {
			stepStatus[sss.Name] = common.StepStatus{
				Phase:  sss.Phase,
				ID:     sss.ID,
				Reason: ss.Reason,
			}
		}
	}
	stepDependsOn := make(map[string][]string)
	if w.app.Spec.Workflow != nil {
		for _, step := range w.app.Spec.Workflow.Steps {
			stepDependsOn[step.Name] = append(stepDependsOn[step.Name], step.DependsOn...)
			for _, sub := range step.SubSteps {
				stepDependsOn[sub.Name] = append(stepDependsOn[sub.Name], sub.DependsOn...)
			}
		}
	} else {
		for _, comp := range w.app.Spec.Components {
			stepDependsOn[comp.Name] = append(stepDependsOn[comp.Name], comp.DependsOn...)
		}
	}
	return &engine{
		status:        wfStatus,
		monitorCtx:    ctx,
		app:           w.app,
		wfCtx:         wfCtx,
		cli:           w.cli,
		debug:         w.debug,
		rk:            w.rk,
		stepStatus:    stepStatus,
		stepDependsOn: stepDependsOn,
	}
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

func (w *workflow) allDone(taskRunners []wfTypes.TaskRunner) (bool, bool) {
	success := true
	status := w.app.Status.Workflow
	for _, t := range taskRunners {
		done := false
		for _, ss := range status.Steps {
			if ss.Name == t.Name() {
				done = custom.IsStepFinish(ss.Phase, ss.Reason)
				success = done && (ss.Phase == common.WorkflowStepPhaseSucceeded)
				break
			}
		}
		if !done {
			return false, false
		}
	}
	return true, success
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

func (e *engine) getBackoffTimes(stepID string) (success bool, backoffTimes int) {
	if v, ok := e.wfCtx.GetValueInMemory(wfTypes.ContextPrefixBackoffTimes, stepID); ok {
		times, ok := v.(int)
		if ok {
			return true, times
		}
	}
	return false, 0
}

func (e *engine) getBackoffWaitTime() int {
	// the default value of min times reaches the max workflow backoff wait time
	minTimes := 15
	found := false
	for _, step := range e.status.Steps {
		success, backoffTimes := e.getBackoffTimes(step.ID)
		if success && backoffTimes < minTimes {
			minTimes = backoffTimes
			found = true
		}
		if step.SubStepsStatus != nil {
			for _, subStep := range step.SubStepsStatus {
				success, backoffTimes := e.getBackoffTimes(subStep.ID)
				if success && backoffTimes < minTimes {
					minTimes = backoffTimes
					found = true
				}
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
		finish := false
		var stepID string
		if status, ok := e.stepStatus[tRunner.Name()]; ok {
			stepID = status.ID
			finish = custom.IsStepFinish(status.Phase, status.Reason)
		}
		if !finish {
			done = false
			if tRunner.Pending(wfCtx, e.stepStatus) {
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
		err := e.steps(todoTasks, true)
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

func (e *engine) Run(taskRunners []wfTypes.TaskRunner, dag bool) error {
	var err error
	if dag {
		err = e.runAsDAG(taskRunners)
	} else {
		err = e.steps(taskRunners, dag)
	}

	e.checkFailedAfterRetries()
	e.setNextExecuteTime()
	return err
}

func (e *engine) checkWorkflowStatusMessage(wfStatus *common.WorkflowStatus) {
	switch {
	case !e.waiting && e.failedAfterRetries && feature.DefaultMutableFeatureGate.Enabled(features.EnableSuspendOnFailure):
		e.status.Message = MessageSuspendFailedAfterRetries
	case e.failedAfterRetries && !feature.DefaultMutableFeatureGate.Enabled(features.EnableSuspendOnFailure):
		e.status.Message = MessageTerminatedFailedAfterRetries
	case wfStatus.Terminated:
		e.status.Message = string(common.WorkflowStateTerminated)
	case wfStatus.Suspend:
		e.status.Message = string(common.WorkflowStateSuspended)
	default:
	}
}

func (e *engine) steps(taskRunners []wfTypes.TaskRunner, dag bool) error {
	wfCtx := e.wfCtx
	var err error
	for index, runner := range taskRunners {
		if status, ok := e.stepStatus[runner.Name()]; ok {
			if custom.IsStepFinish(status.Phase, status.Reason) {
				continue
			}
		}
		options := &wfTypes.TaskRunOptions{
			GetTracer: func(id string, stepStatus oamcore.WorkflowStep) monitorContext.Context {
				return e.monitorCtx.Fork(id, monitorContext.DurationMetric(func(v float64) {
					metrics.StepDurationHistogram.WithLabelValues("application", stepStatus.Type).Observe(v)
				}))
			},
			Engine: e,
		}
		if e.debug {
			options.Debug = func(step string, v *value.Value) error {
				debugContext := debug.NewContext(e.cli, e.rk, e.app, step)
				if err := debugContext.Set(v); err != nil {
					return err
				}
				return nil
			}
		}

		var status common.StepStatus
		operation := &wfTypes.Operation{}
		skip := false
		status, skip = runner.Skip(e.findDependPhase(taskRunners, index, dag), e.stepStatus)
		if !skip {
			status, operation, err = runner.Run(wfCtx, options)
			if err != nil {
				return err
			}
		}

		e.updateStepStatus(status)

		e.failedAfterRetries = e.failedAfterRetries || operation.FailedAfterRetries
		e.waiting = e.waiting || operation.Waiting
		// for the suspend step with duration, there's no need to increase the backoff time in reconcile when it's still running
		if !custom.IsStepFinish(status.Phase, status.Reason) && !isWaitSuspendStep(status) {
			if err := handleBackoffTimes(wfCtx, status, false); err != nil {
				return err
			}
			if dag {
				continue
			}
			return nil
		}
		// clear the backoff time when the step is finished
		if err := handleBackoffTimes(wfCtx, status, true); err != nil {
			return err
		}

		e.finishStep(operation)
		if e.needStop() {
			return nil
		}
	}
	return nil
}

type engine struct {
	failedAfterRetries bool
	waiting            bool
	debug              bool
	status             *common.WorkflowStatus
	monitorCtx         monitorContext.Context
	wfCtx              wfContext.Context
	app                *oamcore.Application
	cli                client.Client
	rk                 resourcekeeper.ResourceKeeper
	parentRunner       string
	stepStatus         map[string]common.StepStatus
	stepDependsOn      map[string][]string
}

func (e *engine) finishStep(operation *wfTypes.Operation) {
	if operation != nil {
		e.status.Suspend = operation.Suspend
		e.status.Terminated = e.status.Terminated || operation.Terminated
	}
}

func (e *engine) updateStepStatus(status common.StepStatus) {
	var (
		conditionUpdated bool
		now              = metav1.NewTime(time.Now())
	)

	parentRunner := e.parentRunner
	stepName := status.Name
	if parentRunner != "" {
		stepName = parentRunner
	}
	e.wfCtx.SetValueInMemory(now.Unix(), wfTypes.ContextKeyLastExecuteTime)
	status.LastExecuteTime = now
	index := -1
	for i, ss := range e.status.Steps {
		if ss.Name == stepName {
			index = i
			if parentRunner != "" {
				// update the sub steps status
				for j, sub := range ss.SubStepsStatus {
					if sub.Name == status.Name {
						status.FirstExecuteTime = sub.FirstExecuteTime
						e.status.Steps[i].SubStepsStatus[j].StepStatus = status
						conditionUpdated = true
						break
					}
				}
			} else {
				// update the parent steps status
				status.FirstExecuteTime = ss.FirstExecuteTime
				e.status.Steps[i].StepStatus = status
				conditionUpdated = true
				break
			}
		}
	}
	if !conditionUpdated {
		status.FirstExecuteTime = now
		if parentRunner != "" {
			if index < 0 {
				e.status.Steps = append(e.status.Steps, common.WorkflowStepStatus{
					StepStatus: common.StepStatus{
						Name:             parentRunner,
						FirstExecuteTime: now,
					}})
				index = len(e.status.Steps) - 1
			}
			e.status.Steps[index].SubStepsStatus = append(e.status.Steps[index].SubStepsStatus, common.WorkflowSubStepStatus{StepStatus: status})
		} else {
			e.status.Steps = append(e.status.Steps, common.WorkflowStepStatus{StepStatus: status})
		}
	}
	e.stepStatus[status.Name] = status
}

func (e *engine) checkFailedAfterRetries() {
	if !e.waiting && e.failedAfterRetries && feature.DefaultMutableFeatureGate.Enabled(features.EnableSuspendOnFailure) {
		e.status.Suspend = true
	}
	if e.failedAfterRetries && !feature.DefaultMutableFeatureGate.Enabled(features.EnableSuspendOnFailure) {
		e.status.Terminated = true
	}
}

func (e *engine) needStop() bool {
	if feature.DefaultMutableFeatureGate.Enabled(features.EnableSuspendOnFailure) {
		e.checkFailedAfterRetries()
	}
	// if the workflow is terminated, we still need to execute all the remaining steps
	return e.status.Suspend
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
	return app.Status.Workflow != nil && (app.Status.Workflow.Message == MessageTerminatedFailedAfterRetries || app.Status.Workflow.Message == MessageSuspendFailedAfterRetries)
}

func (e *engine) findDependPhase(taskRunners []wfTypes.TaskRunner, index int, dag bool) common.WorkflowStepPhase {
	if e.parentRunner != "" {
		if status, ok := e.stepStatus[e.parentRunner]; ok && status.Phase == common.WorkflowStepPhaseSkipped {
			return common.WorkflowStepPhaseSkipped
		}
	}
	if dag {
		return e.findDependsOnPhase(taskRunners[index].Name())
	}
	if index < 1 {
		return common.WorkflowStepPhaseSucceeded
	}
	for i := index - 1; i >= 0; i-- {
		if e.stepStatus[taskRunners[i].Name()].Phase != common.WorkflowStepPhaseSucceeded {
			return e.stepStatus[taskRunners[i].Name()].Phase
		}
	}
	return e.stepStatus[taskRunners[index-1].Name()].Phase
}

func (e *engine) findDependsOnPhase(name string) common.WorkflowStepPhase {
	for _, dependsOn := range e.stepDependsOn[name] {
		if e.stepStatus[dependsOn].Phase != common.WorkflowStepPhaseSucceeded {
			return e.stepStatus[dependsOn].Phase
		}
		if result := e.findDependsOnPhase(dependsOn); result != common.WorkflowStepPhaseSucceeded {
			return result
		}
	}
	return common.WorkflowStepPhaseSucceeded
}

func isWaitSuspendStep(step common.StepStatus) bool {
	return step.Type == wfTypes.WorkflowStepTypeSuspend && step.Phase == common.WorkflowStepPhaseRunning
}

func handleBackoffTimes(wfCtx wfContext.Context, status common.StepStatus, clear bool) error {
	if clear {
		wfCtx.DeleteValueInMemory(wfTypes.ContextPrefixBackoffTimes, status.ID)
		wfCtx.DeleteValueInMemory(wfTypes.ContextPrefixBackoffReason, status.ID)
	} else {
		if val, exists := wfCtx.GetValueInMemory(wfTypes.ContextPrefixBackoffReason, status.ID); !exists || val != status.Message {
			wfCtx.SetValueInMemory(status.Message, wfTypes.ContextPrefixBackoffReason, status.ID)
			wfCtx.DeleteValueInMemory(wfTypes.ContextPrefixBackoffTimes, status.ID)
		}
		wfCtx.IncreaseCountValueInMemory(wfTypes.ContextPrefixBackoffTimes, status.ID)
	}
	if err := wfCtx.Commit(); err != nil {
		return errors.WithMessage(err, "commit workflow context")
	}
	return nil
}

func (e *engine) cleanBackoffTimesForTerminated() {
	for _, ss := range e.status.Steps {
		for _, sub := range ss.SubStepsStatus {
			if sub.Reason == custom.StatusReasonTerminate {
				e.wfCtx.DeleteValueInMemory(wfTypes.ContextPrefixBackoffTimes, sub.ID)
				e.wfCtx.DeleteValueInMemory(wfTypes.ContextPrefixBackoffReason, sub.ID)
			}
		}
		if ss.Reason == custom.StatusReasonTerminate {
			e.wfCtx.DeleteValueInMemory(wfTypes.ContextPrefixBackoffTimes, ss.ID)
			e.wfCtx.DeleteValueInMemory(wfTypes.ContextPrefixBackoffReason, ss.ID)
		}
	}
}

func (e *engine) GetStepStatus(stepName string) common.WorkflowStepStatus {
	// ss is step status
	for _, ss := range e.status.Steps {
		if ss.Name == stepName {
			return ss
		}
	}
	return common.WorkflowStepStatus{}
}

func (e *engine) SetParentRunner(name string) {
	e.parentRunner = name
}

func (e *engine) GetOperation() *wfTypes.Operation {
	return &wfTypes.Operation{
		Suspend:            e.status.Suspend,
		Terminated:         e.status.Terminated,
		Waiting:            e.waiting,
		FailedAfterRetries: e.failedAfterRetries,
	}
}
