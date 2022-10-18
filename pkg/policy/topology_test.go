/*
Copyright 2022 The KubeVela Authors.

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

package policy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1alpha1 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestGetClusterLabelSelectorInTopology(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.DeprecatedPolicySpec, true)()
	multicluster.ClusterGatewaySecretNamespace = types.DefaultKubeVelaNS
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-a",
			Namespace: multicluster.ClusterGatewaySecretNamespace,
			Labels: map[string]string{
				clustercommon.LabelKeyClusterEndpointType:   string(clusterv1alpha1.ClusterEndpointTypeConst),
				clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate),
				"key": "value",
			},
		},
	}, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-b",
			Namespace: multicluster.ClusterGatewaySecretNamespace,
			Labels: map[string]string{
				clustercommon.LabelKeyClusterEndpointType:   string(clusterv1alpha1.ClusterEndpointTypeConst),
				clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate),
				"key": "value",
			},
		},
	}, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-c",
			Namespace: multicluster.ClusterGatewaySecretNamespace,
			Labels: map[string]string{
				clustercommon.LabelKeyClusterEndpointType:   string(clusterv1alpha1.ClusterEndpointTypeConst),
				clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate),
				"key": "none",
			},
		},
	}).Build()
	appNs := "test"
	testCases := map[string]struct {
		Inputs              []v1beta1.AppPolicy
		Outputs             []v1alpha1.PlacementDecision
		Error               string
		AllowCrossNamespace bool
	}{
		"invalid-topology-policy": {
			Inputs: []v1beta1.AppPolicy{{
				Name:       "topology-policy",
				Type:       "topology",
				Properties: &runtime.RawExtension{Raw: []byte(`{"cluster":"x"}`)},
			}},
			Error: "failed to parse topology policy",
		},
		"cluster-not-found": {
			Inputs: []v1beta1.AppPolicy{{
				Name:       "topology-policy",
				Type:       "topology",
				Properties: &runtime.RawExtension{Raw: []byte(`{"clusters":["cluster-x"]}`)},
			}},
			Error: "failed to get cluster",
		},
		"topology-by-clusters": {
			Inputs: []v1beta1.AppPolicy{{
				Name:       "topology-policy",
				Type:       "topology",
				Properties: &runtime.RawExtension{Raw: []byte(`{"clusters":["cluster-a"]}`)},
			}},
			Outputs: []v1alpha1.PlacementDecision{{Cluster: "cluster-a", Namespace: ""}},
		},
		"topology-by-cluster-selector-404": {
			Inputs: []v1beta1.AppPolicy{{
				Name:       "topology-policy",
				Type:       "topology",
				Properties: &runtime.RawExtension{Raw: []byte(`{"clusterSelector":{"key":"bad-value"}}`)},
			}},
			Error: "failed to find any cluster matches given labels",
		},
		"topology-by-cluster-selector-ignore-404": {
			Inputs: []v1beta1.AppPolicy{{
				Name:       "topology-policy",
				Type:       "topology",
				Properties: &runtime.RawExtension{Raw: []byte(`{"clusterSelector":{"key":"bad-value"},"allowEmpty":true}`)},
			}},
			Outputs: []v1alpha1.PlacementDecision{},
		},
		"topology-by-cluster-selector": {
			Inputs: []v1beta1.AppPolicy{{
				Name:       "topology-policy",
				Type:       "topology",
				Properties: &runtime.RawExtension{Raw: []byte(`{"clusterSelector":{"key":"value"}}`)},
			}},
			Outputs: []v1alpha1.PlacementDecision{{Cluster: "cluster-a", Namespace: ""}, {Cluster: "cluster-b", Namespace: ""}},
		},
		"topology-by-cluster-label-selector": {
			Inputs: []v1beta1.AppPolicy{{
				Name:       "topology-policy",
				Type:       "topology",
				Properties: &runtime.RawExtension{Raw: []byte(`{"clusterLabelSelector":{"key":"value"}}`)},
			}},
			Outputs: []v1alpha1.PlacementDecision{{Cluster: "cluster-a", Namespace: ""}, {Cluster: "cluster-b", Namespace: ""}},
		},
		"topology-by-cluster-selector-and-namespace-invalid": {
			Inputs: []v1beta1.AppPolicy{{
				Name:       "topology-policy",
				Type:       "topology",
				Properties: &runtime.RawExtension{Raw: []byte(`{"clusterSelector":{"key":"value"},"namespace":"override"}`)},
			}},
			Error: "cannot cross namespace",
		},
		"topology-by-cluster-selector-and-namespace": {
			Inputs: []v1beta1.AppPolicy{{
				Name:       "topology-policy",
				Type:       "topology",
				Properties: &runtime.RawExtension{Raw: []byte(`{"clusterSelector":{"key":"value"},"namespace":"override"}`)},
			}},
			Outputs:             []v1alpha1.PlacementDecision{{Cluster: "cluster-a", Namespace: "override"}, {Cluster: "cluster-b", Namespace: "override"}},
			AllowCrossNamespace: true,
		},
		"topology-no-clusters-and-cluster-label-selector": {
			Inputs: []v1beta1.AppPolicy{{
				Name:       "topology-policy",
				Type:       "topology",
				Properties: &runtime.RawExtension{Raw: []byte(`{"namespace":"override"}`)},
			}},
			Outputs:             []v1alpha1.PlacementDecision{{Cluster: "local", Namespace: "override"}},
			AllowCrossNamespace: true,
		},
		"no-topology-policy": {
			Inputs:  []v1beta1.AppPolicy{},
			Outputs: []v1alpha1.PlacementDecision{{Cluster: "local", Namespace: ""}},
		},
		"empty-topology-policy": {
			Inputs: []v1beta1.AppPolicy{{Type: "topology", Name: "some-name", Properties: nil}},
			Error:  "have empty properties",
		},
	}
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			pds, err := GetPlacementsFromTopologyPolicies(context.Background(), cli, appNs, tt.Inputs, tt.AllowCrossNamespace)
			if tt.Error != "" {
				r.NotNil(err)
				r.Contains(err.Error(), tt.Error)
			} else {
				r.NoError(err)
				r.Equal(tt.Outputs, pds)
			}
		})
	}
}
