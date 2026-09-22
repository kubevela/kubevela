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
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

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
	policies, err := LoadExternalPoliciesForWorkflow(context.Background(), cli, "demo", []wfTypesv1alpha1.WorkflowStep{{
		WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
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
	_, err = LoadExternalPoliciesForWorkflow(context.Background(), cli, "demo", []wfTypesv1alpha1.WorkflowStep{{
		WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
			Name:       "deploy",
			Type:       DeployWorkflowStep,
			Properties: &runtime.RawExtension{Raw: []byte(`{"policies":["ex","non"]}`)},
		},
	}}, []v1beta1.AppPolicy{})
	r.NotNil(err)
	r.Contains(err.Error(), "external policy non not found")

	// Test invalid policy
	_, err = LoadExternalPoliciesForWorkflow(context.Background(), cli, "demo", []wfTypesv1alpha1.WorkflowStep{{
		WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
			Name:       "deploy",
			Type:       DeployWorkflowStep,
			Properties: &runtime.RawExtension{Raw: []byte(`{"policies":["ex","non"],"unknown-field":"value"}`)},
		},
	}}, []v1beta1.AppPolicy{})
	r.NotNil(err)
	r.Contains(err.Error(), "invalid WorkflowStep deploy")
}

// deployStepFieldNames erases DeployWorkflowStepSpec's types so an
// unsubstituted expression does not fail the decode, and it must therefore be
// kept in step with it. A field added to the spec and not here would silently
// start being rejected as unknown whenever an expression is present.
func TestDeployStepFieldNamesMatchSpec(t *testing.T) {
	jsonNames := func(v interface{}) []string {
		typ := reflect.TypeOf(v)
		var out []string
		for i := 0; i < typ.NumField(); i++ {
			tag := typ.Field(i).Tag.Get("json")
			if tag == "" || tag == "-" {
				continue
			}
			out = append(out, strings.Split(tag, ",")[0])
		}
		sort.Strings(out)
		return out
	}
	spec := jsonNames(DeployWorkflowStepSpec{})
	shadow := jsonNames(deployStepFieldNames{})
	if !reflect.DeepEqual(spec, shadow) {
		t.Fatalf("deployStepFieldNames is out of step with DeployWorkflowStepSpec:\n  spec:   %v\n  shadow: %v", spec, shadow)
	}
}

// A deploy step carrying an expression must still have its field names checked;
// only the types are unknowable before substitution.
func TestDeployStepDecodeToleratesExpressionsButNotTypos(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		wantErr string
	}{
		{"expression in a typed field", `{"policies":["p"],"parallelism":"$(source.n.count)"}`, ""},
		{"typo alongside an expression", `{"policys":["p"],"parallelism":"$(source.n.count)"}`, `unknown field "policys"`},
		{"typo with no expression", `{"policys":["p"]}`, `unknown field "policys"`},
		{"plain valid properties", `{"policies":["p"],"parallelism":4}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := []wfTypesv1alpha1.WorkflowStep{{
				WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
					Name: "d", Type: DeployWorkflowStep,
					Properties: &runtime.RawExtension{Raw: []byte(tc.raw)},
				},
			}}
			// The named policy is supplied inline, so no client lookup happens.
			inline := []v1beta1.AppPolicy{{Name: "p", Type: "topology"}}
			_, err := LoadExternalPoliciesForWorkflow(context.Background(), nil, "default", steps, inline)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected acceptance, got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected an error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

// A policy name that is still an expression names nothing yet. Strict decoding
// accepts it - it is a string, whatever it holds - so the loader looked it up
// literally and failed the whole appfile with "external policy
// $(source.cfg.name) not found", before the step that resolves it ever ran.
func TestLoadExternalPoliciesLeavesExpressionNamesToTheStep(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	steps := []wfTypesv1alpha1.WorkflowStep{{
		WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
			Name: "deploy", Type: DeployWorkflowStep,
			Properties: &runtime.RawExtension{
				Raw: []byte(`{"policies":["$(source.cfg.policyName)"]}`),
			},
		},
	}}

	policies, err := LoadExternalPoliciesForWorkflow(context.Background(), cli, "default", steps, nil)
	require.NoError(t, err)
	require.Empty(t, policies)
}
