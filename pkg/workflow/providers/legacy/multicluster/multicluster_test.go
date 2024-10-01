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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/workflow/pkg/mock"
	clusterv1alpha1 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

func TestMakePlacementDecisions(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	ctx := context.Background()
	multicluster.ClusterGatewaySecretNamespace = types.DefaultKubeVelaNS
	testCases := []struct {
		InputVal        PlacementDecisionVars
		OldCluster      string
		OldNamespace    string
		ExpectError     string
		ExpectCluster   string
		ExpectNamespace string
		PreAddCluster   string
	}{{
		InputVal:    PlacementDecisionVars{},
		ExpectError: "empty policy name",
	}, {
		InputVal: PlacementDecisionVars{
			PolicyName: "example-policy",
		},
		ExpectError: "empty env name",
	}, {
		InputVal: PlacementDecisionVars{
			PolicyName: "example-policy",
			EnvName:    "example-env",
		},
		ExpectError: "empty placement for policy example-policy in env example-env",
	}, {
		InputVal: PlacementDecisionVars{
			PolicyName: "example-policy",
			EnvName:    "example-env",
			Placement: &v1alpha1.EnvPlacement{
				NamespaceSelector: &v1alpha1.NamespaceSelector{
					Labels: map[string]string{"key": "value"},
				},
			},
		},
		ExpectError: "namespace selector in cluster-gateway does not support label selector for now",
	}, {
		InputVal: PlacementDecisionVars{
			PolicyName: "example-policy",
			EnvName:    "example-env",
			Placement: &v1alpha1.EnvPlacement{
				ClusterSelector: &apicommon.ClusterSelector{
					Labels: map[string]string{"key": "value"},
				},
			},
		},
		ExpectError: "cluster selector does not support label selector for now",
	}, {
		InputVal: PlacementDecisionVars{
			PolicyName: "example-policy",
			EnvName:    "example-env",
			Placement:  &v1alpha1.EnvPlacement{},
		},
		ExpectError:     "",
		ExpectCluster:   "local",
		ExpectNamespace: "",
	}, {
		InputVal: PlacementDecisionVars{
			PolicyName: "example-policy",
			EnvName:    "example-env",
			Placement: &v1alpha1.EnvPlacement{
				NamespaceSelector: &v1alpha1.NamespaceSelector{
					Name: "example-namespace",
				},
				ClusterSelector: &apicommon.ClusterSelector{
					Name: "example-cluster",
				},
			},
		},
		ExpectError: "failed to get cluster",
	}, {
		InputVal: PlacementDecisionVars{
			PolicyName: "example-policy",
			EnvName:    "example-env",
			Placement: &v1alpha1.EnvPlacement{
				NamespaceSelector: &v1alpha1.NamespaceSelector{
					Name: "example-namespace",
				},
				ClusterSelector: &apicommon.ClusterSelector{
					Name: "example-cluster",
				},
			},
		},
		ExpectError:     "",
		ExpectCluster:   "example-cluster",
		ExpectNamespace: "example-namespace",
		PreAddCluster:   "example-cluster",
	}, {
		InputVal: PlacementDecisionVars{
			PolicyName: "example-policy",
			EnvName:    "example-env",
			Placement: &v1alpha1.EnvPlacement{
				NamespaceSelector: &v1alpha1.NamespaceSelector{
					Name: "example-namespace",
				},
				ClusterSelector: &apicommon.ClusterSelector{
					Name: "example-cluster",
				},
			},
		},
		OldCluster:      "old-cluster",
		OldNamespace:    "old-namespace",
		ExpectError:     "",
		ExpectCluster:   "example-cluster",
		ExpectNamespace: "example-namespace",
		PreAddCluster:   "example-cluster",
	}, {
		InputVal: PlacementDecisionVars{
			PolicyName: "example-policy",
			EnvName:    "example-env",
			Placement: &v1alpha1.EnvPlacement{
				NamespaceSelector: &v1alpha1.NamespaceSelector{
					Name: "example-namespace",
				},
				ClusterSelector: &apicommon.ClusterSelector{
					Name: "example-cluster",
				},
			},
		},
		ExpectError:     "",
		ExpectCluster:   "example-cluster",
		ExpectNamespace: "example-namespace",
		PreAddCluster:   "example-cluster",
	}}

	r := require.New(t)
	for _, testCase := range testCases {
		app := &v1beta1.Application{}
		act := &mock.Action{}
		if testCase.PreAddCluster != "" {
			_ = cli.Create(context.Background(), &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: multicluster.ClusterGatewaySecretNamespace,
					Name:      testCase.PreAddCluster,
					Labels:    map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate)},
				},
			})
		}
		if testCase.OldNamespace != "" || testCase.OldCluster != "" {
			pd := v1alpha1.PlacementDecision{
				Cluster:   testCase.OldNamespace,
				Namespace: testCase.OldCluster,
			}
			bs, err := json.Marshal(&v1alpha1.EnvBindingStatus{
				Envs: []v1alpha1.EnvStatus{{
					Env:        "example-env",
					Placements: []v1alpha1.PlacementDecision{pd},
				}},
			})
			r.NoError(err)
			app.Status.PolicyStatus = []apicommon.PolicyStatus{{
				Name:   "example-policy",
				Type:   v1alpha1.EnvBindingPolicyType,
				Status: &runtime.RawExtension{Raw: bs},
			}}
		}
		res, err := MakePlacementDecisions(ctx, &PlacementDecisionParams{
			Params: Inputs[PlacementDecisionVars]{
				Inputs: testCase.InputVal,
			},
			RuntimeParams: oamprovidertypes.RuntimeParams{
				App:        app,
				Action:     act,
				KubeClient: cli,
			},
		})
		if testCase.ExpectError == "" {
			r.NoError(err)
		} else {
			r.Contains(err.Error(), testCase.ExpectError)
			continue
		}
		md := res.Outputs.Decisions
		r.Equal(1, len(md))
		r.Equal(testCase.ExpectCluster, md[0].Cluster)
		r.Equal(testCase.ExpectNamespace, md[0].Namespace)
		r.Equal(1, len(app.Status.PolicyStatus))
		r.Equal(testCase.InputVal.PolicyName, app.Status.PolicyStatus[0].Name)
		r.Equal(v1alpha1.EnvBindingPolicyType, app.Status.PolicyStatus[0].Type)
		status := &v1alpha1.EnvBindingStatus{}
		r.NoError(json.Unmarshal(app.Status.PolicyStatus[0].Status.Raw, status))
		r.Equal(1, len(status.Envs))
		r.Equal(testCase.InputVal.EnvName, status.Envs[0].Env)
		r.Equal(1, len(status.Envs[0].Placements))
		r.Equal(testCase.ExpectNamespace, status.Envs[0].Placements[0].Namespace)
		r.Equal(testCase.ExpectCluster, status.Envs[0].Placements[0].Cluster)
	}
}

