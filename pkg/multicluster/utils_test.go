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

	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	velacommon "github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestUpgradeExistingClusterSecret(t *testing.T) {
	oldClusterGatewaySecretNamespace := ClusterGatewaySecretNamespace
	ClusterGatewaySecretNamespace = "default"
	defer func() {
		ClusterGatewaySecretNamespace = oldClusterGatewaySecretNamespace
	}()
	ctx := context.Background()
	c := fake.NewClientBuilder().WithScheme(velacommon.Scheme).Build()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-outdated-cluster-secret",
			Namespace: "default",
			Labels: map[string]string{
				"cluster.core.oam.dev/cluster-credential": "tls",
			},
		},
		Type: corev1.SecretTypeTLS,
	}
	require.NoError(t, c.Create(ctx, secret))
	require.NoError(t, UpgradeExistingClusterSecret(ctx, c))
	newSecret := &corev1.Secret{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(secret), newSecret))
	require.Equal(t, string(v1alpha1.CredentialTypeX509Certificate), newSecret.Labels[clustercommon.LabelKeyClusterCredentialType])
}

func TestContext(t *testing.T) {
	t.Run("TestClusterNameInContext", func(t *testing.T) {
		ctx := context.Background()
		require.Equal(t, "", ClusterNameInContext(ctx))
		ctx = ContextWithClusterName(ctx, "my-cluster")
		require.Equal(t, "my-cluster", ClusterNameInContext(ctx))
	})

	t.Run("TestContextWithClusterName", func(t *testing.T) {
		ctx := context.Background()
		ctx = ContextWithClusterName(ctx, "my-cluster")
		require.Equal(t, "my-cluster", ClusterNameInContext(ctx))
	})

	t.Run("TestContextInLocalCluster", func(t *testing.T) {
		ctx := context.Background()
		ctx = ContextInLocalCluster(ctx)
		require.Equal(t, ClusterLocalName, ClusterNameInContext(ctx))
	})
}

func TestResourcesWithClusterName(t *testing.T) {
	testCases := []struct {
		name        string
		clusterName string
		objs        []*unstructured.Unstructured
		expected    []*unstructured.Unstructured
	}{
		{
			name:        "Empty slice",
			clusterName: "my-cluster",
			objs:        []*unstructured.Unstructured{},
			expected:    nil,
		},
		{
			name:        "Nil object",
			clusterName: "my-cluster",
			objs:        []*unstructured.Unstructured{nil},
			expected:    nil,
		},
		{
			name:        "Object without cluster name label",
			clusterName: "my-cluster",
			objs:        []*unstructured.Unstructured{{Object: map[string]interface{}{}}},
			expected: []*unstructured.Unstructured{{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							oam.LabelAppCluster: "my-cluster",
						},
					},
				},
			}},
		},
		{
			name:        "Object with existing cluster name label",
			clusterName: "my-cluster",
			objs: []*unstructured.Unstructured{{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							oam.LabelAppCluster: "other-cluster",
						},
					},
				},
			}},
			expected: []*unstructured.Unstructured{{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							oam.LabelAppCluster: "other-cluster",
						},
					},
				},
			}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ResourcesWithClusterName(tc.clusterName, tc.objs...)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestGetClusterGatewayService(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()
	apiregistrationv1.AddToScheme(scheme)

	testCases := []struct {
		name      string
		cli       client.Client
		expectErr bool
		verify    func(t *testing.T, svc *apiregistrationv1.ServiceReference)
	}{
		{
			name:      "APIService not found",
			cli:       fake.NewClientBuilder().WithScheme(scheme).Build(),
			expectErr: true,
		},
		{
			name: "APIService found but no service spec",
			cli: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&apiregistrationv1.APIService{
				ObjectMeta: metav1.ObjectMeta{Name: "v1alpha1.cluster.core.oam.dev"},
			}).Build(),
			expectErr: true,
		},
		{
			name: "APIService found but not available",
			cli: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&apiregistrationv1.APIService{
				ObjectMeta: metav1.ObjectMeta{Name: "v1alpha1.cluster.core.oam.dev"},
				Spec: apiregistrationv1.APIServiceSpec{
					Service: &apiregistrationv1.ServiceReference{
						Name:      "my-service",
						Namespace: "my-namespace",
					},
				},
				Status: apiregistrationv1.APIServiceStatus{
					Conditions: []apiregistrationv1.APIServiceCondition{
						{
							Type:   apiregistrationv1.Available,
							Status: apiregistrationv1.ConditionFalse,
						},
					},
				},
			}).Build(),
			expectErr: true,
			verify: func(t *testing.T, svc *apiregistrationv1.ServiceReference) {
				require.NotNil(t, svc)
				require.Equal(t, "my-service", svc.Name)
			},
		},
		{
			name: "APIService found and available",
			cli: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&apiregistrationv1.APIService{
				ObjectMeta: metav1.ObjectMeta{Name: "v1alpha1.cluster.core.oam.dev"},
				Spec: apiregistrationv1.APIServiceSpec{
					Service: &apiregistrationv1.ServiceReference{
						Name:      "my-service",
						Namespace: "my-namespace",
					},
				},
				Status: apiregistrationv1.APIServiceStatus{
					Conditions: []apiregistrationv1.APIServiceCondition{
						{
							Type:   apiregistrationv1.Available,
							Status: apiregistrationv1.ConditionTrue,
						},
					},
				},
			}).Build(),
			verify: func(t *testing.T, svc *apiregistrationv1.ServiceReference) {
				require.NotNil(t, svc)
				require.Equal(t, "my-service", svc.Name)
			},
		},
		{
			name: "Client Get error",
			cli: &mockClient{
				Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
				getErr: errors.New("client error"),
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := GetClusterGatewayService(ctx, tc.cli)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if tc.verify != nil {
				tc.verify(t, svc)
			}
		})
	}
}

func TestListExistingClusterSecrets(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()
	ClusterGatewaySecretNamespace = "vela-system"

	testCases := []struct {
		name      string
		cli       client.Client
		expectErr bool
		verify    func(t *testing.T, secrets []v1.Secret)
	}{
		{
			name: "No secrets exist",
			cli:  fake.NewClientBuilder().WithScheme(scheme).Build(),
			verify: func(t *testing.T, secrets []v1.Secret) {
				require.Empty(t, secrets)
			},
		},
		{
			name: "Secrets exist, but none have the required label",
			cli: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-label",
					Namespace: ClusterGatewaySecretNamespace,
				},
			}).Build(),
			verify: func(t *testing.T, secrets []v1.Secret) {
				require.Empty(t, secrets)
			},
		},
		{
			name: "Secrets exist with the required label",
			cli: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "with-label",
					Namespace: ClusterGatewaySecretNamespace,
					Labels:    map[string]string{clustercommon.LabelKeyClusterCredentialType: string(v1alpha1.CredentialTypeX509Certificate)},
				},
			}).Build(),
			verify: func(t *testing.T, secrets []v1.Secret) {
				require.Len(t, secrets, 1)
				require.Equal(t, "with-label", secrets[0].Name)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			secrets, err := ListExistingClusterSecrets(ctx, tc.cli)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if tc.verify != nil {
				tc.verify(t, secrets)
			}
		})
	}
}
