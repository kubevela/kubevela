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

package cli

import (
	"testing"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestGetAppHealth(t *testing.T) {
	r := require.New(t)

	healthyService := common.ApplicationComponentStatus{
		Name:    "web",
		Healthy: true,
	}
	unhealthyService := common.ApplicationComponentStatus{
		Name:    "web",
		Healthy: false,
	}
	unhealthyTrait := common.ApplicationComponentStatus{
		Name:    "web",
		Healthy: true,
		Traits: []common.ApplicationTraitStatus{{
			Type:    "scaler",
			Healthy: false,
		}},
	}

	tests := []struct {
		name string
		app  *v1beta1.Application
		want bool
	}{
		{
			name: "running with healthy services",
			app: &v1beta1.Application{Status: common.AppStatus{
				Phase:    common.ApplicationRunning,
				Services: []common.ApplicationComponentStatus{healthyService},
			}},
			want: true,
		},
		{
			name: "running with unhealthy service",
			app: &v1beta1.Application{Status: common.AppStatus{
				Phase:    common.ApplicationRunning,
				Services: []common.ApplicationComponentStatus{unhealthyService},
			}},
			want: false,
		},
		{
			name: "running with unhealthy trait",
			app: &v1beta1.Application{Status: common.AppStatus{
				Phase:    common.ApplicationRunning,
				Services: []common.ApplicationComponentStatus{unhealthyTrait},
			}},
			want: false,
		},
		{
			name: "workflowFailed with empty services must not be healthy",
			app: &v1beta1.Application{Status: common.AppStatus{
				Phase:    common.ApplicationWorkflowFailed,
				Services: nil,
				Workflow: &common.WorkflowStatus{
					Finished:   true,
					Terminated: true,
					Steps: []workflowv1alpha1.WorkflowStepStatus{{
						StepStatus: workflowv1alpha1.StepStatus{
							Name:  "web",
							Phase: workflowv1alpha1.WorkflowStepPhaseFailed,
						},
					}},
				},
			}},
			want: false,
		},
		{
			name: "workflowTerminated with empty services must not be healthy",
			app: &v1beta1.Application{Status: common.AppStatus{
				Phase: common.ApplicationWorkflowTerminated,
			}},
			want: false,
		},
		{
			name: "unhealthy phase must not be healthy",
			app: &v1beta1.Application{Status: common.AppStatus{
				Phase:    common.ApplicationUnhealthy,
				Services: []common.ApplicationComponentStatus{unhealthyService},
			}},
			want: false,
		},
		{
			name: "runningWorkflow with failed step and empty services must not be healthy",
			app: &v1beta1.Application{Status: common.AppStatus{
				Phase: common.ApplicationRunningWorkflow,
				Workflow: &common.WorkflowStatus{
					Steps: []workflowv1alpha1.WorkflowStepStatus{{
						StepStatus: workflowv1alpha1.StepStatus{
							Name:  "web",
							Phase: workflowv1alpha1.WorkflowStepPhaseFailed,
						},
					}},
				},
			}},
			want: false,
		},
		{
			name: "runningWorkflow empty services intermediate is not healthy",
			app: &v1beta1.Application{Status: common.AppStatus{
				Phase:    common.ApplicationRunningWorkflow,
				Services: nil,
			}},
			want: false,
		},
		{
			name: "runningWorkflow wait healthy with unhealthy service",
			app: &v1beta1.Application{Status: common.AppStatus{
				Phase:    common.ApplicationRunningWorkflow,
				Services: []common.ApplicationComponentStatus{unhealthyService},
			}},
			want: false,
		},
		{
			name: "running with empty services is not healthy",
			app: &v1beta1.Application{Status: common.AppStatus{
				Phase:    common.ApplicationRunning,
				Services: nil,
			}},
			want: false,
		},
		{
			name: "workflowSuspending with healthy services stays healthy",
			app: &v1beta1.Application{Status: common.AppStatus{
				Phase:    common.ApplicationWorkflowSuspending,
				Services: []common.ApplicationComponentStatus{healthyService},
			}},
			want: true,
		},
		{
			name: "failed substep in step-group must not be healthy even if services healthy",
			app: &v1beta1.Application{Status: common.AppStatus{
				Phase:    common.ApplicationRunningWorkflow,
				Services: []common.ApplicationComponentStatus{healthyService},
				Workflow: &common.WorkflowStatus{
					Steps: []workflowv1alpha1.WorkflowStepStatus{{
						StepStatus: workflowv1alpha1.StepStatus{
							Name:  "group",
							Type:  "step-group",
							Phase: workflowv1alpha1.WorkflowStepPhaseRunning,
						},
						SubStepsStatus: []workflowv1alpha1.StepStatus{{
							Name:  "child",
							Phase: workflowv1alpha1.WorkflowStepPhaseFailed,
						}},
					}},
				},
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r.Equal(tt.want, getAppHealth(tt.app), tt.name)
		})
	}
}
