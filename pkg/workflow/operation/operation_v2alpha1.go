/*
 Copyright 2026. The KubeVela Authors.

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

package operation

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	wfContext "github.com/kubevela/workflow/pkg/context"
	"github.com/kubevela/workflow/pkg/cue/model/sets"
	wfTypes "github.com/kubevela/workflow/pkg/types"
	wfUtils "github.com/kubevela/workflow/pkg/utils"

	v2alpha1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v2alpha1"
)

// NewOperationWorkflowOperator gets a workflow operator with k8sClient,
// ioWriter (optional, useful for cli) and an Operation.
func NewOperationWorkflowOperator(cli client.Client, w io.Writer, op *v2alpha1.Operation) wfUtils.WorkflowOperator {
	return operationWorkflowOperator{
		cli:          cli,
		outputWriter: w,
		operation:    op,
	}
}

// NewOperationWorkflowStepOperator gets a workflow step operator with
// k8sClient, ioWriter (optional, useful for cli) and an Operation.
func NewOperationWorkflowStepOperator(cli client.Client, w io.Writer, op *v2alpha1.Operation) wfUtils.WorkflowStepOperator {
	return operationWorkflowStepOperator{
		cli:          cli,
		outputWriter: w,
		operation:    op,
	}
}

type operationWorkflowOperator struct {
	cli          client.Client
	outputWriter io.Writer
	operation    *v2alpha1.Operation
}

type operationWorkflowStepOperator struct {
	cli          client.Client
	outputWriter io.Writer
	operation    *v2alpha1.Operation
}

func (wo operationWorkflowStepOperator) asOperator() operationWorkflowOperator {
	return operationWorkflowOperator{cli: wo.cli, outputWriter: wo.outputWriter, operation: wo.operation}
}

// unwrapSteps copies out the plain engine-shaped status wfUtils.OperateSteps
// (the one vendored helper this file calls directly) expects -- it takes
// []workflowv1alpha1.WorkflowStepStatus, not our own
// []v2alpha1.OperationWorkflowStepStatus. Pair with rewrapSteps to write the
// mutations back.
func unwrapSteps(steps []v2alpha1.OperationWorkflowStepStatus) []workflowv1alpha1.WorkflowStepStatus {
	if len(steps) == 0 {
		return nil
	}
	out := make([]workflowv1alpha1.WorkflowStepStatus, len(steps))
	for i, s := range steps {
		out[i] = s.WorkflowStepStatus
	}
	return out
}

// rewrapSteps writes engineSteps (as mutated in place by wfUtils.OperateSteps)
// back into ws, preserving each entry's own Attempts.
func rewrapSteps(ws []v2alpha1.OperationWorkflowStepStatus, engineSteps []workflowv1alpha1.WorkflowStepStatus) {
	for i := range ws {
		ws[i].WorkflowStepStatus = engineSteps[i]
	}
}

// workflowStatus returns the single (local-cluster) workflow status entry --
// multi-cluster dispatch isn't implemented yet, so index 0 is always the
// right (and only) entry once a workflow has started.
func (wo operationWorkflowOperator) workflowStatus() (*v2alpha1.OperationWorkflowStatus, error) {
	op := wo.operation
	if len(op.Status.Workflows) == 0 {
		return nil, fmt.Errorf("operation %q has not started a workflow yet", op.Name)
	}
	return &op.Status.Workflows[0], nil
}

// Suspend suspends the whole workflow.
func (wo operationWorkflowOperator) Suspend(ctx context.Context) error {
	return wo.suspend(ctx, "")
}

// Suspend suspends the workflow from a specific step.
func (wo operationWorkflowStepOperator) Suspend(ctx context.Context, step string) error {
	if step == "" {
		return fmt.Errorf("step can not be empty")
	}
	return wo.asOperator().suspend(ctx, step)
}

func (wo operationWorkflowOperator) suspend(ctx context.Context, step string) error {
	op := wo.operation
	if op.IsTerminal() {
		return fmt.Errorf("can not suspend a terminated operation")
	}
	ws, err := wo.workflowStatus()
	if err != nil {
		return err
	}

	ws.Suspend = true
	steps := unwrapSteps(ws.Steps)
	found := step == ""

	for i, s := range steps {
		for j, sub := range s.SubStepsStatus {
			if sub.Phase != workflowv1alpha1.WorkflowStepPhaseRunning {
				continue
			}
			if step == "" {
				wfUtils.OperateSteps(steps, i, j, workflowv1alpha1.WorkflowStepPhaseSuspending)
			} else if step == sub.Name {
				wfUtils.OperateSteps(steps, i, j, workflowv1alpha1.WorkflowStepPhaseSuspending)
				found = true
				break
			}
		}
		if s.Phase != workflowv1alpha1.WorkflowStepPhaseRunning {
			continue
		}
		if step == "" {
			wfUtils.OperateSteps(steps, i, -1, workflowv1alpha1.WorkflowStepPhaseSuspending)
		} else if step == s.Name {
			wfUtils.OperateSteps(steps, i, -1, workflowv1alpha1.WorkflowStepPhaseSuspending)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("can not find step %s", step)
	}
	rewrapSteps(ws.Steps, steps)

	if err := wo.cli.Status().Update(ctx, op); err != nil {
		return err
	}
	if step == "" {
		return writeOutputF(wo.outputWriter, "Successfully suspend operation: %s\n", op.Name)
	}
	return writeOutputF(wo.outputWriter, "Successfully suspend operation %s from step %s\n", op.Name, step)
}

// Resume resumes a suspended workflow.
func (wo operationWorkflowOperator) Resume(ctx context.Context) error {
	return wo.resume(ctx, "")
}

// Resume resumes a suspended workflow from a specific step.
func (wo operationWorkflowStepOperator) Resume(ctx context.Context, step string) error {
	if step == "" {
		return fmt.Errorf("step can not be empty")
	}
	return wo.asOperator().resume(ctx, step)
}

func (wo operationWorkflowOperator) resume(ctx context.Context, step string) error {
	op := wo.operation
	if op.IsTerminal() {
		return fmt.Errorf("can not resume a terminated operation")
	}
	ws, err := wo.workflowStatus()
	if err != nil {
		return err
	}
	if !ws.Suspend {
		// A hard error here (rather than a printed message and a nil
		// return) matters because the CLI unconditionally polls to
		// terminal after Resume returns nil -- silently no-op'ing would
		// let that poll observe and report some unrelated later
		// completion as if this resume caused it.
		return fmt.Errorf("operation %s is not suspended", op.Name)
	}

	ws.Suspend = false
	steps := unwrapSteps(ws.Steps)
	found := step == ""

	for i, s := range steps {
		for j, sub := range s.SubStepsStatus {
			if sub.Phase != workflowv1alpha1.WorkflowStepPhaseSuspending {
				continue
			}
			if step == "" {
				wfUtils.OperateSteps(steps, i, j, workflowv1alpha1.WorkflowStepPhaseRunning)
			} else if step == sub.Name {
				wfUtils.OperateSteps(steps, i, j, workflowv1alpha1.WorkflowStepPhaseRunning)
				found = true
				break
			}
		}
		if s.Phase != workflowv1alpha1.WorkflowStepPhaseSuspending {
			continue
		}
		if step == "" {
			wfUtils.OperateSteps(steps, i, -1, workflowv1alpha1.WorkflowStepPhaseRunning)
		} else if step == s.Name {
			wfUtils.OperateSteps(steps, i, -1, workflowv1alpha1.WorkflowStepPhaseRunning)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("can not find step %s", step)
	}
	rewrapSteps(ws.Steps, steps)

	if err := wo.cli.Status().Update(ctx, op); err != nil {
		return err
	}
	if step == "" {
		return writeOutputF(wo.outputWriter, "Successfully resume operation: %s\n", op.Name)
	}
	return writeOutputF(wo.outputWriter, "Successfully resume operation %s from step %s\n", op.Name, step)
}

// Rollback is not supported for Operation.
func (wo operationWorkflowOperator) Rollback(_ context.Context) error {
	return fmt.Errorf("cannot rollback an Operation")
}

// Restart restarts the whole workflow.
//
// Unlike CleanStatusFromStep/RestartFromStep upstream, restarting is not
// gated on the operation's (or, for --step, the target step's) current
// phase being Failed. The operator is trusted to know what it's doing.
func (wo operationWorkflowOperator) Restart(ctx context.Context) error {
	return wo.restartFrom(ctx, "")
}

// Restart restarts the workflow from a specific step.
func (wo operationWorkflowStepOperator) Restart(ctx context.Context, step string) error {
	if step == "" {
		return fmt.Errorf("step can not be empty")
	}
	return wo.asOperator().restartFrom(ctx, step)
}

func (wo operationWorkflowOperator) restartFrom(ctx context.Context, step string) error {
	op := wo.operation
	if op.Status.Phase == v2alpha1.OperationPhaseRunning {
		return fmt.Errorf("cannot restart a running operation")
	}
	ws, err := wo.workflowStatus()
	if err != nil {
		return err
	}

	if step != "" {
		if err := wo.restartFromStep(ctx, ws, step); err != nil {
			return err
		}
	} else {
		// Whole-workflow restart: every recorded step is being re-attempted,
		// so snapshot everything before the wipe. Mirrors RestartWorkflow's
		// whole-workflow path exactly, including deleting the old
		// context-backend ConfigMap -- the wipe below drops the reference to
		// it, so leaving the object behind would just leak it.
		if ws.ContextBackend != nil {
			cm := &corev1.ConfigMap{}
			err := wo.cli.Get(ctx, client.ObjectKey{Namespace: op.Namespace, Name: ws.ContextBackend.Name}, cm)
			switch {
			case err == nil:
				if err := wo.cli.Delete(ctx, cm); err != nil {
					return err
				}
			case !kerrors.IsNotFound(err):
				return err
			}
		}
		snapshotAttempts(ws, ws.Steps)
		ws.Mode = oamv1alpha1.WorkflowExecuteMode{}
		ws.Phase = ""
		ws.Message = ""
		ws.Suspend = false
		ws.SuspendState = ""
		ws.Terminated = false
		ws.Finished = false
		ws.ContextBackend = nil
		ws.Steps = nil
		ws.StartTime = metav1.Time{}
		ws.EndTime = metav1.Time{}
	}

	op.Status.Phase = v2alpha1.OperationPhaseRunning
	op.Status.CompletionTime = nil
	op.Status.Message = ""
	op.Status.Attempts++

	if err := wo.cli.Status().Update(ctx, op); err != nil {
		return err
	}
	if step == "" {
		return writeOutputF(wo.outputWriter, "Successfully restart operation: %s\n", op.Name)
	}
	return writeOutputF(wo.outputWriter, "Successfully restart operation %s from step %s\n", op.Name, step)
}

// restartFromStep is the --step counterpart to the whole-workflow wipe in
// restartFrom: it resets only the target step and its dependency set (in
// Operations' default sequential mode, that's every step positioned after
// it), and strips the same step+dependency set's recorded outputs from the
// context-backend ConfigMap so a step before the restart point still hands
// its already-recorded output to a restarted step reading it via
// `inputs: [{from: ...}]`.
func (wo operationWorkflowOperator) restartFromStep(ctx context.Context, ws *v2alpha1.OperationWorkflowStatus, stepName string) error {
	op := wo.operation
	if op.Status.Template == nil {
		return fmt.Errorf("operation %q has not resolved a template yet", op.Name)
	}

	// Mirrors RestartFromStep exactly: a step-scoped restart still has to
	// clear the whole-run flags a prior terminal run left behind, not just
	// the target step's own status. Missing this left Terminated (or
	// Suspend/Finished) stuck at true from the original run, so the engine
	// reported the workflow terminated/failed again on the very next
	// reconcile regardless of how the restarted step itself turned out.
	//
	// Phase/Message/SuspendState are also cleared here, purely so a status
	// read in the brief window before the next reconcile overwrites them
	// doesn't show a stale terminal phase/message from the run being
	// restarted -- the embedded engine itself never reads these back as
	// input (only Terminated gates re-execution, per
	// checkWorkflowTerminated), so this is cosmetic, not a correctness fix.
	// Mode is deliberately left alone: it's the workflow's configured
	// DAG/sequential execution mode, not run state, and zeroing it would
	// silently revert a DAG-mode template back to sequential.
	ws.Terminated = false
	ws.Suspend = false
	ws.Finished = false
	ws.Phase = ""
	ws.Message = ""
	ws.SuspendState = ""
	if !ws.EndTime.IsZero() {
		ws.EndTime = metav1.Time{}
	}

	steps := op.Status.Template.Workflow.Steps

	stepStatus, affected, dependency, err := cleanOperationStatusFromStep(steps, ws.Steps, ws.Mode, stepName)
	if err != nil {
		return err
	}

	var cm *corev1.ConfigMap
	if ws.ContextBackend != nil {
		cm = &corev1.ConfigMap{}
		err := wo.cli.Get(ctx, client.ObjectKey{Namespace: op.Namespace, Name: ws.ContextBackend.Name}, cm)
		switch {
		case err == nil:
		case kerrors.IsNotFound(err):
			// Mirrors the whole-workflow path: a ConfigMap that's already
			// gone just means there's no recorded context to clean up, not
			// a reason to abort the restart.
			cm = nil
		default:
			return err
		}
	}

	snapshotAttempts(ws, affected)
	ws.Steps = stepStatus

	if cm != nil && cm.Data != nil {
		s, err := clearOperationContextVars(steps, cm.Data[wfContext.ConfigMapKeyVars], stepName, dependency)
		if err != nil {
			return err
		}
		cm.Data[wfContext.ConfigMapKeyVars] = s
		if err := wo.cli.Update(ctx, cm); err != nil {
			return err
		}
	}
	return nil
}

// Terminate terminates a running operation.
func (wo operationWorkflowOperator) Terminate(ctx context.Context) error {
	op := wo.operation
	op.Status.Phase = v2alpha1.OperationPhaseFailed
	now := metav1.Now()
	op.Status.CompletionTime = &now
	workflows := op.Status.Workflows
	if len(workflows) > 0 {
		ws := &workflows[0]
		ws.Terminated = true
		ws.Suspend = false
		terminateActiveSteps(ws)
	}
	if err := wo.cli.Status().Update(ctx, op); err != nil {
		return err
	}
	return writeOutputF(wo.outputWriter, "Successfully terminate operation: %s\n", op.Name)
}

// terminateActiveSteps flips any step or sub-step still Running/Suspending
// to Failed, mirroring the embedded engine's own TerminateWorkflow -- without
// this, a terminated Operation could keep steps reading as still in
// progress even though the whole run is now terminal.
func terminateActiveSteps(ws *v2alpha1.OperationWorkflowStatus) {
	for i := range ws.Steps {
		step := &ws.Steps[i]
		if step.Phase == workflowv1alpha1.WorkflowStepPhaseRunning || step.Phase == workflowv1alpha1.WorkflowStepPhaseSuspending {
			step.Phase = workflowv1alpha1.WorkflowStepPhaseFailed
			step.Reason = wfTypes.StatusReasonTerminate
		}
		for j := range step.SubStepsStatus {
			sub := &step.SubStepsStatus[j]
			if sub.Phase == workflowv1alpha1.WorkflowStepPhaseRunning || sub.Phase == workflowv1alpha1.WorkflowStepPhaseSuspending {
				sub.Phase = workflowv1alpha1.WorkflowStepPhaseFailed
				sub.Reason = wfTypes.StatusReasonTerminate
			}
		}
	}
}

// snapshotAttempts preserves affected steps' Attempts -- already recorded
// by the controller the moment each one completed (see recordStepCompletion
// in pkg/controller/.../operation/generator.go, called every reconcile, not
// just at restart) -- into ws.StepAttempts (pending re-attachment once the
// step's entry reappears -- see that field's doc), before
// restartFrom/restartFromStep removes the live entry. affected lists the
// step (and sub-step) status entries being reset -- for a whole-workflow
// restart that's every entry in ws.Steps; for a --step restart it's only
// the target step and its dependency set.
//
// Sub-steps aren't wrapped (no nested Attempts field of their own -- see
// OperationWorkflowStepStatus's doc), so recordStepCompletion never runs
// for them; their attempt history is still computed here, from their
// current status directly, the way top-level steps' used to be before the
// controller took over logging those.
func snapshotAttempts(ws *v2alpha1.OperationWorkflowStatus, affected []v2alpha1.OperationWorkflowStepStatus) {
	if ws.StepAttempts == nil {
		ws.StepAttempts = map[string][]v2alpha1.OperationStepAttempt{}
	}
	for _, s := range affected {
		if len(s.Attempts) > 0 {
			ws.StepAttempts[s.Name] = s.Attempts
		}
		for _, sub := range s.SubStepsStatus {
			// A sub-step still Running/Suspending when a restart snapshots
			// it hasn't actually completed -- recording it here would log a
			// non-terminal snapshot as though it were a finished attempt.
			if !wfTypes.IsStepFinish(sub.Phase, sub.Reason) {
				continue
			}
			recordAttempt(ws, sub.Name, sub)
		}
	}
}

func recordAttempt(ws *v2alpha1.OperationWorkflowStatus, name string, s workflowv1alpha1.StepStatus) {
	ws.StepAttempts[name] = append(ws.StepAttempts[name], v2alpha1.OperationStepAttempt{
		AttemptNumber: int64(len(ws.StepAttempts[name]) + 1),
		Phase:         s.Phase,
		Message:       s.Message,
		Reason:        s.Reason,
		StartTime:     s.FirstExecuteTime,
	})
}

// cleanOperationStatusFromStep resets stepName (and, in Operations' default
// sequential mode, every step positioned after it) so it re-executes.
// Mirrors github.com/kubevela/workflow/pkg/utils.CleanStatusFromStep, minus
// the "can not restart from a non-failed step" precondition. Returns the
// reset step status, the affected status entries (so the caller can
// snapshot them before they're gone), and the dependency set by name (so
// the caller can clear the same steps' recorded outputs from the
// context-backend ConfigMap).
func cleanOperationStatusFromStep(steps []oamv1alpha1.WorkflowStep, stepStatus []v2alpha1.OperationWorkflowStepStatus, mode oamv1alpha1.WorkflowExecuteMode, stepName string) ([]v2alpha1.OperationWorkflowStepStatus, []v2alpha1.OperationWorkflowStepStatus, []string, error) {
	found := false
	var affected []v2alpha1.OperationWorkflowStepStatus
	dependency := make([]string, 0)

	for i, step := range stepStatus {
		if step.Name == stepName {
			dependency = operationStepDependency(steps, stepName, mode.Steps == workflowv1alpha1.WorkflowModeDAG)
			affected = selectStepStatus(dependency, stepStatus, stepName, false)
			stepStatus = deleteOperationStepStatus(dependency, stepStatus, stepName, false)
			found = true
			break
		}
		for _, sub := range step.SubStepsStatus {
			if sub.Name == stepName {
				subDependency := operationStepDependency(steps, stepName, mode.SubSteps == workflowv1alpha1.WorkflowModeDAG)
				affected = []v2alpha1.OperationWorkflowStepStatus{{WorkflowStepStatus: workflowv1alpha1.WorkflowStepStatus{StepStatus: sub}}}
				for _, s := range step.SubStepsStatus {
					if slices.Contains(subDependency, s.Name) {
						affected = append(affected, v2alpha1.OperationWorkflowStepStatus{WorkflowStepStatus: workflowv1alpha1.WorkflowStepStatus{StepStatus: s}})
					}
				}
				stepStatus[i].SubStepsStatus = deleteOperationSubStepStatus(subDependency, step.SubStepsStatus, stepName)
				stepStatus[i].Phase = workflowv1alpha1.WorkflowStepPhaseRunning
				stepStatus[i].Reason = ""
				stepDependency := operationStepDependency(steps, step.Name, mode.Steps == workflowv1alpha1.WorkflowModeDAG)
				affected = append(affected, selectStepStatus(stepDependency, stepStatus, stepName, true)...)
				stepStatus = deleteOperationStepStatus(stepDependency, stepStatus, stepName, true)
				dependency = mergeUniqueStrings(subDependency, stepDependency)
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return nil, nil, nil, fmt.Errorf("step %s not found", stepName)
	}
	return stepStatus, affected, dependency, nil
}

// selectStepStatus returns the status entries that deleteOperationStepStatus
// would remove -- i.e. the ones being reset -- so the caller can snapshot
// them first.
func selectStepStatus(dependency []string, steps []v2alpha1.OperationWorkflowStepStatus, stepName string, group bool) []v2alpha1.OperationWorkflowStepStatus {
	var selected []v2alpha1.OperationWorkflowStepStatus
	for _, step := range steps {
		if group && slices.Contains(dependency, step.Name) {
			selected = append(selected, step)
			continue
		}
		if !group && (slices.Contains(dependency, step.Name) || step.Name == stepName) {
			selected = append(selected, step)
		}
	}
	return selected
}

func deleteOperationStepStatus(dependency []string, steps []v2alpha1.OperationWorkflowStepStatus, stepName string, group bool) []v2alpha1.OperationWorkflowStepStatus {
	status := make([]v2alpha1.OperationWorkflowStepStatus, 0, len(steps))
	for _, step := range steps {
		if group && !slices.Contains(dependency, step.Name) {
			status = append(status, step)
			continue
		}
		if !group && !slices.Contains(dependency, step.Name) && step.Name != stepName {
			status = append(status, step)
		}
	}
	return status
}

func deleteOperationSubStepStatus(dependency []string, subSteps []workflowv1alpha1.StepStatus, stepName string) []workflowv1alpha1.StepStatus {
	status := make([]workflowv1alpha1.StepStatus, 0, len(subSteps))
	for _, step := range subSteps {
		if !slices.Contains(dependency, step.Name) && step.Name != stepName {
			status = append(status, step)
		}
	}
	return status
}

// operationStepDependency mirrors the unexported getStepDependency helper in
// github.com/kubevela/workflow/pkg/utils -- reimplemented here because that
// helper isn't exported and the exported free functions around it (
// RestartWorkflow, RestartFromStep) operate on a *WorkflowRun, not an
// Operation.
func operationStepDependency(steps []oamv1alpha1.WorkflowStep, stepName string, dag bool) []string {
	if !dag {
		dependency := make([]string, 0)
		for i, step := range steps {
			if step.Name == stepName {
				for index := i + 1; index < len(steps); index++ {
					dependency = append(dependency, steps[index].Name)
				}
				return dependency
			}
			for j, sub := range step.SubSteps {
				if sub.Name == stepName {
					for index := j + 1; index < len(step.SubSteps); index++ {
						dependency = append(dependency, step.SubSteps[index].Name)
					}
					return dependency
				}
			}
		}
		return dependency
	}

	dependsOn := make(map[string][]string)
	stepOutputs := make(map[string]string)
	for _, step := range steps {
		for _, output := range step.Outputs {
			stepOutputs[output.Name] = step.Name
		}
		dependsOn[step.Name] = step.DependsOn
		for _, sub := range step.SubSteps {
			for _, output := range sub.Outputs {
				stepOutputs[output.Name] = sub.Name
			}
			dependsOn[sub.Name] = sub.DependsOn
		}
	}
	for _, step := range steps {
		for _, input := range step.Inputs {
			if name, ok := stepOutputs[input.From]; ok && !slices.Contains(dependsOn[step.Name], name) {
				dependsOn[step.Name] = append(dependsOn[step.Name], name)
			}
		}
		for _, sub := range step.SubSteps {
			for _, input := range sub.Inputs {
				if name, ok := stepOutputs[input.From]; ok && !slices.Contains(dependsOn[sub.Name], name) {
					dependsOn[sub.Name] = append(dependsOn[sub.Name], name)
				}
			}
		}
	}
	return findStepDependents(stepName, dependsOn)
}

func findStepDependents(stepName string, dependsOn map[string][]string) []string {
	dependency := make([]string, 0)
	for step, deps := range dependsOn {
		for _, dep := range deps {
			if dep == stepName {
				dependency = append(dependency, step)
				dependency = append(dependency, findStepDependents(step, dependsOn)...)
			}
		}
	}
	return dependency
}

func mergeUniqueStrings(a, b []string) []string {
	for _, item := range b {
		if !slices.Contains(a, item) {
			a = append(a, item)
		}
	}
	return a
}

// clearOperationContextVars strips the recorded outputs of stepName and its
// dependency set from the context-backend ConfigMap's CUE vars document.
// Mirrors the unexported clearContextVars helper in
// github.com/kubevela/workflow/pkg/utils, for the same reason
// operationStepDependency does.
// nolint:staticcheck
func clearOperationContextVars(steps []oamv1alpha1.WorkflowStep, varsCUE string, stepName string, dependency []string) (string, error) {
	outputs := make([]string, 0)
	for _, step := range steps {
		if step.Name == stepName || slices.Contains(dependency, step.Name) {
			for _, output := range step.Outputs {
				outputs = append(outputs, output.Name)
			}
		}
		for _, sub := range step.SubSteps {
			if sub.Name == stepName || slices.Contains(dependency, sub.Name) {
				for _, output := range sub.Outputs {
					outputs = append(outputs, output.Name)
				}
			}
		}
	}

	v := cuecontext.New().CompileString(varsCUE)
	node := v.Syntax(cue.ResolveReferences(true))
	x, ok := node.(*ast.StructLit)
	if !ok {
		return "", fmt.Errorf("value is not a struct lit")
	}
	element := make([]ast.Decl, 0)
	for i := range x.Elts {
		if field, ok := x.Elts[i].(*ast.Field); ok {
			label := strings.Trim(sets.LabelStr(field.Label), `"`)
			if !slices.Contains(outputs, label) {
				element = append(element, field)
			}
		}
	}
	x.Elts = element
	b, err := format.Node(x)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
