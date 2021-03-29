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
	"github.com/stretchr/testify/assert"
	istioapiv1beta1 "istio.io/api/networking/v1beta1"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

var appConfigWithTraitsBytes = []byte(`
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
        "componentName": "testsvc",
        "traits": [
          {
            "trait": {
              "apiVersion": "v1",
              "kind": "Service",
              "spec": {
                "ports": [
                  {
                    "port": 8000,
                    "targetPort": 8000
                  }
                ],
                "selector": {
                  "app.oam.dev/component": "testsvc"
                }
              }
            }
          }
        ]
      }
    ]
  }
}
`)

var appref = oamcore.Application{ // work around AppRevision validation
	Spec: oamcore.ApplicationSpec{
		Components: []oamcore.ApplicationComponent{},
	},
}

var _ = Describe("Test AppDeployment Controller", func() {

	ctx := context.Background()
	ns := "default"

	BeforeEach(func() {
		By("Setup resources before an integration test")
	})

	AfterEach(func() {
		By("Clean up resources after an integration test")
	})

	It("should update from one version 2 replicas to two versions each with one replica", func() {
		apprev1 := setupAppRevision(ctx, "example-app-v1", ns, appConfig1Bytes, comp1Bytes)

		apprev2 := setupAppRevision(ctx, "example-app-v2", ns, appConfig2Bytes, comp2Bytes)

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
		reconcileAppDeployment(ctx, appd)

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

		reconcileAppDeployment(ctx, appd)

		checkDeployment(ctx, "testsvc-v1", ns, 1)
		checkDeployment(ctx, "testsvc-v2", ns, 1)
	})

	It("should apply both workload and trait resources", func() {

		apprev3 := setupAppRevision(ctx, "example-app-v3", ns, appConfigWithTraitsBytes, comp1Bytes)

		appd := &oamcore.AppDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-trait",
				Namespace: ns,
			},
			Spec: oamcore.AppDeploymentSpec{
				AppRevisions: []oamcore.AppRevision{{
					RevisionName: apprev3.Name,
					Placement: []oamcore.ClusterPlacement{{
						Distribution: oamcore.Distribution{
							Replicas: 2,
						},
					}},
				}},
			},
		}
		reconcileAppDeployment(ctx, appd)
		checkDeployment(ctx, "testsvc-v3", ns, 2)
		checkService(ctx, "testsvc-v3", ns)
	})

	It("should apply traffic", func() {

		apprev4 := setupAppRevision(ctx, "example-app-v4", ns, appConfig1Bytes, comp1Bytes)
		apprev5 := setupAppRevision(ctx, "example-app-v5", ns, appConfig2Bytes, comp2Bytes)

		appd := &oamcore.AppDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-traffic",
				Namespace: ns,
			},
			Spec: oamcore.AppDeploymentSpec{
				Traffic: &oamcore.Traffic{
					Hosts:    []string{"example.com"},
					Gateways: []string{"vela-gateway"},
					HTTP: []oamcore.HTTPRule{{
						WeightedTargets: []oamcore.WeightedTarget{{
							RevisionName:  apprev4.Name,
							ComponentName: "testsvc",
							Port:          8000,
							Weight:        50,
						}, {
							RevisionName:  apprev5.Name,
							ComponentName: "testsvc",
							Port:          80,
							Weight:        50,
						}},
					}},
				},
				AppRevisions: []oamcore.AppRevision{{
					RevisionName: apprev4.Name,
					Placement: []oamcore.ClusterPlacement{{
						Distribution: oamcore.Distribution{
							Replicas: 1,
						},
					}},
				}, {
					RevisionName: apprev5.Name,
					Placement: []oamcore.ClusterPlacement{{
						Distribution: oamcore.Distribution{
							Replicas: 1,
						},
					}},
				}},
			},
		}
		reconcileAppDeployment(ctx, appd)

		svc1 := makeService("testsvc", ns, apprev4.Name, 8000)
		checkService(ctx, svc1.Name, ns)
		svc2 := makeService("testsvc", ns, apprev5.Name, 80)
		checkService(ctx, svc2.Name, ns)

		vsvc := &istioclientv1beta1.VirtualService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appd.Name,
				Namespace: appd.Namespace,
			},
			Spec: istioapiv1beta1.VirtualService{
				Hosts:    []string{"example.com"},
				Gateways: []string{"vela-gateway"},
				Http: []*istioapiv1beta1.HTTPRoute{{
					Route: []*istioapiv1beta1.HTTPRouteDestination{{
						Destination: &istioapiv1beta1.Destination{
							Host: svc1.Name,
						},
						Weight: int32(50),
					}, {
						Destination: &istioapiv1beta1.Destination{
							Host: svc2.Name,
						},
						Weight: int32(50),
					}},
				}},
			},
		}

		getVSvc := &istioclientv1beta1.VirtualService{}
		key := client.ObjectKey{Name: vsvc.Name, Namespace: vsvc.Namespace}
		Expect(k8sClient.Get(ctx, key, getVSvc)).Should(BeNil())

		Expect(assert.ObjectsAreEqual(vsvc.Spec, getVSvc.Spec)).Should(BeEquivalentTo(true))
	})
})

func setupAppRevision(ctx context.Context, name, ns string, ac []byte, comps ...[]byte) *oamcore.ApplicationRevision {

	rawComps := []common.RawComponent{}
	for _, comp := range comps {
		compCopy := make([]byte, len(comp))
		copy(compCopy, comp)
		rawComps = append(rawComps, common.RawComponent{Raw: runtime.RawExtension{Raw: compCopy}})
	}

	acCopy := make([]byte, len(ac))
	copy(acCopy, ac)

	apprev := &oamcore.ApplicationRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: oamcore.ApplicationRevisionSpec{
			Application:              *appref.DeepCopy(),
			Components:               rawComps,
			ApplicationConfiguration: runtime.RawExtension{Raw: acCopy},
		},
	}
	Expect(k8sClient.Create(ctx, apprev)).Should(BeNil())
	return apprev
}

func reconcileAppDeployment(ctx context.Context, appd *oamcore.AppDeployment) {
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

func checkService(ctx context.Context, name, ns string) {
	key := client.ObjectKey{
		Name:      name,
		Namespace: ns,
	}
	svc := &corev1.Service{}
	Expect(k8sClient.Get(ctx, key, svc)).Should(BeNil())
}
