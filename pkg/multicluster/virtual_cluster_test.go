/*
Copyright 2020-2022 The KubeVela Authors.

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

	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/oam-dev/kubevela/apis/types"
)

var _ = Describe("Test Virtual Cluster", func() {

	It("Test Virtual Cluster", func() {
		ClusterGatewaySecretNamespace = "vela-system"
		ctx := context.Background()
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ClusterGatewaySecretNamespace}})).Should(Succeed())

		By("Initialize Secrets")
		Expect(k8sClient.Create(ctx, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: ClusterGatewaySecretNamespace,
				Labels: map[string]string{
					v1alpha1.LabelKeyClusterCredentialType: string(v1alpha1.CredentialTypeX509Certificate),
					v1alpha1.LabelKeyClusterEndpointType:   v1alpha1.ClusterEndpointTypeConst,
					"key":                                  "value",
				},
			},
		})).Should(Succeed())
		Expect(k8sClient.Create(ctx, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-no-label",
				Namespace: ClusterGatewaySecretNamespace,
				Labels: map[string]string{
					v1alpha1.LabelKeyClusterCredentialType: string(v1alpha1.CredentialTypeX509Certificate),
				},
			},
		})).Should(Succeed())
		Expect(k8sClient.Create(ctx, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-invalid",
				Namespace: ClusterGatewaySecretNamespace,
			},
		})).Should(Succeed())

		By("Test Get Virtual Cluster From Cluster Secret")
		vc, err := GetVirtualCluster(ctx, k8sClient, "test-cluster")
		Expect(err).Should(Succeed())
		Expect(vc.Type).Should(Equal(v1alpha1.CredentialTypeX509Certificate))
		Expect(vc.Labels["key"]).Should(Equal("value"))

		_, err = GetVirtualCluster(ctx, k8sClient, "cluster-not-found")
		Expect(err).ShouldNot(Succeed())
		Expect(err.Error()).Should(ContainSubstring("no such cluster"))

		_, err = GetVirtualCluster(ctx, k8sClient, "cluster-invalid")
		Expect(err).ShouldNot(Succeed())
		Expect(err.Error()).Should(ContainSubstring("not a valid cluster"))

		By("Add OCM ManagedCluster")
		Expect(k8sClient.Create(ctx, &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ocm-bad-cluster",
				Namespace: ClusterGatewaySecretNamespace,
			},
		})).Should(Succeed())
		Expect(k8sClient.Create(ctx, &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ocm-cluster",
				Namespace: ClusterGatewaySecretNamespace,
				Labels:    map[string]string{"key": "value"},
			},
			Spec: clusterv1.ManagedClusterSpec{
				ManagedClusterClientConfigs: []clusterv1.ClientConfig{{URL: "test-url"}},
			},
		})).Should(Succeed())

		By("Test Get Virtual Cluster From OCM")

		_, err = GetVirtualCluster(ctx, k8sClient, "ocm-bad-cluster")
		Expect(err).ShouldNot(Succeed())
		Expect(err.Error()).Should(ContainSubstring("has no client config"))

		vc, err = GetVirtualCluster(ctx, k8sClient, "ocm-cluster")
		Expect(err).Should(Succeed())
		Expect(vc.Type).Should(Equal(types.CredentialTypeOCMManagedCluster))

		By("Test List Virtual Clusters")

		vcs, err := ListVirtualClusters(ctx, k8sClient)
		Expect(err).Should(Succeed())
		Expect(len(vcs)).Should(Equal(4))

		vcs, err = FindVirtualClustersByLabels(ctx, k8sClient, map[string]string{"key": "value"})
		Expect(err).Should(Succeed())
		Expect(len(vcs)).Should(Equal(2))
	})

})