func TestPatchApplication(t *testing.T) {
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	baseApp := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{
		Components: []apicommon.ApplicationComponent{{
			Name:       "comp-1",
			Type:       "webservice",
			Properties: &runtime.RawExtension{Raw: []byte(`{"image":"base"}`)},
		}, {
			Name:       "comp-3",
			Type:       "webservice",
			Properties: &runtime.RawExtension{Raw: []byte(`{"image":"ext"}`)},
			Traits: []apicommon.ApplicationTrait{{
				Type:       "scaler",
				Properties: &runtime.RawExtension{Raw: []byte(`{"replicas":3}`)},
			}, {
				Type:       "env",
				Properties: &runtime.RawExtension{Raw: []byte(`{"env":{"key":"value"}}`)},
			}, {
				Type:       "labels",
				Properties: &runtime.RawExtension{Raw: []byte(`{"lKey":"lVal"}`)},
			}},
		}},
	}}
	testCases := []struct {
		InputVal         ApplicationVars
		ExpectError      string
		ExpectComponents []apicommon.ApplicationComponent
	}{{
		InputVal:    ApplicationVars{},
		ExpectError: "empty env name",
	}, {
		InputVal: ApplicationVars{
			EnvName: "example-env",
		},
		ExpectComponents: baseApp.Spec.Components,
	}, {
		InputVal: ApplicationVars{
			EnvName: "example-env",
			Patch: &v1alpha1.EnvPatch{
				Components: []v1alpha1.EnvComponentPatch{
					{
						Name: "comp-0",
						Type: "webservice",
					}, {
						Name:       "comp-1",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"image":"patch","port":8080}`)},
					}, {
						Name:       "comp-3",
						Type:       "webservice",
						Properties: &runtime.RawExtension{Raw: []byte(`{"image":"patch","port":8090}`)},
						Traits: []v1alpha1.EnvTraitPatch{
							{
								Type:       "scaler",
								Properties: &runtime.RawExtension{Raw: []byte(`{"replicas":5}`)},
							}, {
								Type:       "env",
								Properties: &runtime.RawExtension{Raw: []byte(`{"env":{"Key":"Value"}}`)},
							}, {
								Type:       "annotations",
								Properties: &runtime.RawExtension{Raw: []byte(`{"aKey":"aVal"}`)},
							},
						}}, {
						Name: "comp-4",
						Type: "webservice",
					},
				},
			},
			Selector: &v1alpha1.EnvSelector{
				Components: []string{"comp-2", "comp-1", "comp-3", "comp-0"},
			},
		},
		ExpectComponents: []apicommon.ApplicationComponent{{
			Name:       "comp-1",
			Type:       "worker",
			Properties: &runtime.RawExtension{Raw: []byte(`{"image":"patch","port":8080}`)},
		}, {
			Name:       "comp-3",
			Type:       "webservice",
			Properties: &runtime.RawExtension{Raw: []byte(`{"image":"patch","port":8090}`)},
			Traits: []apicommon.ApplicationTrait{{
				Type:       "scaler",
				Properties: &runtime.RawExtension{Raw: []byte(`{"replicas":5}`)},
			}, {
				Type:       "env",
				Properties: &runtime.RawExtension{Raw: []byte(`{"env":{"Key":"Value","key":"value"}}`)},
			}, {
				Type:       "labels",
				Properties: &runtime.RawExtension{Raw: []byte(`{"lKey":"lVal"}`)},
			}, {
				Type:       "annotations",
				Properties: &runtime.RawExtension{Raw: []byte(`{"aKey":"aVal"}`)},
			}},
		}, {
			Name: "comp-0",
			Type: "webservice",
		}},
	}}
	for _, testCase := range testCases {
		r := require.New(t)
		act := &mock.Action{}
		res, err := PatchApplication(ctx, &ApplicationParams{
			Params: Inputs[ApplicationVars]{
				Inputs: testCase.InputVal,
			},
			RuntimeParams: oamprovidertypes.RuntimeParams{
				Action:     act,
				App:        baseApp,
				KubeClient: cli,
			},
		})
		if testCase.ExpectError == "" {
			r.NoError(err)
		} else {
			r.Contains(err.Error(), testCase.ExpectError)
			continue
		}
		patchApp := res.Outputs
		r.Equal(len(testCase.ExpectComponents), len(patchApp.Spec.Components))
		for idx, comp := range testCase.ExpectComponents {
			_comp := patchApp.Spec.Components[idx]
			r.Equal(comp.Name, _comp.Name)
			r.Equal(comp.Type, _comp.Type)
			if comp.Properties == nil {
				r.Equal(comp.Properties, _comp.Properties)
			} else {
				r.Equal(string(comp.Properties.Raw), string(_comp.Properties.Raw))
			}
			r.Equal(len(comp.Traits), len(_comp.Traits))
			for _idx, trait := range comp.Traits {
				_trait := _comp.Traits[_idx]
				r.Equal(trait.Type, _trait.Type)
				if trait.Properties == nil {
					r.Equal(trait.Properties, _trait.Properties)
				} else {
					r.Equal(string(trait.Properties.Raw), string(_trait.Properties.Raw))
				}
			}
		}
	}
}

func TestListClusters(t *testing.T) {
	multicluster.ClusterGatewaySecretNamespace = types.DefaultKubeVelaNS
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	clusterNames := []string{"cluster-a", "cluster-b"}
	for _, secretName := range clusterNames {
		secret := &corev1.Secret{}
		secret.Name = secretName
		secret.Namespace = multicluster.ClusterGatewaySecretNamespace
		secret.Labels = map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate)}
		r.NoError(cli.Create(context.Background(), secret))
	}
	res, err := ListClusters(ctx, &oamprovidertypes.OAMParams[any]{
		RuntimeParams: oamprovidertypes.RuntimeParams{
			KubeClient: cli,
		},
	})
	r.NoError(err)
	r.Equal(clusterNames, res.Outputs.Clusters)
}
