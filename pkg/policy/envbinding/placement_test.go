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

package envbinding

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestUpdateClusterConnections(t *testing.T) {
	app := &v1beta1.Application{}
	app.Status.LatestRevision = &common.Revision{Name: "v1"}
	status := &v1alpha1.EnvBindingStatus{
		ClusterConnections: []v1alpha1.ClusterConnection{{
			ClusterName:        "cluster-1",
			LastActiveRevision: "v0",
		}, {
			ClusterName:        "cluster-2",
			LastActiveRevision: "v0",
		}},
	}
	decisions := []v1alpha1.PlacementDecision{{
		Cluster: "cluster-1",
	}, {
		Cluster: "cluster-3",
	}}
	updateClusterConnections(status, decisions, app)

	r := require.New(t)
	expectedConnections := []v1alpha1.ClusterConnection{{
		ClusterName:        "cluster-1",
		LastActiveRevision: "v1",
	}, {
		ClusterName:        "cluster-2",
		LastActiveRevision: "v0",
	}, {
		ClusterName:        "cluster-3",
		LastActiveRevision: "v1",
	}}
	r.Equal(len(expectedConnections), len(status.ClusterConnections))
	for idx, conn := range expectedConnections {
		_conn := status.ClusterConnections[idx]
		r.Equal(conn.ClusterName, _conn.ClusterName)
		r.Equal(conn.LastActiveRevision, _conn.LastActiveRevision)
	}
}

func TestWritePlacementDecisions(t *testing.T) {
	policyName := "test-policy"
	envName1 := "env-1"
	envName2 := "env-2"
	decisions1 := []v1alpha1.PlacementDecision{{Cluster: "cluster-1"}}
	decisions2 := []v1alpha1.PlacementDecision{{Cluster: "cluster-2"}}

	makeAppWithPolicy := func(t *testing.T) *v1beta1.Application {
		app := &v1beta1.Application{}
		err := WritePlacementDecisions(app, policyName, envName1, decisions1)
		require.NoError(t, err)
		return app
	}

	testCases := []struct {
		name      string
		setupApp  func(t *testing.T) *v1beta1.Application
		envName   string
		decisions []v1alpha1.PlacementDecision
		wantErr   bool
		verify    func(t *testing.T, app *v1beta1.Application)
	}{
		{
			name: "add to empty policy status",
			setupApp: func(t *testing.T) *v1beta1.Application {
				return &v1beta1.Application{}
			},
			envName:   envName1,
			decisions: decisions1,
			wantErr:   false,
			verify: func(t *testing.T, app *v1beta1.Application) {
				r := require.New(t)
				r.Len(app.Status.PolicyStatus, 1)
				r.Equal(policyName, app.Status.PolicyStatus[0].Name)
				r.Equal(v1alpha1.EnvBindingPolicyType, app.Status.PolicyStatus[0].Type)
				status := &v1alpha1.EnvBindingStatus{}
				err := json.Unmarshal(app.Status.PolicyStatus[0].Status.Raw, status)
				r.NoError(err)
				r.Len(status.Envs, 1)
				r.Equal(envName1, status.Envs[0].Env)
				r.Equal(decisions1, status.Envs[0].Placements)
			},
		},
		{
			name:      "update existing env in existing policy",
			setupApp:  makeAppWithPolicy,
			envName:   envName1,
			decisions: decisions2,
			wantErr:   false,
			verify: func(t *testing.T, app *v1beta1.Application) {
				r := require.New(t)
				r.Len(app.Status.PolicyStatus, 1)
				status := &v1alpha1.EnvBindingStatus{}
				err := json.Unmarshal(app.Status.PolicyStatus[0].Status.Raw, status)
				r.NoError(err)
				r.Len(status.Envs, 1)
				r.Equal(envName1, status.Envs[0].Env)
				r.Equal(decisions2, status.Envs[0].Placements)
			},
		},
		{
			name:      "add new env to existing policy",
			setupApp:  makeAppWithPolicy,
			envName:   envName2,
			decisions: decisions2,
			wantErr:   false,
			verify: func(t *testing.T, app *v1beta1.Application) {
				r := require.New(t)
				r.Len(app.Status.PolicyStatus, 1)
				status := &v1alpha1.EnvBindingStatus{}
				err := json.Unmarshal(app.Status.PolicyStatus[0].Status.Raw, status)
				r.NoError(err)
				r.Len(status.Envs, 2)
				envMap := make(map[string][]v1alpha1.PlacementDecision)
				for _, envStatus := range status.Envs {
					envMap[envStatus.Env] = envStatus.Placements
				}
				r.Equal(decisions1, envMap[envName1])
				r.Equal(decisions2, envMap[envName2])
			},
		},
		{
			name: "handle malformed existing status",
			setupApp: func(t *testing.T) *v1beta1.Application {
				return &v1beta1.Application{
					Status: common.AppStatus{
						PolicyStatus: []common.PolicyStatus{
							{
								Name:   policyName,
								Type:   v1alpha1.EnvBindingPolicyType,
								Status: &runtime.RawExtension{Raw: []byte("this is not json")},
							},
						},
					},
				}
			},
			envName:   envName1,
			decisions: decisions1,
			wantErr:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			app := tc.setupApp(t)
			err := WritePlacementDecisions(app, policyName, tc.envName, tc.decisions)

			if tc.wantErr {
				r.Error(err)
			} else {
				r.NoError(err)
			}

			if tc.verify != nil {
				tc.verify(t, app)
			}
		})
	}
}
