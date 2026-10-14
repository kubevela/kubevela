/*
Copyright 2026 The KubeVela Authors.

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
	"testing"

	"github.com/stretchr/testify/require"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

func TestConvertWorkflowStatusCarriesStepDependencies(t *testing.T) {
	run := workflowv1alpha1.WorkflowRunStatus{Dependencies: []workflowv1alpha1.StepDependency{
		{Step: "notify", DependsOn: "cfg", Source: workflowv1alpha1.StepDependencySourceDependsOn},
		{Step: "notify", DependsOn: "build", Source: workflowv1alpha1.StepDependencySourceExpression},
	}}
	got := ConvertWorkflowStatus(run, "app-v1")
	require.Equal(t, []common.WorkflowStepDependency{
		{Step: "notify", DependsOn: "cfg", Source: "dependsOn"},
		{Step: "notify", DependsOn: "build", Source: "expression"},
	}, got.Dependencies)
	require.Equal(t, run.Dependencies, StepDependencies(got.Dependencies), "and back, for the workflow engine")
}

func TestWithStepReadsMarksAndAddsWhatComponentReadsImply(t *testing.T) {
	dep := func(step, on, source string) common.WorkflowStepDependency {
		return common.WorkflowStepDependency{Step: step, DependsOn: on, Source: source}
	}
	for name, tc := range map[string]struct {
		deps  []common.WorkflowStepDependency
		reads map[string][]string
		want  []common.WorkflowStepDependency
	}{
		"an edge KubeVela added for a read is an expression's, not the spec's": {
			deps:  []common.WorkflowStepDependency{dep("api", "db", "dependsOn")},
			reads: map[string][]string{"api": {"db"}},
			want:  []common.WorkflowStepDependency{dep("api", "db", "expression")},
		},
		"a read the order answers without an edge is still recorded": {
			deps:  nil,
			reads: map[string][]string{"api": {"db"}},
			want:  []common.WorkflowStepDependency{dep("api", "db", "expression")},
		},
		"a written dependsOn stays the spec's": {
			deps:  []common.WorkflowStepDependency{dep("api", "cfg", "dependsOn")},
			reads: map[string][]string{"api": {"db"}},
			want:  []common.WorkflowStepDependency{dep("api", "cfg", "dependsOn"), dep("api", "db", "expression")},
		},
		"a step expression's read, already marked, is left as it is": {
			deps:  []common.WorkflowStepDependency{dep("notify", "build", "expression")},
			reads: map[string][]string{"api": {"db"}},
			want:  []common.WorkflowStepDependency{dep("api", "db", "expression"), dep("notify", "build", "expression")},
		},
		"a read recorded both ways appears once": {
			deps:  []common.WorkflowStepDependency{dep("api", "db", "expression")},
			reads: map[string][]string{"api": {"db"}},
			want:  []common.WorkflowStepDependency{dep("api", "db", "expression")},
		},
		"sorted by step, each step's entries kept in order": {
			deps:  []common.WorkflowStepDependency{dep("web", "cfg", "dependsOn"), dep("api", "cfg", "dependsOn")},
			reads: map[string][]string{"web": {"db"}, "api": {"cache"}},
			want: []common.WorkflowStepDependency{dep("api", "cfg", "dependsOn"), dep("api", "cache", "expression"),
				dep("web", "cfg", "dependsOn"), dep("web", "db", "expression")},
		},
		"nothing read: the engine's list as it is": {
			deps: []common.WorkflowStepDependency{dep("api", "cfg", "dependsOn")},
			want: []common.WorkflowStepDependency{dep("api", "cfg", "dependsOn")},
		},
	} {
		t.Run(name, func(t *testing.T) {
			in := append([]common.WorkflowStepDependency(nil), tc.deps...)
			require.Equal(t, tc.want, WithStepReads(tc.deps, tc.reads))
			require.Equal(t, in, tc.deps, "the list passed in is not changed")
		})
	}
}
