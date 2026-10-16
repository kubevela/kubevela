/*
Copyright 2026 The KubeVela Authors.

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
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	velacommon "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/servingcert"
)

const (
	testGatewayNamespace = "vela-system"
	testGatewaySecret    = "kubevela-vela-core-cluster-gateway-tls-v2"
	testGatewayService   = "kubevela-cluster-gateway-service"
)

var testGatewaySvc = servingcert.Service{Namespace: testGatewayNamespace, Name: testGatewayService}

func testGatewayAPIService(caBundle []byte, insecure bool) *apiregistrationv1.APIService {
	return &apiregistrationv1.APIService{
		ObjectMeta: metav1.ObjectMeta{Name: ClusterGatewayAPIServiceName},
		Spec: apiregistrationv1.APIServiceSpec{
			Group:                 "cluster.core.oam.dev",
			Version:               "v1alpha1",
			GroupPriorityMinimum:  2000,
			VersionPriority:       10,
			InsecureSkipTLSVerify: insecure,
			CABundle:              caBundle,
			Service: &apiregistrationv1.ServiceReference{
				Namespace: testGatewayNamespace,
				Name:      testGatewayService,
				Port:      ptr.To[int32](9443),
			},
		},
	}
}

// testJobGatewaySecret is a Secret like the one the old tls-secret-create Job made.
func testJobGatewaySecret(t *testing.T) *corev1.Secret {
	t.Helper()
	data, err := servingcert.Generate(testGatewaySvc.RequiredHosts(), time.Now().Add(-time.Hour), time.Now().Add(365*24*time.Hour))
	require.NoError(t, err)
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testGatewaySecret, Namespace: testGatewayNamespace},
		Data:       data,
	}
}

func getGatewaySecret(t *testing.T, c client.Client) *corev1.Secret {
	t.Helper()
	secret := &corev1.Secret{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: testGatewayNamespace, Name: testGatewaySecret}, secret))
	return secret
}

func getGatewayAPIService(t *testing.T, c client.Client) *apiregistrationv1.APIService {
	t.Helper()
	apiService := &apiregistrationv1.APIService{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: ClusterGatewayAPIServiceName}, apiService))
	return apiService
}

func TestEnsureClusterGatewayTLS(t *testing.T) {
	ctx := context.Background()

	t.Run("generates the Secret for the APIService's Service and sets the caBundle", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(velacommon.Scheme).
			WithObjects(testGatewayAPIService(nil, false)).Build()

		ca, err := EnsureClusterGatewayTLS(ctx, c, testGatewayNamespace, testGatewaySecret)
		require.NoError(t, err)

		secret := getGatewaySecret(t, c)
		require.Equal(t, secret.Data[servingcert.CAKey], ca)
		require.NoError(t, servingcert.Check(secret.Data, testGatewaySvc.Hosts(), time.Now()))
		apiService := getGatewayAPIService(t, c)
		require.Equal(t, ca, apiService.Spec.CABundle)
		require.False(t, apiService.Spec.InsecureSkipTLSVerify)
	})

	t.Run("replaces the Cg== placeholder and turns off insecureSkipTLSVerify", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(velacommon.Scheme).
			WithObjects(testGatewayAPIService([]byte("\n"), true)).Build()

		ca, err := EnsureClusterGatewayTLS(ctx, c, testGatewayNamespace, testGatewaySecret)
		require.NoError(t, err)

		apiService := getGatewayAPIService(t, c)
		require.Equal(t, ca, apiService.Spec.CABundle)
		require.False(t, apiService.Spec.InsecureSkipTLSVerify)
	})

	t.Run("keeps the CA from a Secret made by the old Job", func(t *testing.T) {
		existing := testJobGatewaySecret(t)
		c := fake.NewClientBuilder().WithScheme(velacommon.Scheme).
			WithObjects(testGatewayAPIService(existing.Data[servingcert.CAKey], false), existing.DeepCopy()).Build()

		ca, err := EnsureClusterGatewayTLS(ctx, c, testGatewayNamespace, testGatewaySecret)
		require.NoError(t, err)

		require.Equal(t, existing.Data[servingcert.CAKey], ca)
		require.Equal(t, existing.Data, getGatewaySecret(t, c).Data)
		require.Equal(t, ca, getGatewayAPIService(t, c).Spec.CABundle)
	})

	t.Run("does not update the APIService when it already trusts the CA", func(t *testing.T) {
		existing := testJobGatewaySecret(t)
		updates := 0
		c := fake.NewClientBuilder().WithScheme(velacommon.Scheme).
			WithObjects(testGatewayAPIService(existing.Data[servingcert.CAKey], false), existing).
			WithInterceptorFuncs(interceptor.Funcs{
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					updates++
					return c.Update(ctx, obj, opts...)
				},
			}).Build()

		_, err := EnsureClusterGatewayTLS(ctx, c, testGatewayNamespace, testGatewaySecret)
		require.NoError(t, err)
		require.Zero(t, updates)
	})

	t.Run("fails when the APIService does not exist", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(velacommon.Scheme).Build()

		_, err := EnsureClusterGatewayTLS(ctx, c, testGatewayNamespace, testGatewaySecret)
		require.Error(t, err)
	})

	t.Run("fails when the APIService is served locally", func(t *testing.T) {
		apiService := testGatewayAPIService(nil, false)
		apiService.Spec.Service = nil
		c := fake.NewClientBuilder().WithScheme(velacommon.Scheme).WithObjects(apiService).Build()

		_, err := EnsureClusterGatewayTLS(ctx, c, testGatewayNamespace, testGatewaySecret)
		require.Error(t, err)
	})
}
