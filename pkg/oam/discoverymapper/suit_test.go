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

package discoverymapper

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var scheme = runtime.NewScheme()

func TestMapper(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Test Mapper Suite")
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		UseExistingCluster:       pointer.Bool(false),
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	Expect(crdv1.AddToScheme(scheme)).Should(BeNil())
	// +kubebuilder:scaffold:scheme
	By("Create the k8s client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Mapper discovery resources", func() {

	It("discovery built-in k8s resource", func() {
		dism, err := New(cfg)
		Expect(err).Should(BeNil())
		mapper, err := dism.GetMapper()
		Expect(err).Should(BeNil())
		mapping, err := mapper.RESTMapping(schema.GroupKind{Group: "apps", Kind: "Deployment"}, "v1")
		Expect(err).Should(BeNil())
		Expect(mapping.Resource).Should(Equal(schema.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		}))
	})

	It("discovery CRD", func() {

		By("Check built-in resource")
		dism, err := New(cfg)
		Expect(err).Should(BeNil())
		mapper, err := dism.GetMapper()
		Expect(err).Should(BeNil())
		var mapping *meta.RESTMapping
		mapping, err = mapper.RESTMapping(schema.GroupKind{Group: "", Kind: "Pod"}, "v1")
		Expect(err).Should(BeNil())
		Expect(mapping.Resource).Should(Equal(schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "pods",
		}))

		By("CRD should be discovered after refresh")
		crd := crdv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foos.example.com",
				Labels: map[string]string{"crd": "dependency"},
			},
			Spec: crdv1.CustomResourceDefinitionSpec{
				Group: "example.com",
				Names: crdv1.CustomResourceDefinitionNames{
					Kind:   "Foo",
					Plural: "foos",
				},
				Versions: []crdv1.CustomResourceDefinitionVersion{{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &crdv1.CustomResourceValidation{
						OpenAPIV3Schema: &crdv1.JSONSchemaProps{
							Type: "object",
						}},
				}, {
					Name:   "v1beta1",
					Served: true,
					Schema: &crdv1.CustomResourceValidation{
						OpenAPIV3Schema: &crdv1.JSONSchemaProps{
							Type: "object",
						}},
				}},
				Scope: crdv1.NamespaceScoped,
			},
		}
		Expect(k8sClient.Create(context.Background(), &crd)).Should(BeNil())
		updatedCrdObj := crdv1.CustomResourceDefinition{}
		Eventually(func() bool {
			if err := k8sClient.Get(context.Background(),
				client.ObjectKey{Name: "foos.example.com"}, &updatedCrdObj); err != nil {
				return false
			}
			return len(updatedCrdObj.Spec.Versions) == 2
		}, 3*time.Second, time.Second).Should(BeTrue())

		Eventually(func() error {
			mapping, err = dism.RESTMapping(schema.GroupKind{Group: "example.com", Kind: "Foo"}, "v1")
			return err
		}, time.Second*2, time.Millisecond*300).Should(BeNil())
		Expect(mapping.Resource).Should(Equal(schema.GroupVersionResource{
			Group:    "example.com",
			Version:  "v1",
			Resource: "foos",
		}))

		var kinds []schema.GroupVersionKind
		Eventually(func() []schema.GroupVersionKind {
			kinds, _ = dism.KindsFor(schema.GroupVersionResource{Group: "example.com", Version: "", Resource: "foos"})
			return kinds
		}, time.Second*60, time.Second*3).Should(Equal([]schema.GroupVersionKind{
			{Group: "example.com", Version: "v1", Kind: "Foo"},
			{Group: "example.com", Version: "v1beta1", Kind: "Foo"},
		}))
		kinds, err = dism.KindsFor(schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "foos"})
		Expect(err).Should(BeNil())
		Expect(kinds).Should(Equal([]schema.GroupVersionKind{{Group: "example.com", Version: "v1", Kind: "Foo"}}))
	})

	It("get GVK from k8s resource", func() {
		dism, err := New(cfg)
		Expect(err).Should(BeNil())

		By("Test Pod")
		podAPIVersion, podKind := "v1", "Pod"
		podGV, err := schema.ParseGroupVersion(podAPIVersion)
		Expect(err).Should(BeNil())
		podGVR, err := dism.ResourcesFor(podGV.WithKind(podKind))
		Expect(err).Should(BeNil())
		Expect(podGVR).Should(Equal(schema.GroupVersionResource{
			Version:  "v1",
			Resource: "pods",
		}))

		By("Test Deployment")
		deploymentAPIVersion, deploymentKind := "apps/v1", "Deployment"
		deploymentGV, err := schema.ParseGroupVersion(deploymentAPIVersion)
		Expect(err).Should(BeNil())
		deploymentGVR, err := dism.ResourcesFor(deploymentGV.WithKind(deploymentKind))
		Expect(err).Should(BeNil())
		Expect(deploymentGVR).Should(Equal(schema.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		}))

		By("Test CronJob")
		cronJobAPIVersion, cronJobKind := "batch/v1", "Job"
		cronJobGV, err := schema.ParseGroupVersion(cronJobAPIVersion)
		Expect(err).Should(BeNil())
		cronJobGVR, err := dism.ResourcesFor(cronJobGV.WithKind(cronJobKind))
		Expect(err).Should(BeNil())
		Expect(cronJobGVR).Should(Equal(schema.GroupVersionResource{
			Group:    "batch",
			Version:  "v1",
			Resource: "jobs",
		}))

		By("Test Invalid GVK")
		apiVersion, kind := "apps/v1", "Job"
		gv, err := schema.ParseGroupVersion(apiVersion)
		Expect(err).Should(BeNil())
		_, err = dism.ResourcesFor(gv.WithKind(kind))
		Expect(err).Should(HaveOccurred())
	})

	It("check API resource scope", func() {
		dism, err := New(cfg)
		Expect(err).Should(BeNil())

		var (
			clusterCRKind   = "ImClusterScope"
			namespaceCRKind = "ImNamespaceScope"
		)

		By("Register a cluster-scoped CRD")
		clusterScopeCRD := crdv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "imclusterscopes.example.com",
			},
			Spec: crdv1.CustomResourceDefinitionSpec{
				Scope: crdv1.ClusterScoped,
				Group: "example.com",
				Names: crdv1.CustomResourceDefinitionNames{
					Kind:   clusterCRKind,
					Plural: "imclusterscopes",
				},
				Versions: []crdv1.CustomResourceDefinitionVersion{{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &crdv1.CustomResourceValidation{
						OpenAPIV3Schema: &crdv1.JSONSchemaProps{
							Type: "object",
						}},
				}},
			},
		}
		Expect(k8sClient.Create(context.Background(), &clusterScopeCRD)).Should(BeNil())

		By("Register a namespace-scoped CRD")
		namespaceScopeCRD := crdv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "imnamespacescopes.example.com",
			},
			Spec: crdv1.CustomResourceDefinitionSpec{
				Scope: crdv1.NamespaceScoped,
				Group: "example.com",
				Names: crdv1.CustomResourceDefinitionNames{
					Kind:   namespaceCRKind,
					Plural: "imnamespacescopes",
				},
				Versions: []crdv1.CustomResourceDefinitionVersion{{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &crdv1.CustomResourceValidation{
						OpenAPIV3Schema: &crdv1.JSONSchemaProps{
							Type: "object",
						}},
				}},
			},
		}
		Expect(k8sClient.Create(context.Background(), &namespaceScopeCRD)).Should(BeNil())

		By("Verify checking built-in cluster-scoped resource")
		clusterBuiltInRsc := schema.GroupKind{
			Group: "",
			Kind:  "PersistentVolume",
		}
		isNamespaced, err := IsNamespacedScope(dism, clusterBuiltInRsc)
		Expect(err).Should(BeNil())
		Expect(isNamespaced).Should(BeFalse())

		By("Verify checking built-in namespace-scoped resource")
		namespaceBuiltInRsc := schema.GroupKind{
			Group: "apps",
			Kind:  "Deployment",
		}
		isNamespaced, err = IsNamespacedScope(dism, namespaceBuiltInRsc)
		Expect(err).Should(BeNil())
		Expect(isNamespaced).Should(BeTrue())

		By("Verify checking cluster-scoped custom resource")
		clusterCR := schema.GroupKind{
			Group: "example.com",
			Kind:  clusterCRKind,
		}
		By("Wait for refreshing DiscoveryMapper")
		Eventually(func() error {
			isNamespaced, err = IsNamespacedScope(dism, clusterCR)
			return err
		}, time.Second*2, time.Millisecond*300).Should(BeNil())
		Expect(isNamespaced).Should(BeFalse())

		By("Verify checking namespace-scoped custom resource")
		namespaceCR := schema.GroupKind{
			Group: "example.com",
			Kind:  namespaceCRKind,
		}
		By("Wait for refreshing DiscoveryMapper")
		Eventually(func() error {
			isNamespaced, err = IsNamespacedScope(dism, namespaceCR)
			return err
		}, time.Second*2, time.Millisecond*300).Should(BeNil())
		Expect(isNamespaced).Should(BeTrue())

		By("Cannot check an unknown resource")
		unknownCR := schema.GroupKind{
			Group: "unknow.com",
			Kind:  "Unknown",
		}
		_, err = IsNamespacedScope(dism, unknownCR)
		Expect(err).ShouldNot(BeNil())
	})
})
