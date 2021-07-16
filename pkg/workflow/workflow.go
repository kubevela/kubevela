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
	"encoding/json"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/apply"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

type workflow struct {
	app        *oamcore.Application
	cli        client.Client
	applicator *apply.APIApplicator
}

// NewWorkflow returns a Workflow implementation.
func NewWorkflow(app *oamcore.Application, cli client.Client) Workflow {
	return &workflow{
		app: app,
		cli: cli,
	}
}

func (w *workflow) ExecuteSteps(ctx context.Context, rev string, taskRunners []wfTypes.TaskRunner) (bool, error) {
	if w.app.Spec.Workflow == nil {
		return true, nil
	}

	steps := w.app.Spec.Workflow.Steps
	if len(steps) == 0 {
		return true, nil
	}

	if w.app.Status.Workflow == nil || w.app.Status.Workflow.AppRevision != rev {
		w.app.Status.Workflow = &common.WorkflowStatus{
			AppRevision: rev,
			Steps:       []common.WorkflowStepStatus{},
		}
	}

	wfStatus := w.app.Status.Workflow

	if len(taskRunners) >= wfStatus.StepIndex || wfStatus.Terminated {
		return true, nil
	}

	//
	if wfStatus.Suspend {
		return true, nil
	}

	w.app.Status.Phase = common.ApplicationRunningWorkflow
	var (
		wfCtx wfContext.Context
		err   error
	)

	if wfStatus.ContextBackend != nil {
		wfCtx, err = wfContext.LoadContext(w.cli, w.app.Namespace, rev)
	} else {
		wfCtx, err = wfContext.NewContext(w.cli, w.app.Namespace, rev)
		wfStatus.ContextBackend = wfCtx.StoreRef()
	}

	if err != nil {
		return false, err
	}

	for _, run := range taskRunners[wfStatus.StepIndex:] {
		status, action, err := run(wfCtx)
		if err != nil {
			return false, err
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

		wfStatus.Terminated = action.Terminated
		wfStatus.Suspend = action.Suspend

		if wfStatus.Terminated || wfStatus.Suspend {
			return true, nil
		}

		if err := wfCtx.Commit(); err != nil {
			return false, errors.WithMessage(err, "commit context")
		}
		wfStatus.StepIndex += 1
	}

	return true, nil // all steps done
}

func (w *workflow) applyWorkflowStep(ctx context.Context, obj *unstructured.Unstructured, wctx *types.WorkflowContext) error {
	if err := addWorkflowContextToAnnotation(obj, wctx); err != nil {
		return err
	}

	return w.applicator.Apply(ctx, obj)
}

func addWorkflowContextToAnnotation(obj *unstructured.Unstructured, wc *types.WorkflowContext) error {
	b, err := json.Marshal(wc)
	if err != nil {
		return err
	}
	m := map[string]string{
		oam.AnnotationWorkflowContext: string(b),
	}
	obj.SetAnnotations(oamutil.MergeMapOverrideWithDst(m, obj.GetAnnotations()))
	return nil
}

const (
	// CondTypeWorkflowFinish is the type of the Condition indicating workflow progress
	CondTypeWorkflowFinish = "workflow-progress"

	// CondReasonSucceeded is the reason of the workflow progress condition which is succeeded
	CondReasonSucceeded = "Succeeded"
	// CondReasonStopped is the reason of the workflow progress condition which is stopped
	CondReasonStopped = "Stopped"
	// CondReasonFailed is the reason of the workflow progress condition which is failed
	CondReasonFailed = "Failed"

	// CondStatusTrue is the status of the workflow progress condition which is True
	CondStatusTrue = "True"
)

func (w *workflow) syncWorkflowStatus(step oamcore.WorkflowStep, obj *unstructured.Unstructured) (*common.WorkflowStepStatus, error) {
	status := &common.WorkflowStepStatus{
		Name: step.Name,
		Type: step.Type,
		ResourceRef: runtimev1alpha1.TypedReference{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			Name:       obj.GetName(),
			UID:        obj.GetUID(),
		},
	}

	cond, found, err := utils.GetUnstructuredObjectStatusCondition(obj, CondTypeWorkflowFinish)
	if err != nil {
		return nil, err
	}

	if !found || cond.Status != CondStatusTrue {
		status.Phase = common.WorkflowStepPhaseRunning
		return status, nil
	}

	switch cond.Reason {
	case CondReasonSucceeded:
		observedG, err := parseGeneration(cond.Message)
		if err != nil {
			return nil, err
		}
		if observedG != obj.GetGeneration() {
			status.Phase = common.WorkflowStepPhaseRunning
		} else {
			status.Phase = common.WorkflowStepPhaseSucceeded
		}
	case CondReasonFailed:
		status.Phase = common.WorkflowStepPhaseFailed
	case CondReasonStopped:
		status.Phase = common.WorkflowStepPhaseStopped
	default:
		status.Phase = common.WorkflowStepPhaseRunning
	}
	return status, nil
}

func parseGeneration(message string) (int64, error) {
	m := &SucceededMessage{}
	err := json.Unmarshal([]byte(message), m)
	return m.ObservedGeneration, err
}
