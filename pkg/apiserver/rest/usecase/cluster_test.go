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

package usecase

import (
	"context"
	"time"

	prismclusterv1alpha1 "github.com/kubevela/prism/pkg/apis/cluster/v1alpha1"
	clustergatewayv1alpha1 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustergatewaycommon "github.com/oam-dev/cluster-gateway/pkg/common"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	pkgutil "github.com/oam-dev/kubevela/pkg/utils"
)

var _ = Describe("Test cluster usecase function", func() {
	var (
		ds    datastore.DataStore
		cache *utils.MemoryCacheStore
		ctx   context.Context
		err   error
	)

	BeforeEach(func() {
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: "cluster-test-kubevela-" + pkgutil.RandomString(4)})
		Expect(err).Should(Succeed())
		cache = utils.NewMemoryCacheStore(context.Background())
		ctx = context.Background()

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: prismclusterv1alpha1.StorageNamespace}}
		Expect(k8sClient.Create(ctx, ns)).Should(SatisfyAny(Succeed(), util.AlreadyExistMatcher{}))
	})

	createClusterSecret := func(name, alias string) error {
		secret := &corev1.Secret{}
		secret.Name = name
		secret.Namespace = prismclusterv1alpha1.StorageNamespace
		secret.SetAnnotations(map[string]string{prismclusterv1alpha1.AnnotationClusterAlias: alias})
		secret.SetLabels(map[string]string{clustergatewaycommon.LabelKeyClusterCredentialType: string(clustergatewayv1alpha1.CredentialTypeX509Certificate)})
		time.Sleep(time.Second)
		return k8sClient.Create(ctx, secret)
	}

	AfterEach(func() {
		secrets := &corev1.SecretList{}
		Expect(k8sClient.List(ctx, secrets, client.InNamespace(prismclusterv1alpha1.StorageNamespace), client.HasLabels{clustergatewaycommon.LabelKeyClusterCredentialType})).Should(Succeed())
		for _, secret := range secrets.Items {
			Expect(k8sClient.Delete(ctx, secret.DeepCopy())).Should(Succeed())
		}
	})

	It("Test get kube cluster", func() {
		usecase := clusterUsecaseImpl{
			ds:        ds,
			caches:    cache,
			k8sClient: k8sClient,
		}
		Expect(ds.Add(ctx, &model.Cluster{Name: "first", Alias: "first-alias", Icon: "first-icon"})).Should(Succeed())
		resp, err := usecase.GetKubeCluster(ctx, "first")
		Expect(err).Should(Succeed())
		Expect(resp.Alias).Should(Equal("first-alias"))
		Expect(resp.Icon).Should(Equal("first-icon"))
		_, err = usecase.GetKubeCluster(ctx, "prism-cluster")
		Expect(err).Should(Equal(bcode.ErrClusterNotFoundInDataStore))
		Expect(createClusterSecret("prism-cluster", "prism-alias")).Should(Succeed())
		resp, err = usecase.GetKubeCluster(ctx, "prism-cluster")
		Expect(err).Should(Succeed())
		Expect(resp.Alias).Should(Equal("prism-alias"))
		_, err = usecase.GetKubeCluster(ctx, "non-exist-cluster")
		Expect(err).Should(Equal(bcode.ErrClusterNotFoundInDataStore))
	})

	It("Test list kube clusters", func() {
		usecase := clusterUsecaseImpl{
			ds:        ds,
			caches:    cache,
			k8sClient: k8sClient,
		}
		Expect(createClusterSecret("prism-cluster1", "prism-alias1")).Should(Succeed())
		Expect(ds.Add(ctx, &model.Cluster{Name: "prism-cluster1", Alias: "prism-alias1", Icon: "prism-icon1"})).Should(Succeed())
		Expect(ds.Add(ctx, &model.Cluster{Name: "local"})).Should(Succeed())
		resp, err := usecase.ListKubeClusters(ctx, "", 1, 5)
		Expect(err).Should(Succeed())
		Expect(len(resp.Clusters)).Should(Equal(2))
		Expect(resp.Clusters[0].Name).Should(Equal("local"))
		Expect(resp.Clusters[1].Name).Should(Equal("prism-cluster1"))
		Expect(createClusterSecret("prism-cluster2", "prism-alias2")).Should(Succeed())
		Expect(createClusterSecret("cluster3", "prism-alias3")).Should(Succeed())
		resp, err = usecase.ListKubeClusters(ctx, "", 1, 5)
		Expect(err).Should(Succeed())
		Expect(len(resp.Clusters)).Should(Equal(4))
		Expect(resp.Clusters[3].Icon).Should(Equal("prism-icon1"))
		resp, err = usecase.ListKubeClusters(ctx, "prism-cluster", 1, 5)
		Expect(err).Should(Succeed())
		Expect(len(resp.Clusters)).Should(Equal(2))
		resp, err = usecase.ListKubeClusters(ctx, "", 2, 3)
		Expect(err).Should(Succeed())
		Expect(len(resp.Clusters)).Should(Equal(1))
		resp, err = usecase.ListKubeClusters(ctx, "", 3, 3)
		Expect(err).Should(Succeed())
		Expect(len(resp.Clusters)).Should(Equal(0))
	})
})

//type fakePrismClusterClient struct {
//	client.Client
//}
//
//func (c fakePrismClusterClient) newClusterSecret(name, alias string) *corev1.Secret {
//	secret := &corev1.Secret{}
//	secret.Name = name
//	secret.Namespace = prismclusterv1alpha1.StorageNamespace
//	secret.SetAnnotations(map[string]string{prismclusterv1alpha1.AnnotationClusterAlias: alias})
//	secret.SetLabels(map[string]string{clustergatewaycommon.LabelKeyClusterCredentialType: string(clustergatewayv1alpha1.CredentialTypeX509Certificate)})
//	return secret
//}
//
//func (c fakePrismClusterClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
//	if cluster, isPrismCluster := obj.(*corev1.Secret); isPrismCluster && key.Name == "prism-cluster" {
//		c.newClusterSecret("prism-cluster", "prism-alias").DeepCopyInto(cluster)
//		return nil
//	}
//	return c.Client.Get(ctx, key, obj)
//}
//
//func (c fakePrismClusterClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
//	if clusters, isPrismCluster := list.(*corev1.SecretList); isPrismCluster {
//		cluster1 := c.newClusterSecret("prism-cluster1", "prism-alias1")
//		cluster2 := c.newClusterSecret("prism-cluster2", "prism-alias2")
//		cluster3 := c.newClusterSecret("prism-cluster3", "prism-alias3")
//		clusters.Items = []corev1.Secret{*cluster1, *cluster2, *cluster3}
//		return nil
//	}
//	return c.Client.List(ctx, list, opts...)
//}
