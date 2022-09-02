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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestLoadExternalPoliciesForWorkflow(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(&v1alpha1.Policy{
		ObjectMeta: v1.ObjectMeta{
			Name:      "ex",
			Namespace: "demo",
		},
		Type: "ex-type",
	}).Build()
	policies, err := LoadExternalPoliciesForWorkflow(context.Background(), cli, "demo", []workflowv1alpha1.WorkflowStep{{
		WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
			Name:       "deploy",
			Type:       DeployWorkflowStep,
			Properties: &runtime.RawExtension{Raw: []byte(`{"auto":false,"policies":["ex","internal"],"parallelism":10}`)},
		},
	}}, []v1beta1.AppPolicy{{
		Name: "internal",
		Type: "internal",
	}})
	r.NoError(err)
	r.Equal(2, len(policies))
	r.Equal("ex", policies[1].Name)
	r.Equal("ex-type", policies[1].Type)

	// Test policy not found
	_, err = LoadExternalPoliciesForWorkflow(context.Background(), cli, "demo", []workflowv1alpha1.WorkflowStep{{
		WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
			Name:       "deploy",
			Type:       DeployWorkflowStep,
			Properties: &runtime.RawExtension{Raw: []byte(`{"policies":["ex","non"]}`)},
		},
	}}, []v1beta1.AppPolicy{})
	r.NotNil(err)
	r.Contains(err.Error(), "external policy non not found")

	// Test invalid policy
	_, err = LoadExternalPoliciesForWorkflow(context.Background(), cli, "demo", []workflowv1alpha1.WorkflowStep{{
		WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
			Name:       "deploy",
			Type:       DeployWorkflowStep,
			Properties: &runtime.RawExtension{Raw: []byte(`{"policies":["ex","non"],"unknown-field":"value"}`)},
		},
	}}, []v1beta1.AppPolicy{})
	r.NotNil(err)
	r.Contains(err.Error(), "invalid WorkflowStep deploy")
}
