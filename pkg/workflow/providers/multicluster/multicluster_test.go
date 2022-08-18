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

	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/mock"
	clusterv1alpha1 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestReadPlacementDecisions(t *testing.T) {
	testCases := []struct {
		InputVal             map[string]interface{}
		OldCluster           string
		OldNamespace         string
		ExpectError          string
		ExpectDecisionExists bool
		ExpectCluster        string
		ExpectNamespace      string
	}{{
		InputVal:    map[string]interface{}{},
		ExpectError: "var(path=inputs.policyName) not exist",
	}, {
		InputVal: map[string]interface{}{
			"policyName": "example-policy",
		},
		ExpectError: "var(path=inputs.envName) not exist",
	}, {
		InputVal: map[string]interface{}{
			"policyName": "example-policy",
			"envName":    "example-env",
		},
		ExpectError:          "",
		ExpectDecisionExists: false,
	}, {
		InputVal: map[string]interface{}{
			"policyName": "example-policy",
			"envName":    "example-env",
		},
		OldCluster:           "example-cluster",
		OldNamespace:         "example-namespace",
		ExpectError:          "",
		ExpectDecisionExists: true,
		ExpectCluster:        "example-cluster",
		ExpectNamespace:      "example-namespace",
	}}
	r := require.New(t)
	for _, testCase := range testCases {
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
		app := &v1beta1.Application{}
		p := &provider{
			Client: cli,
			app:    app,
		}
		act := &mock.Action{}
		v, err := value.NewValue("", nil, "")
		r.NoError(err)
		r.NoError(v.FillObject(testCase.InputVal, "inputs"))
		if testCase.ExpectCluster != "" || testCase.ExpectNamespace != "" {
			pd := v1alpha1.PlacementDecision{
				Cluster:   testCase.OldCluster,
				Namespace: testCase.OldNamespace,
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
		err = p.ReadPlacementDecisions(nil, nil, v, act)
		if testCase.ExpectError == "" {
			r.NoError(err)
		} else {
			r.Contains(err.Error(), testCase.ExpectError)
			continue
		}
		outputs, err := v.LookupValue("outputs")
		r.NoError(err)
		md := map[string][]v1alpha1.PlacementDecision{}
		r.NoError(outputs.UnmarshalTo(&md))
		if !testCase.ExpectDecisionExists {
			r.Equal(0, len(md))
		} else {
			r.Equal(1, len(md["decisions"]))
			r.Equal(testCase.ExpectCluster, md["decisions"][0].Cluster)
			r.Equal(testCase.ExpectNamespace, md["decisions"][0].Namespace)
		}
	}
}

func TestMakePlacementDecisions(t *testing.T) {
	multicluster.ClusterGatewaySecretNamespace = types.DefaultKubeVelaNS
	testCases := []struct {
		InputVal        map[string]interface{}
		OldCluster      string
		OldNamespace    string
		ExpectError     string
		ExpectCluster   string
		ExpectNamespace string
		PreAddCluster   string
	}{{
		InputVal:    map[string]interface{}{},
		ExpectError: "var(path=inputs.policyName) not exist",
	}, {
		InputVal: map[string]interface{}{
			"policyName": "example-policy",
		},
		ExpectError: "var(path=inputs.envName) not exist",
	}, {
		InputVal: map[string]interface{}{
			"policyName": "example-policy",
			"envName":    "example-env",
		},
		ExpectError: "var(path=inputs.placement) not exist",
	}, {
		InputVal: map[string]interface{}{
			"policyName": "example-policy",
			"envName":    "example-env",
			"placement":  "example-placement",
		},
		ExpectError: "failed to parse placement while making placement decision",
	}, {
		InputVal: map[string]interface{}{
			"policyName": "example-policy",
			"envName":    "example-env",
			"placement": map[string]interface{}{
				"namespaceSelector": map[string]interface{}{
					"labels": map[string]string{"key": "value"},
				},
			},
		},
		ExpectError: "namespace selector in cluster-gateway does not support label selector for now",
	}, {
		InputVal: map[string]interface{}{
			"policyName": "example-policy",
			"envName":    "example-env",
			"placement": map[string]interface{}{
				"clusterSelector": map[string]interface{}{
					"labels": map[string]string{"key": "value"},
				},
			},
		},
		ExpectError: "cluster selector does not support label selector for now",
	}, {
		InputVal: map[string]interface{}{
			"policyName": "example-policy",
			"envName":    "example-env",
			"placement":  map[string]interface{}{},
		},
		ExpectError:     "",
		ExpectCluster:   "local",
		ExpectNamespace: "",
	}, {
		InputVal: map[string]interface{}{
			"policyName": "example-policy",
			"envName":    "example-env",
			"placement": map[string]interface{}{
				"clusterSelector": map[string]interface{}{
					"name": "example-cluster",
				},
				"namespaceSelector": map[string]interface{}{
					"name": "example-namespace",
				},
			},
		},
		ExpectError: "failed to get cluster",
	}, {
		InputVal: map[string]interface{}{
			"policyName": "example-policy",
			"envName":    "example-env",
			"placement": map[string]interface{}{
				"clusterSelector": map[string]interface{}{
					"name": "example-cluster",
				},
				"namespaceSelector": map[string]interface{}{
					"name": "example-namespace",
				},
			},
		},
		ExpectError:     "",
		ExpectCluster:   "example-cluster",
		ExpectNamespace: "example-namespace",
		PreAddCluster:   "example-cluster",
	}, {
		InputVal: map[string]interface{}{
			"policyName": "example-policy",
			"envName":    "example-env",
			"placement": map[string]interface{}{
				"clusterSelector": map[string]interface{}{
					"name": "example-cluster",
				},
				"namespaceSelector": map[string]interface{}{
					"name": "example-namespace",
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
		InputVal: map[string]interface{}{
			"policyName": "example-policy",
			"envName":    "example-env",
			"placement": map[string]interface{}{
				"clusterSelector": map[string]interface{}{
					"name": "example-cluster",
				},
				"namespaceSelector": map[string]interface{}{
					"name": "example-namespace",
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
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
		app := &v1beta1.Application{}
		p := &provider{
			Client: cli,
			app:    app,
		}
		act := &mock.Action{}
		v, err := value.NewValue("", nil, "")
		r.NoError(err)
		r.NoError(v.FillObject(testCase.InputVal, "inputs"))
		if testCase.PreAddCluster != "" {
			r.NoError(cli.Create(context.Background(), &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: multicluster.ClusterGatewaySecretNamespace,
					Name:      testCase.PreAddCluster,
					Labels:    map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate)},
				},
			}))
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
		err = p.MakePlacementDecisions(nil, nil, v, act)
		if testCase.ExpectError == "" {
			r.NoError(err)
		} else {
			r.Contains(err.Error(), testCase.ExpectError)
			continue
		}
		outputs, err := v.LookupValue("outputs")
		r.NoError(err)
		md := map[string][]v1alpha1.PlacementDecision{}
		r.NoError(outputs.UnmarshalTo(&md))
		r.Equal(1, len(md["decisions"]))
		r.Equal(testCase.ExpectCluster, md["decisions"][0].Cluster)
		r.Equal(testCase.ExpectNamespace, md["decisions"][0].Namespace)
		r.Equal(1, len(app.Status.PolicyStatus))
		r.Equal(testCase.InputVal["policyName"], app.Status.PolicyStatus[0].Name)
		r.Equal(v1alpha1.EnvBindingPolicyType, app.Status.PolicyStatus[0].Type)
		status := &v1alpha1.EnvBindingStatus{}
		r.NoError(json.Unmarshal(app.Status.PolicyStatus[0].Status.Raw, status))
		r.Equal(1, len(status.Envs))
		r.Equal(testCase.InputVal["envName"], status.Envs[0].Env)
		r.Equal(1, len(status.Envs[0].Placements))
		r.Equal(testCase.ExpectNamespace, status.Envs[0].Placements[0].Namespace)
		r.Equal(testCase.ExpectCluster, status.Envs[0].Placements[0].Cluster)
	}
}

func TestPatchApplication(t *testing.T) {
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
		InputVal         map[string]interface{}
		ExpectError      string
		ExpectComponents []apicommon.ApplicationComponent
	}{{
		InputVal:    map[string]interface{}{},
		ExpectError: "var(path=inputs.envName) not exist",
	}, {
		InputVal: map[string]interface{}{
			"envName": "example-env",
		},
		ExpectComponents: baseApp.Spec.Components,
	}, {
		InputVal: map[string]interface{}{
			"envName": "example-env",
			"patch":   "bad patch",
		},
		ExpectError: "failed to unmarshal patch for env",
	}, {
		InputVal: map[string]interface{}{
			"envName":  "example-env",
			"selector": "bad selector",
		},
		ExpectError: "failed to unmarshal selector for env",
	}, {
		InputVal: map[string]interface{}{
			"envName": "example-env",
			"patch": map[string]interface{}{
				"components": []map[string]interface{}{{
					"name": "comp-0",
					"type": "webservice",
				}, {
					"name": "comp-1",
					"type": "worker",
					"properties": map[string]interface{}{
						"image": "patch",
						"port":  8080,
					},
				}, {
					"name": "comp-3",
					"type": "webservice",
					"properties": map[string]interface{}{
						"image": "patch",
						"port":  8090,
					},
					"traits": []map[string]interface{}{{
						"type":       "scaler",
						"properties": map[string]interface{}{"replicas": 5},
					}, {
						"type":       "env",
						"properties": map[string]interface{}{"env": map[string]string{"Key": "Value"}},
					}, {
						"type":       "annotations",
						"properties": map[string]interface{}{"aKey": "aVal"}},
					},
				}, {
					"name": "comp-4",
					"type": "webservice",
				}},
			},
			"selector": map[string]interface{}{
				"components": []string{"comp-2", "comp-1", "comp-3", "comp-0"},
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
	r := require.New(t)
	for _, testCase := range testCases {
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
		p := &provider{
			Client: cli,
			app:    baseApp,
		}
		act := &mock.Action{}
		v, err := value.NewValue("", nil, "")
		r.NoError(err)
		r.NoError(v.FillObject(testCase.InputVal, "inputs"))
		err = p.PatchApplication(nil, nil, v, act)
		if testCase.ExpectError == "" {
			r.NoError(err)
		} else {
			r.Contains(err.Error(), testCase.ExpectError)
			continue
		}
		outputs, err := v.LookupValue("outputs")
		r.NoError(err)
		patchApp := &v1beta1.Application{}
		r.NoError(outputs.UnmarshalTo(patchApp))
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
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	clusterNames := []string{"cluster-a", "cluster-b"}
	for _, secretName := range clusterNames {
		secret := &corev1.Secret{}
		secret.Name = secretName
		secret.Namespace = multicluster.ClusterGatewaySecretNamespace
		secret.Labels = map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate)}
		r.NoError(cli.Create(context.Background(), secret))
	}
	app := &v1beta1.Application{}
	p := &provider{
		Client: cli,
		app:    app,
	}
	act := &mock.Action{}
	v, err := value.NewValue("", nil, "")
	r.NoError(err)
	r.NoError(p.ListClusters(nil, nil, v, act))
	outputs, err := v.LookupValue("outputs")
	r.NoError(err)
	obj := struct {
		Clusters []string `json:"clusters"`
	}{}
	r.NoError(outputs.UnmarshalTo(&obj))
	r.Equal(clusterNames, obj.Clusters)
}
