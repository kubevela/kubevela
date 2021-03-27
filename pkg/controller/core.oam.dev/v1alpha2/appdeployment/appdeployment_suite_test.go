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

package appdeployment

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var comp1Bytes = []byte(`
{
   "apiVersion": "core.oam.dev/v1alpha2",
   "kind": "Component",
   "metadata": {
      "labels": {
         "app.oam.dev/name": "example-app"
      },
      "name": "testsvc",
      "namespace": "default"
   },
   "spec": {
      "workload": {
         "apiVersion": "apps/v1",
         "kind": "Deployment",
         "metadata": {
            "labels": {
               "app.oam.dev/component": "testsvc",
               "app.oam.dev/name": "example-app",
               "workload.oam.dev/type": "webservice"
            }
         },
         "spec": {
            "selector": {
               "matchLabels": {
                  "app.oam.dev/component": "testsvc"
               }
            },
            "template": {
               "metadata": {
                  "labels": {
                     "app.oam.dev/component": "testsvc"
                  }
               },
               "spec": {
                  "containers": [
                     {
                        "image": "crccheck/hello-world",
                        "name": "testsvc",
                        "ports": [
                           {
                              "containerPort": 8000
                           }
                        ]
                     }
                  ]
               }
            }
         }
      }
   }
}
`)

var comp2Bytes = []byte(`
{
   "apiVersion": "core.oam.dev/v1alpha2",
   "kind": "Component",
   "metadata": {
      "labels": {
         "app.oam.dev/name": "example-app"
      },
      "name": "testsvc",
      "namespace": "default"
   },
   "spec": {
      "workload": {
         "apiVersion": "apps/v1",
         "kind": "Deployment",
         "metadata": {
            "labels": {
               "app.oam.dev/component": "testsvc",
               "app.oam.dev/name": "example-app",
               "workload.oam.dev/type": "webservice"
            }
         },
         "spec": {
            "selector": {
               "matchLabels": {
                  "app.oam.dev/component": "testsvc"
               }
            },
            "template": {
               "metadata": {
                  "labels": {
                     "app.oam.dev/component": "testsvc"
                  }
               },
               "spec": {
                  "containers": [
                     {
                        "image": "ngingx",
                        "name": "testsvc",
                        "ports": [
                           {
                              "containerPort": 80
                           }
                        ]
                     }
                  ]
               }
            }
         }
      }
   }
}
`)

var appConfig1Bytes = []byte(`
{
   "apiVersion": "core.oam.dev/v1alpha2",
   "kind": "ApplicationConfiguration",
   "metadata": {
      "annotations": {
         "app.oam.dev/revision-only": "true"
      },
      "labels": {
         "app.oam.dev/name": "example-app"
      },
      "name": "example-app",
      "namespace": "default"
   },
   "spec": {
      "components": [
         {
            "componentName": "testsvc"
         }
      ]
   }
}
`)

var appConfig2Bytes = []byte(`
{
   "apiVersion": "core.oam.dev/v1alpha2",
   "kind": "ApplicationConfiguration",
   "metadata": {
      "annotations": {
         "app.oam.dev/revision-only": "true"
      },
      "labels": {
         "app.oam.dev/name": "example-app"
      },
      "name": "example-app",
      "namespace": "default"
   },
   "spec": {
      "components": [
         {
            "componentName": "testsvc"
         }
      ]
   }
}
`)

var _ = Describe("Test AppDeployment Controller", func() {

	ctx := context.Background()

	var appref = oamcore.Application{ // work around AppRevision validation
		Spec: oamcore.ApplicationSpec{
			Components: []oamcore.ApplicationComponent{},
		},
	}

	BeforeEach(func() {
		By("Setup resources before an integration test")
	})

	AfterEach(func() {
		By("Clean up resources after an integration test")
	})

	It("should update from one version 2 replicas to two versions each with one replica", func() {
		ns := "default"
		// setup AppRevision example-app-v1
		apprev1 := &oamcore.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-app-v1",
				Namespace: ns,
			},
			Spec: oamcore.ApplicationRevisionSpec{
				Application: *appref.DeepCopy(),
				Components: []common.RawComponent{{
					Raw: runtime.RawExtension{Raw: comp1Bytes},
				}},
				ApplicationConfiguration: runtime.RawExtension{Raw: appConfig1Bytes},
			},
		}
		Expect(k8sClient.Create(ctx, apprev1)).Should(BeNil())

		// setup AppRevision example-app-v2
		apprev2 := &oamcore.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-app-v2",
				Namespace: ns,
			},
			Spec: oamcore.ApplicationRevisionSpec{
				Application: *appref.DeepCopy(),
				Components: []common.RawComponent{{
					Raw: runtime.RawExtension{Raw: comp2Bytes},
				}},
				ApplicationConfiguration: runtime.RawExtension{Raw: appConfig2Bytes},
			},
		}
		Expect(k8sClient.Create(ctx, apprev2)).Should(BeNil())

		// apply appDeployment with only v1 with two replicas
		appd := &oamcore.AppDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-appdeploy",
				Namespace: ns,
			},
			Spec: oamcore.AppDeploymentSpec{
				AppRevisions: []oamcore.AppRevision{{
					RevisionName: apprev1.Name,
					Placement: []oamcore.ClusterPlacement{{
						Distribution: oamcore.Distribution{
							Replicas: 2,
						},
					}},
				}},
			},
		}
		testReconcile(ctx, appd)

		checkDeployment(ctx, "testsvc-v1", ns, 2)

		// apply appDeployment with v1/v2 each with one replica

		appd.Spec.AppRevisions = []oamcore.AppRevision{{
			RevisionName: apprev1.Name,
			Placement: []oamcore.ClusterPlacement{{
				Distribution: oamcore.Distribution{
					Replicas: 1,
				},
			}},
		}, {
			RevisionName: apprev2.Name,
			Placement: []oamcore.ClusterPlacement{{
				Distribution: oamcore.Distribution{
					Replicas: 1,
				},
			}},
		}}

		testReconcile(ctx, appd)

		checkDeployment(ctx, "testsvc-v1", ns, 1)
		checkDeployment(ctx, "testsvc-v2", ns, 1)
	})
})

func testReconcile(ctx context.Context, appd *oamcore.AppDeployment) {
	// APIApplicator needs this
	appd.TypeMeta = metav1.TypeMeta{
		APIVersion: oamcore.SchemeGroupVersion.String(),
		Kind:       oamcore.AppDeploymentKind,
	}

	applicator := apply.NewAPIApplicator(k8sClient)
	Expect(applicator.Apply(ctx, appd)).Should(BeNil())
	rKey := apimachinerytypes.NamespacedName{
		Name:      appd.Name,
		Namespace: appd.Namespace,
	}
	_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: rKey})
	Expect(err).Should(BeNil())

	appdKey := client.ObjectKey{
		Name:      appd.Name,
		Namespace: appd.Namespace,
	}
	Expect(k8sClient.Get(ctx, appdKey, appd)).Should(BeNil())
}

func checkDeployment(ctx context.Context, name, ns string, replica int) {
	key := client.ObjectKey{
		Name:      name,
		Namespace: ns,
	}
	deployment := &appsv1.Deployment{}
	Expect(k8sClient.Get(ctx, key, deployment)).Should(BeNil())
	Expect(int(*deployment.Spec.Replicas)).Should(Equal(replica))
}
