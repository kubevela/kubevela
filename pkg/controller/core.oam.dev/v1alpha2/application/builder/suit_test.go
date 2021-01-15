/*
Copyright 2020 The KubeVela Authors.

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

package builder

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/parser"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var testScheme = runtime.NewScheme()

func TestAPIs(t *testing.T) {

	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.LoggerTo(GinkgoWriter, true))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("../../../../..", "charts", "vela-core", "crds")},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = v1alpha2.SchemeBuilder.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	err = scheme.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Test Application Controller", func() {
	// TestApp is test data
	var TestApp = &parser.Appfile{
		Name: "test",
		Services: []*parser.Workload{
			{
				Name: "myweb",
				Type: "worker",
				Params: map[string]interface{}{
					"image":  "busybox",
					"cmd":    []interface{}{"sleep", "1000"},
					"config": "myconfig",
				},
				Scopes: []parser.Scope{
					{Name: "test-scope", GVK: schema.GroupVersionKind{
						Group:   "core.oam.dev",
						Version: "v1alpha2",
						Kind:    "HealthScope",
					}},
				},
				Template: `
      output: {
        apiVersion: "apps/v1"
      	kind:       "Deployment"
      	spec: {
      		selector: matchLabels: {
      			"app.oam.dev/component": context.name
      		}
      
      		template: {
      			metadata: labels: {
      				"app.oam.dev/component": context.name
      			}
      
      			spec: {
      				containers: [{
      					name:  context.name
      					image: parameter.image
      
      					if parameter["cmd"] != _|_ {
      						command: parameter.cmd
      					}
      					if context["config"] != _|_ {
      						env: context.config
      					}
      				}]
      			}
      		}
      
      		selector:
      			matchLabels:
      				"app.oam.dev/component": context.name
      	}
      }
      
      parameter: {
      	// +usage=Which image would you like to use for your service
      	// +short=i
      	image: string
      
      	cmd?: [...string]
      }`,
				Traits: []*parser.Trait{
					{
						Name: "scaler",
						Params: map[string]interface{}{
							"replicas": float64(10),
						},
						Template: `
      output: {
      	apiVersion: "core.oam.dev/v1alpha2"
      	kind:       "ManualScalerTrait"
      	spec: {
      		replicaCount: parameter.replicas
      	}
      }
      parameter: {
      	//+short=r
      	replicas: *1 | int
      }
`,
					},
				},
			},
		},
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "kubevela-test-myweb-myconfig", Namespace: "default"},
		Data:       map[string]string{"c1": "v1", "c2": "v2"},
	}

	BeforeEach(func() {})
	AfterEach(func() {})

	It("app-without-trait will only create workload", func() {
		Expect(k8sClient.Create(context.Background(), cm.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		ac, components, err := Build("default", TestApp, k8sClient)
		Expect(err).To(BeNil())
		expectAppConfig := &v1alpha2.ApplicationConfiguration{
			TypeMeta: v1.TypeMeta{
				Kind:       "ApplicationConfiguration",
				APIVersion: "core.oam.dev/v1alpha2",
			}, ObjectMeta: v1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
				Labels:    map[string]string{"application.oam.dev": "test"},
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{
					{
						ComponentName: "myweb",
						Scopes: []v1alpha2.ComponentScope{
							{
								ScopeReference: v1alpha1.TypedReference{
									APIVersion: "core.oam.dev/v1alpha2",
									Kind:       "HealthScope",
									Name:       "test-scope",
								},
							},
						},
						Traits: []v1alpha2.ComponentTrait{
							{
								Trait: runtime.RawExtension{
									Object: &unstructured.Unstructured{
										Object: map[string]interface{}{
											"apiVersion": "core.oam.dev/v1alpha2",
											"kind":       "ManualScalerTrait",
											"metadata": map[string]interface{}{
												"labels": map[string]interface{}{
													"trait.oam.dev/type": "scaler",
												},
											},
											"spec": map[string]interface{}{"replicaCount": int64(10)},
										},
									},
								}},
						},
					},
				},
			},
		}
		Expect(ac).To(BeEquivalentTo(expectAppConfig))

		expectComponent := &v1alpha2.Component{
			TypeMeta: v1.TypeMeta{
				Kind:       "Component",
				APIVersion: "core.oam.dev/v1alpha2",
			}, ObjectMeta: v1.ObjectMeta{
				Name:      "myweb",
				Namespace: "default",
				Labels:    map[string]string{"application.oam.dev": "test"},
			}, Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"workload.oam.dev/type": "worker",
								},
							},
							"spec": map[string]interface{}{
								"selector": map[string]interface{}{
									"matchLabels": map[string]interface{}{
										"app.oam.dev/component": "myweb"}},
								"template": map[string]interface{}{
									"metadata": map[string]interface{}{"labels": map[string]interface{}{"app.oam.dev/component": "myweb"}},
									"spec": map[string]interface{}{
										"containers": []interface{}{
											map[string]interface{}{
												"command": []interface{}{"sleep", "1000"},
												"image":   "busybox",
												"name":    "myweb",
												"env": []interface{}{
													map[string]interface{}{"name": "c1", "value": "v1"},
													map[string]interface{}{"name": "c2", "value": "v2"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		By(" built components' length must be 1")
		Expect(len(components)).To(BeEquivalentTo(1))
		Expect(components[0].ObjectMeta).To(BeEquivalentTo(expectComponent.ObjectMeta))
		Expect(components[0].TypeMeta).To(BeEquivalentTo(expectComponent.TypeMeta))
		Expect(cmp.Diff(components[0].Spec.Workload.Object, expectComponent.Spec.Workload.Object)).To(BeEquivalentTo(""))
	})

})
