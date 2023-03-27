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

package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestWorkflowStepGenerator(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(common2.Scheme).WithObjects(&workflowv1alpha1.Workflow{
		ObjectMeta: v1.ObjectMeta{
			Name:      "ref-wf",
			Namespace: "test",
		},
		WorkflowSpec: workflowv1alpha1.WorkflowSpec{
			Steps: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name: "manual-approve",
					Type: "suspend",
				},
			}, {
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name: "deploy",
					Type: "deploy",
				},
			}},
		},
	}).Build()
	testCases := map[string]struct {
		input    []workflowv1alpha1.WorkflowStep
		app      *v1beta1.Application
		output   []workflowv1alpha1.WorkflowStep
		hasError bool
	}{
		"apply-component-with-existing-steps": {
			input: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "example-comp-1",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"example-comp-1"}`)},
				},
			}},
			app: &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name: "example-comp-1",
					}, {
						Name: "example-comp-2",
					}},
				},
			},
			output: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "example-comp-1",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"example-comp-1"}`)},
				},
			}},
		},
		"apply-component-with-no-steps": {
			input: []workflowv1alpha1.WorkflowStep{},
			app: &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name: "example-comp-1",
					}, {
						Name: "example-comp-2",
					}},
				},
			},
			output: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "example-comp-1",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"example-comp-1"}`)},
				},
			}, {
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "example-comp-2",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"example-comp-2"}`)},
				},
			}},
		},
		"env-binding-bad": {
			input: []workflowv1alpha1.WorkflowStep{},
			app: &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name: "example-comp-1",
					}},
					Policies: []v1beta1.AppPolicy{{
						Name:       "example-policy",
						Type:       v1alpha1.EnvBindingPolicyType,
						Properties: &runtime.RawExtension{Raw: []byte(`bad value`)},
					}},
				},
			},
			hasError: true,
		},
		"env-binding-correct": {
			input: []workflowv1alpha1.WorkflowStep{},
			app: &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name: "example-comp-1",
					}},
					Policies: []v1beta1.AppPolicy{{
						Name:       "example-policy",
						Type:       v1alpha1.EnvBindingPolicyType,
						Properties: &runtime.RawExtension{Raw: []byte(`{"envs":[{"name":"env-1"},{"name":"env-2"}]}`)},
					}},
				},
			},
			output: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "deploy-example-policy-env-1",
					Type:       "deploy2env",
					Properties: &runtime.RawExtension{Raw: []byte(`{"env":"env-1","policy":"example-policy"}`)},
				},
			}, {
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "deploy-example-policy-env-2",
					Type:       "deploy2env",
					Properties: &runtime.RawExtension{Raw: []byte(`{"env":"env-2","policy":"example-policy"}`)},
				},
			}},
		},
		"deploy-workflow": {
			input: []workflowv1alpha1.WorkflowStep{},
			app: &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name: "example-comp-1",
					}},
					Policies: []v1beta1.AppPolicy{{
						Name: "example-topology-policy-1",
						Type: v1alpha1.TopologyPolicyType,
					}, {
						Name: "example-topology-policy-2",
						Type: v1alpha1.TopologyPolicyType,
					}, {
						Name: "example-override-policy-1",
						Type: v1alpha1.OverridePolicyType,
					}, {
						Name: "example-override-policy-2",
						Type: v1alpha1.OverridePolicyType,
					}},
				},
			},
			output: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "deploy-example-topology-policy-1",
					Type:       "deploy",
					Properties: &runtime.RawExtension{Raw: []byte(`{"policies":["example-override-policy-1","example-override-policy-2","example-topology-policy-1"]}`)},
				},
			}, {
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "deploy-example-topology-policy-2",
					Type:       "deploy",
					Properties: &runtime.RawExtension{Raw: []byte(`{"policies":["example-override-policy-1","example-override-policy-2","example-topology-policy-2"]}`)},
				},
			}},
		},
		"deploy-with-ref-without-po-workflow": {
			input: []workflowv1alpha1.WorkflowStep{},
			app: &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name: "example-comp",
						Type: "ref-objects",
					}},
				},
			},
			output: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "deploy",
					Type:       "deploy",
					Properties: &runtime.RawExtension{Raw: []byte(`{"policies":[]}`)},
				},
			}},
		},
		"ref-workflow": {
			input: nil,
			app: &v1beta1.Application{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
				},
				Spec: v1beta1.ApplicationSpec{
					Workflow: &v1beta1.Workflow{
						Ref: "ref-wf",
					},
				},
			},
			output: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name: "manual-approve",
					Type: "suspend",
				},
			}, {
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name: "deploy",
					Type: "deploy",
				},
			}},
		},
		"ref-workflow-conflict": {
			input: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name: "deploy",
					Type: "deploy",
				},
			}},
			app: &v1beta1.Application{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
				},
				Spec: v1beta1.ApplicationSpec{
					Workflow: &v1beta1.Workflow{
						Ref: "ref-wf",
						Steps: []workflowv1alpha1.WorkflowStep{{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "deploy",
								Type: "deploy",
							},
						}},
					},
				},
			},
			hasError: true,
		},
		"ref-workflow-not-found": {
			input: nil,
			app: &v1beta1.Application{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "test",
				},
				Spec: v1beta1.ApplicationSpec{
					Workflow: &v1beta1.Workflow{
						Ref: "ref-wf-404",
					},
				},
			},
			hasError: true,
		},
	}
	generator := NewChainWorkflowStepGenerator(
		&RefWorkflowStepGenerator{Context: context.Background(), Client: cli},
		&DeployWorkflowStepGenerator{},
		&Deploy2EnvWorkflowStepGenerator{},
		&ApplyComponentWorkflowStepGenerator{},
	)
	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			output, err := generator.Generate(testCase.app, testCase.input)
			if testCase.hasError {
				r.Error(err)
			} else {
				r.NoError(err)
				r.Equal(testCase.output, output)
			}
		})
	}
}
