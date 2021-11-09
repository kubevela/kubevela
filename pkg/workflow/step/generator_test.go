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
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestWorkflowStepGenerator(t *testing.T) {
	testCases := []struct {
		input    []v1beta1.WorkflowStep
		app      *v1beta1.Application
		output   []v1beta1.WorkflowStep
		hasError bool
	}{{
		input: []v1beta1.WorkflowStep{{
			Name:       "example-comp-1",
			Type:       "apply-component",
			Properties: &runtime.RawExtension{Raw: []byte(`{"component":"example-comp-1"}`)},
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
		output: []v1beta1.WorkflowStep{{
			Name:       "example-comp-1",
			Type:       "apply-component",
			Properties: &runtime.RawExtension{Raw: []byte(`{"component":"example-comp-1"}`)},
		}},
	}, {
		input: []v1beta1.WorkflowStep{},
		app: &v1beta1.Application{
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{
					Name: "example-comp-1",
				}, {
					Name: "example-comp-2",
				}},
			},
		},
		output: []v1beta1.WorkflowStep{{
			Name:       "example-comp-1",
			Type:       "apply-component",
			Properties: &runtime.RawExtension{Raw: []byte(`{"component":"example-comp-1"}`)},
		}, {
			Name:       "example-comp-2",
			Type:       "apply-component",
			Properties: &runtime.RawExtension{Raw: []byte(`{"component":"example-comp-2"}`)},
		}},
	}, {
		input: []v1beta1.WorkflowStep{},
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
	}, {
		input: []v1beta1.WorkflowStep{},
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
		output: []v1beta1.WorkflowStep{{
			Name:       "deploy-example-policy-env-1",
			Type:       "deploy2env",
			Properties: &runtime.RawExtension{Raw: []byte(`{"env":"env-1","policy":"example-policy"}`)},
		}, {
			Name:       "deploy-example-policy-env-2",
			Type:       "deploy2env",
			Properties: &runtime.RawExtension{Raw: []byte(`{"env":"env-2","policy":"example-policy"}`)},
		}},
	}}
	generator := NewChainWorkflowStepGenerator(
		&Deploy2EnvWorkflowStepGenerator{},
		&ApplyComponentWorkflowStepGenerator{},
	)
	r := require.New(t)
	for _, testCase := range testCases {
		output, err := generator.Generate(testCase.app, testCase.input)
		if testCase.hasError {
			r.Error(err)
			continue
		}
		r.NoError(err)
		r.Equal(testCase.output, output)
	}
}
