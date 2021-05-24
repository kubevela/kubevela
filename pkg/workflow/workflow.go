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
	"fmt"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

type workflow struct {
	app        *oamcore.Application
	applicator apply.Applicator
}

// NewWorkflow returns a Workflow implementation.
func NewWorkflow(app *oamcore.Application, applicator apply.Applicator) Workflow {
	return &workflow{
		app:        app,
		applicator: applicator,
	}
}

func (w *workflow) ExecuteSteps(ctx context.Context, rev string, objects []*unstructured.Unstructured) (bool, error) {
	steps := w.app.Spec.Workflow

	if len(steps) == 0 {
		return true, nil
	}

	w.app.Status.Phase = common.ApplicationRunningWorkflow

	w.app.Status.Workflow = []common.WorkflowStepStatus{}
	for i, step := range steps {
		obj := objects[i].DeepCopy()
		obj.SetName(step.Name)
		obj.SetNamespace(w.app.Namespace)
		obj.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion: oamcore.SchemeGroupVersion.String(),
			Kind:       oamcore.ApplicationKind,
			Name:       w.app.Name,
			UID:        w.app.UID,
			Controller: pointer.BoolPtr(false),
		}})
		err := w.applyWorkflowStep(ctx, obj, &types.WorkflowContext{
			AppName:       w.app.Name,
			AppRevision:   rev,
			WorkflowIndex: i,
		})
		if err != nil {
			return false, err
		}

		status, err := w.syncWorkflowStatus(step, obj)
		if err != nil {
			return false, err
		}

		w.app.Status.Workflow = append(w.app.Status.Workflow, *status)
		switch status.Phase {
		case common.WorkflowStepPhaseSucceeded: // This one is done. Continue
		case common.WorkflowStepPhaseRunning: // Need to retry shortly.
			return false, nil
		default:
			return true, nil
		}
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

	cond, found, err := utils.GetUnstructuredObjectStatusCondition(obj, "workflow-finish")
	if err != nil {
		return nil, err
	}

	if !found || cond.Status != "True" {
		status.Phase = common.WorkflowStepPhaseRunning
		return status, nil
	}

	switch cond.Reason {
	case "Succeeded":
		observedG, err := parseGeneration(cond.Message)
		fmt.Println("haha4", observedG, err)
		if err != nil {
			return nil, err
		}
		if observedG != obj.GetGeneration() {
			status.Phase = common.WorkflowStepPhaseRunning
		} else {
			status.Phase = common.WorkflowStepPhaseSucceeded
		}
	case "Failed":
		status.Phase = common.WorkflowStepPhaseFailed
	case "Stopped":
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
