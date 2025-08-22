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

	"github.com/stretchr/testify/assert"
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

func TestApplyComponentWorkflowStepGeneratorWithDependsOn(t *testing.T) {
	r := require.New(t)

	testCases := map[string]struct {
		input    []workflowv1alpha1.WorkflowStep
		app      *v1beta1.Application
		output   []workflowv1alpha1.WorkflowStep
		hasError bool
	}{
		"component-with-single-dependency": {
			input: []workflowv1alpha1.WorkflowStep{},
			app: &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name: "database",
						Type: "webservice",
					}, {
						Name:      "backend",
						Type:      "webservice",
						DependsOn: []string{"database"},
					}},
				},
			},
			output: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "database",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"database"}`)},
				},
			}, {
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "backend",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"backend"}`)},
					DependsOn:  []string{"database"},
				},
			}},
		},
		"component-with-multiple-dependencies": {
			input: []workflowv1alpha1.WorkflowStep{},
			app: &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name: "database",
						Type: "webservice",
					}, {
						Name: "cache",
						Type: "webservice",
					}, {
						Name:      "backend",
						Type:      "webservice",
						DependsOn: []string{"database", "cache"},
					}},
				},
			},
			output: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "database",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"database"}`)},
				},
			}, {
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "cache",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"cache"}`)},
				},
			}, {
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "backend",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"backend"}`)},
					DependsOn:  []string{"database", "cache"},
				},
			}},
		},
		"component-with-chained-dependencies": {
			input: []workflowv1alpha1.WorkflowStep{},
			app: &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name: "database",
						Type: "webservice",
					}, {
						Name:      "backend",
						Type:      "webservice",
						DependsOn: []string{"database"},
					}, {
						Name:      "frontend",
						Type:      "webservice",
						DependsOn: []string{"backend"},
					}},
				},
			},
			output: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "database",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"database"}`)},
				},
			}, {
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "backend",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"backend"}`)},
					DependsOn:  []string{"database"},
				},
			}, {
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "frontend",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"frontend"}`)},
					DependsOn:  []string{"backend"},
				},
			}},
		},
		"component-without-dependency": {
			input: []workflowv1alpha1.WorkflowStep{},
			app: &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name: "standalone",
						Type: "webservice",
					}},
				},
			},
			output: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "standalone",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"standalone"}`)},
				},
			}},
		},
		"mixed-components-some-with-dependencies": {
			input: []workflowv1alpha1.WorkflowStep{},
			app: &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name: "independent1",
						Type: "webservice",
					}, {
						Name: "database",
						Type: "webservice",
					}, {
						Name:      "dependent1",
						Type:      "webservice",
						DependsOn: []string{"database"},
					}, {
						Name: "independent2",
						Type: "webservice",
					}},
				},
			},
			output: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "independent1",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"independent1"}`)},
				},
			}, {
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "database",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"database"}`)},
				},
			}, {
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "dependent1",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"dependent1"}`)},
					DependsOn:  []string{"database"},
				},
			}, {
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "independent2",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"independent2"}`)},
				},
			}},
		},
	}

	generator := &ApplyComponentWorkflowStepGenerator{}

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

func TestComponentDependsOnFieldPreservation(t *testing.T) {
	r := require.New(t)

	// This test specifically verifies that the DependsOn field from components
	// is correctly carried forward to workflow steps, enabling execution gating
	app := &v1beta1.Application{
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Name: "database",
				Type: "webservice",
			}, {
				Name:      "api-server",
				Type:      "webservice",
				DependsOn: []string{"database"},
			}, {
				Name:      "worker",
				Type:      "webservice",
				DependsOn: []string{"database", "api-server"},
			}},
		},
	}

	generator := &ApplyComponentWorkflowStepGenerator{}
	output, err := generator.Generate(app, []workflowv1alpha1.WorkflowStep{})

	r.NoError(err)
	r.Len(output, 3)

	// Verify the database component has no dependencies
	r.Equal("database", output[0].Name)
	r.Nil(output[0].DependsOn)

	// Verify api-server has database dependency
	r.Equal("api-server", output[1].Name)
	r.Equal([]string{"database"}, output[1].DependsOn)

	// Verify worker has both dependencies
	r.Equal("worker", output[2].Name)
	r.Equal([]string{"database", "api-server"}, output[2].DependsOn)
}

func TestChainGeneratorWithComponentDependsOn(t *testing.T) {
	r := require.New(t)

	// Test that the chain generator preserves component dependencies
	cli := fake.NewClientBuilder().WithScheme(common2.Scheme).Build()

	app := &v1beta1.Application{
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Name: "database",
				Type: "webservice",
			}, {
				Name:      "backend",
				Type:      "webservice",
				DependsOn: []string{"database"},
			}},
		},
	}

	generator := NewChainWorkflowStepGenerator(
		&RefWorkflowStepGenerator{Context: context.Background(), Client: cli},
		&DeployWorkflowStepGenerator{},
		&Deploy2EnvWorkflowStepGenerator{},
		&ApplyComponentWorkflowStepGenerator{},
	)

	output, err := generator.Generate(app, []workflowv1alpha1.WorkflowStep{})
	r.NoError(err)
	r.Len(output, 2)

	// Find the backend step and verify it has the dependency
	var backendStep *workflowv1alpha1.WorkflowStep
	for i, step := range output {
		if step.Name == "backend" {
			backendStep = &output[i]
			break
		}
	}

	r.NotNil(backendStep, "Backend step should exist")
	r.Equal([]string{"database"}, backendStep.DependsOn, "Backend step should depend on database")
}

func TestIsBuiltinWorkflowStepType(t *testing.T) {
	assert.True(t, IsBuiltinWorkflowStepType("suspend"))
	assert.True(t, IsBuiltinWorkflowStepType("apply-component"))
	assert.True(t, IsBuiltinWorkflowStepType("step-group"))
	assert.True(t, IsBuiltinWorkflowStepType("builtin-apply-component"))
}
