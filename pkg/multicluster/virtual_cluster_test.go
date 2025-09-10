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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"

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
					clustercommon.LabelKeyClusterCredentialType: string(v1alpha1.CredentialTypeX509Certificate),
					clustercommon.LabelKeyClusterEndpointType:   string(v1alpha1.ClusterEndpointTypeConst),
					"key": "value",
				},
				Annotations: map[string]string{v1alpha1.AnnotationClusterAlias: "test-alias"},
			},
		})).Should(Succeed())
		Expect(k8sClient.Create(ctx, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-no-label",
				Namespace: ClusterGatewaySecretNamespace,
				Labels: map[string]string{
					clustercommon.LabelKeyClusterCredentialType: string(v1alpha1.CredentialTypeX509Certificate),
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
				Name:        "ocm-cluster",
				Namespace:   ClusterGatewaySecretNamespace,
				Labels:      map[string]string{"key": "value"},
				Annotations: map[string]string{v1alpha1.AnnotationClusterAlias: "ocm-alias"},
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

		By("Test virtual cluster list for clusterNameMapper")
		cli := fakeClient{Client: k8sClient}
		cnm, err := NewClusterNameMapper(ctx, cli)
		Expect(err).Should(Succeed())
		Expect(cnm.GetClusterName("example")).Should(Equal("example (example-alias)"))
		Expect(cnm.GetClusterName("no-alias")).Should(Equal("no-alias"))
		cli.returnBadRequest = true
		_, err = NewClusterNameMapper(ctx, cli)
		Expect(err).Should(Satisfy(errors.IsBadRequest))
		cli.returnBadRequest = false
		cli.virtualClusterNotRegistered = true
		cnm, err = NewClusterNameMapper(ctx, cli)
		Expect(err).Should(Succeed())
		Expect(cnm.GetClusterName("example")).Should(Equal("example"))
		Expect(cnm.GetClusterName("test-cluster")).Should(Equal("test-cluster (test-alias)"))
		Expect(cnm.GetClusterName("ocm-cluster")).Should(Equal("ocm-cluster (ocm-alias)"))
		cli.returnBadRequest = true
		cli.virtualClusterNotRegistered = true
		_, err = NewClusterNameMapper(ctx, cli)
		Expect(err).ShouldNot(Succeed())
	})
	It("Test Cluster Version Get and Set", func() {
		ClusterGatewaySecretNamespace = "vela-system2"
		ctx := context.Background()
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ClusterGatewaySecretNamespace}})).Should(Succeed())
		cv, err := GetVersionInfoFromCluster(ctx, "local", cfg)
		Expect(err).Should(BeNil())
		Expect(cv.Minor).Should(Not(BeEquivalentTo("")))
		Expect(cv.Major).Should(BeEquivalentTo("1"))
	})

	It("Test virtual cluster helpers", func() {
		By("Test FullName")
		vcWithAlias := &VirtualCluster{Name: "test", Alias: "alias"}
		Expect(vcWithAlias.FullName()).To(Equal("test (alias)"))
		vcWithoutAlias := &VirtualCluster{Name: "test"}
		Expect(vcWithoutAlias.FullName()).To(Equal("test"))

		By("Test get/set cluster alias")
		secret := &v1.Secret{}
		setClusterAlias(secret, "my-alias")
		Expect(getClusterAlias(secret)).To(Equal("my-alias"))
		annots := secret.GetAnnotations()
		Expect(annots).ToNot(BeNil())
		Expect(annots[v1alpha1.AnnotationClusterAlias]).To(Equal("my-alias"))

		By("Test NewVirtualClusterFromLocal")
		vc := NewVirtualClusterFromLocal()
		Expect(vc.Name).To(Equal(ClusterLocalName))
		Expect(vc.Accepted).To(BeTrue())
		Expect(vc.EndPoint).To(Equal(types.ClusterBlankEndpoint))

		By("Test MatchVirtualClusterLabels")
		ClusterGatewaySecretNamespace = "vela-system" // as set in other test
		labels := MatchVirtualClusterLabels{"key": "val"}
		opts := &client.ListOptions{}
		labels.ApplyToList(opts)
		Expect(opts.Namespace).To(Equal(ClusterGatewaySecretNamespace))
		Expect(opts.LabelSelector).NotTo(BeNil())
		Expect(opts.LabelSelector.String()).To(ContainSubstring("key=val"))
		Expect(opts.LabelSelector.String()).To(ContainSubstring(clustercommon.LabelKeyClusterCredentialType))

		delOpts := &client.DeleteAllOfOptions{}
		labels.ApplyToDeleteAllOf(delOpts)
		Expect(delOpts.ListOptions.Namespace).To(Equal(ClusterGatewaySecretNamespace))
		Expect(delOpts.ListOptions.LabelSelector).NotTo(BeNil())
		Expect(delOpts.ListOptions.LabelSelector.String()).To(ContainSubstring("key=val"))

		By("Test get/set cluster version")
		versionedSecret := &v1.Secret{}
		cv := types.ClusterVersion{Major: "1", Minor: "20", GitVersion: "v1.20.0"}
		setClusterVersion(versionedSecret, cv)

		newCV, err := getClusterVersionFromObject(versionedSecret)
		Expect(err).To(Succeed())
		Expect(newCV).To(Equal(cv))

		versionedSecret.Annotations = nil
		_, err = getClusterVersionFromObject(versionedSecret)
		Expect(err).ToNot(Succeed())

		secretWithEmptyAnnotation := &v1.Secret{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}}}
		_, err = getClusterVersionFromObject(secretWithEmptyAnnotation)
		Expect(err).ToNot(Succeed())
	})

	It("Test GetVersionInfoFromObject", func() {
		ClusterGatewaySecretNamespace = "vela-system3"
		ctx := context.Background()
		ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ClusterGatewaySecretNamespace}}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, ns)).Should(Succeed())
		}()

		By("Setup a secret with version info")
		cv := types.ClusterVersion{Major: "1", Minor: "21", GitVersion: "v1.21.0"}
		cvJSON, err := json.Marshal(cv)
		Expect(err).To(Succeed())
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-with-version",
				Namespace: ClusterGatewaySecretNamespace,
				Labels:    map[string]string{clustercommon.LabelKeyClusterCredentialType: "X509"},
				Annotations: map[string]string{
					types.AnnotationClusterVersion: string(cvJSON),
				},
			},
		}
		Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

		By("Test getting version from the secret")
		retrievedCV := GetVersionInfoFromObject(ctx, k8sClient, "cluster-with-version")
		Expect(retrievedCV).To(Equal(cv))

		By("Test with a cluster that doesn't exist, should fallback to control plane version")
		originalCPVersion := types.ControlPlaneClusterVersion
		types.ControlPlaneClusterVersion = types.ClusterVersion{GitVersion: "v1.22.0"}
		defer func() { types.ControlPlaneClusterVersion = originalCPVersion }()
		retrievedCV = GetVersionInfoFromObject(ctx, k8sClient, "non-existent-cluster")
		Expect(retrievedCV).To(Equal(types.ControlPlaneClusterVersion))

		By("Test with a secret without version info, should fallback")
		secretNoVersion := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-no-version",
				Namespace: ClusterGatewaySecretNamespace,
				Labels:    map[string]string{clustercommon.LabelKeyClusterCredentialType: "X509"},
			},
		}
		Expect(k8sClient.Create(ctx, secretNoVersion)).Should(Succeed())
		retrievedCV = GetVersionInfoFromObject(ctx, k8sClient, "cluster-no-version")
		Expect(retrievedCV).To(Equal(types.ControlPlaneClusterVersion))
	})
})

type fakeClient struct {
	client.Client
	returnBadRequest            bool
	virtualClusterNotRegistered bool
}

func (c fakeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if !c.virtualClusterNotRegistered && c.returnBadRequest {
		return errors.NewBadRequest("")
	}
	if src, ok := list.(*v1alpha1.VirtualClusterList); ok {
		if c.virtualClusterNotRegistered {
			return runtime.NewNotRegisteredErrForKind("", schema.GroupVersionKind{})
		}
		objs := &v1alpha1.VirtualClusterList{Items: []v1alpha1.VirtualCluster{{
			ObjectMeta: metav1.ObjectMeta{Name: "example"},
			Spec:       v1alpha1.VirtualClusterSpec{Alias: "example-alias"},
		}, {
			ObjectMeta: metav1.ObjectMeta{Name: "no-alias"},
		}}}
		objs.DeepCopyInto(src)
		return nil
	}
	if c.returnBadRequest {
		return errors.NewBadRequest("")
	}
	return c.Client.List(ctx, list, opts...)
}
