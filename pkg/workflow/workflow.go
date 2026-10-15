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
	"fmt"
	"slices"
	"sort"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// IsFailedAfterRetry check if application is hang due to FailedAfterRetry
func IsFailedAfterRetry(app *oamcore.Application) bool {
	return app.Status.Workflow != nil && app.Status.Workflow.Message == wfTypes.MessageSuspendFailedAfterRetries
}

// ConvertWorkflowStatus convert workflow run status to workflow status
func ConvertWorkflowStatus(status workflowv1alpha1.WorkflowRunStatus, revision string) *common.WorkflowStatus {
	return &common.WorkflowStatus{
		AppRevision:    revision,
		Mode:           fmt.Sprintf("%s-%s", status.Mode.Steps, status.Mode.SubSteps),
		Phase:          status.Phase,
		Message:        status.Message,
		Suspend:        status.Suspend,
		SuspendState:   status.SuspendState,
		Terminated:     status.Terminated,
		Finished:       status.Finished,
		ContextBackend: status.ContextBackend,
		Steps:          status.Steps,
		Dependencies:   workflowStepDependencies(status.Dependencies),
		StartTime:      status.StartTime,
		EndTime:        status.EndTime,
	}
}

// workflowStepDependencies is the engine's record of what each step waits on, as
// the Application reports it.
func workflowStepDependencies(in []workflowv1alpha1.StepDependency) []common.WorkflowStepDependency {
	if in == nil {
		return nil
	}
	out := make([]common.WorkflowStepDependency, len(in))
	for i, d := range in {
		out[i] = common.WorkflowStepDependency{Step: d.Step, DependsOn: d.DependsOn, Source: string(d.Source)}
	}
	return out
}

// StepDependencies is an Application's record of what each step waits on, as the
// workflow engine keeps it.
func StepDependencies(in []common.WorkflowStepDependency) []workflowv1alpha1.StepDependency {
	if in == nil {
		return nil
	}
	out := make([]workflowv1alpha1.StepDependency, len(in))
	for i, d := range in {
		out[i] = workflowv1alpha1.StepDependency{Step: d.Step, DependsOn: d.DependsOn,
			Source: workflowv1alpha1.StepDependencySource(d.Source)}
	}
	return out
}

// WithStepReads adds KubeVela's component reads to deps, the workflow engine's
// record of what each step waits on. reads maps each step to the steps whose
// components the components it applies read (appfile.StepReadDependencies).
// An entry of deps that is one of those reads is marked as an expression
// dependency; a read missing from deps, because the order answered it without a
// dependsOn, is added as one. The result is sorted by step, each step's entries
// in their original order.
func WithStepReads(deps []common.WorkflowStepDependency, reads map[string][]string) []common.WorkflowStepDependency {
	if len(reads) == 0 {
		return deps
	}
	out := make([]common.WorkflowStepDependency, 0, len(deps))
	marked := map[[2]string]bool{}
	for _, d := range deps {
		if slices.Contains(reads[d.Step], d.DependsOn) {
			d.Source = string(workflowv1alpha1.StepDependencySourceExpression)
		}
		marked[[2]string{d.Step, d.DependsOn}] = true
		out = append(out, d)
	}
	steps := make([]string, 0, len(reads))
	for step := range reads {
		steps = append(steps, step)
	}
	sort.Strings(steps)
	for _, step := range steps {
		for _, on := range reads[step] {
			if !marked[[2]string{step, on}] {
				out = append(out, common.WorkflowStepDependency{Step: step, DependsOn: on,
					Source: string(workflowv1alpha1.StepDependencySourceExpression)})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Step < out[j].Step })
	return out
}
