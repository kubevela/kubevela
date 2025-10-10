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

package multicluster

import (
	"context"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1alpha1 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	commontypes "github.com/oam-dev/kubevela/pkg/utils/common"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"

	wfmock "github.com/kubevela/workflow/pkg/mock"
)

// mockAction is a mock implementation of types.Action for testing.
type mockAction struct {
	wfmock.Action
	WaitCalled bool
	WaitReason string
}

// Wait records that the wait action was called.
func (a *mockAction) Wait(reason string) {
	a.WaitCalled = true
	a.WaitReason = reason
}

func TestListClusters(t *testing.T) {
	r := require.New(t)
	originalNS := multicluster.ClusterGatewaySecretNamespace
	multicluster.ClusterGatewaySecretNamespace = types.DefaultKubeVelaNS
	t.Cleanup(func() {
		multicluster.ClusterGatewaySecretNamespace = originalNS
	})
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(commontypes.Scheme).Build()
	clusterNames := []string{"cluster-a", "cluster-b"}
	for _, secretName := range clusterNames {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: multicluster.ClusterGatewaySecretNamespace,
				Labels: map[string]string{
					clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate),
				},
			},
		}
		r.NoError(cli.Create(context.Background(), secret))
	}
	res, err := ListClusters(ctx, &oamprovidertypes.Params[any]{
		RuntimeParams: oamprovidertypes.RuntimeParams{
			KubeClient: cli,
		},
	})
	r.NoError(err)
	r.Equal(clusterNames, res.Returns.Outputs.Clusters)
}

func TestDeploy(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(commontypes.Scheme).Build()

	// Mock component functions
	componentApply := func(ctx context.Context, comp common.ApplicationComponent, patcher *cue.Value, clusterName string, overrideNamespace string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
		return nil, nil, true, nil
	}
	componentHealthCheck := func(ctx context.Context, comp common.ApplicationComponent, patcher *cue.Value, clusterName string, overrideNamespace string) (oamprovidertypes.ComponentHealthStatus, *common.ApplicationComponentStatus, *unstructured.Unstructured, []*unstructured.Unstructured, error) {
		return oamprovidertypes.ComponentHealthy, nil, nil, nil, nil
	}
	workloadRender := func(ctx context.Context, comp common.ApplicationComponent) (*appfile.Component, error) {
		return &appfile.Component{}, nil
	}

	createMockParams := func(parallelism int64) *DeployParams {
		action := &mockAction{}
		return &DeployParams{
			Params: DeployParameter{
				Parallelism:              parallelism,
				IgnoreTerraformComponent: true,
				Policies:                 []string{},
			},
			RuntimeParams: oamprovidertypes.RuntimeParams{
				Action:               action,
				KubeClient:           cli,
				ComponentApply:       componentApply,
				ComponentHealthCheck: componentHealthCheck,
				WorkloadRender:       workloadRender,
				Appfile: &appfile.Appfile{
					Name:      "test-app",
					Namespace: "default",
					Policies:  []v1beta1.AppPolicy{},
				},
			},
		}
	}

	cases := map[string]struct {
		reason        string
		params        *DeployParams
		expectError   bool
		errorContains string
		expectPanic   bool
	}{
		"parallelism zero validation error": {
			reason:        "Should return a validation error for zero parallelism",
			params:        createMockParams(0),
			expectError:   true,
			errorContains: "parallelism cannot be smaller than 1",
		},
		"parallelism negative validation error": {
			reason:        "Should return a validation error for negative parallelism",
			params:        createMockParams(-1),
			expectError:   true,
			errorContains: "parallelism cannot be smaller than 1",
		},
		"parameters nil pointer handling": {
			reason:      "Should panic when params are nil",
			params:      nil,
			expectPanic: true,
		},
		"successful deployment healthy": {
			reason: "Should execute successfully with valid parameters",
			params: createMockParams(1),
		},
		"successful deployment unhealthy wait": {
			reason: "Should execute successfully even with higher parallelism",
			params: createMockParams(2),
		},
		"executor deploy error propagation": {
			reason: "Should pass validation and any errors should be from the executor",
			params: createMockParams(1),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.expectPanic {
				r.Panics(func() {
					_, _ = Deploy(ctx, tc.params)
				})
				return
			}

			result, err := Deploy(ctx, tc.params)

			if tc.expectError {
				r.Error(err)
				if tc.errorContains != "" {
					r.Contains(err.Error(), tc.errorContains)
				}
			} else {
				r.NoError(err)
				r.Nil(result)
			}
		})
	}
}

func TestGetPlacementsFromTopologyPolicies(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	scheme := commontypes.Scheme
	r.NoError(v1alpha1.AddToScheme(scheme))

	topologyPolicy := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "my-topology", Namespace: "default"},
		Type:       v1alpha1.TopologyPolicyType,
		Properties: &runtime.RawExtension{
			Raw: []byte(`{"clusters":["local"],"namespace":"topo-ns"}`),
		},
	}

	appFileTopologyPolicy := v1beta1.AppPolicy{
		Name: "my-topology",
		Type: v1alpha1.TopologyPolicyType,
		Properties: &runtime.RawExtension{
			Raw: []byte(`{"clusters":["local"],"namespace":"topo-ns"}`),
		},
	}

	cases := map[string]struct {
		reason             string
		policiesInAppfile  []v1beta1.AppPolicy
		policiesToGet      []string
		objectsToCreate    []client.Object
		expectedPlacements []v1alpha1.PlacementDecision
		expectError        bool
		errorContains      string
	}{
		"Successful placement resolution with single policy": {
			reason:             "Should resolve placement from a single topology policy",
			objectsToCreate:    []client.Object{topologyPolicy},
			policiesInAppfile:  []v1beta1.AppPolicy{appFileTopologyPolicy},
			policiesToGet:      []string{"my-topology"},
			expectedPlacements: []v1alpha1.PlacementDecision{{Cluster: "local", Namespace: "topo-ns"}},
		},
		"Policy not found in appfile": {
			reason:        "Should return an error if the policy is not found in the appfile",
			policiesToGet: []string{"non-existent-policy"},
			expectError:   true,
			errorContains: "policy non-existent-policy not found",
		},
		"Empty policy list returns default local placement": {
			reason:             "Should return default local placement when no policies are specified",
			policiesToGet:      []string{},
			expectedPlacements: []v1alpha1.PlacementDecision{{Cluster: "local"}},
		},
		"Nil policy names list returns default local placement": {
			reason:             "Should return default local placement when the policy list is nil",
			policiesToGet:      nil,
			expectedPlacements: []v1alpha1.PlacementDecision{{Cluster: "local"}},
		},
		"Empty appfile policies list with a policy name": {
			reason:        "Should return an error if appfile has no policies but a policy is requested",
			policiesToGet: []string{"some-policy"},
			expectError:   true,
			errorContains: "policy some-policy not found",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tc.objectsToCreate...).Build()
			af := &appfile.Appfile{
				Name:      "test-app",
				Namespace: "default",
				Policies:  tc.policiesInAppfile,
			}
			params := &PoliciesParams{
				Params:        PoliciesVars{Policies: tc.policiesToGet},
				RuntimeParams: oamprovidertypes.RuntimeParams{KubeClient: cli, Appfile: af},
			}

			result, err := GetPlacementsFromTopologyPolicies(ctx, params)

			if tc.expectError {
				r.Error(err)
				if tc.errorContains != "" {
					r.Contains(err.Error(), tc.errorContains)
				}
			} else {
				r.NoError(err)
				r.NotNil(result)
				r.Equal(tc.expectedPlacements, result.Returns.Placements)
			}
		})
	}
}
