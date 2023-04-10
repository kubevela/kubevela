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

package application

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oam-dev/kubevela/pkg/oam/testutil"

	terraformtypes "github.com/oam-dev/terraform-controller/api/types"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const workloadDefinition = `
apiVersion: core.oam.dev/v1beta1
kind: WorkloadDefinition
metadata:
  name: test-worker
  annotations:
    definition.oam.dev/description: "Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic."
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    cue:
      template: |
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
                }]
              }
            }
          }
        }
        parameter: {
          image: string
          cmd?: [...string]
        }
`

var _ = Describe("Test Application apply", func() {
	var app *v1beta1.Application
	var namespaceName string
	var ns corev1.Namespace

	BeforeEach(func() {
		ctx := context.TODO()
		namespaceName = "apply-test-" + strconv.Itoa(time.Now().Second()) + "-" + strconv.Itoa(time.Now().Nanosecond())
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}
		app = &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
		}
		app.Namespace = namespaceName
		app.Spec = v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Type: "test-worker",
				Name: "test-app",
				Properties: &runtime.RawExtension{
					Raw: []byte(`{"image": "oamdev/testapp:v1", "cmd": ["node", "server.js"]}`),
				},
			}},
		}
		By("Create the Namespace for test")
		Expect(k8sClient.Create(ctx, &ns)).Should(Succeed())
	})

	AfterEach(func() {
		By("[TEST] Clean up resources after an integration test")
		Expect(k8sClient.Delete(context.TODO(), &ns)).Should(Succeed())
	})

	It("Test update or create app revision", func() {
		ctx := context.TODO()
		By("[TEST] Create a workload definition")
		var deployDef v1beta1.WorkloadDefinition
		Expect(yaml.Unmarshal([]byte(workloadDefinition), &deployDef)).Should(BeNil())
		deployDef.Namespace = app.Namespace
		Expect(k8sClient.Create(ctx, &deployDef)).Should(SatisfyAny(BeNil()))

		By("[TEST] Create a application")
		app.Name = "poc"
		err := k8sClient.Create(ctx, app)
		Expect(err).Should(BeNil())

		By("[TEST] get a application")
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace}})
		testapp := v1beta1.Application{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, &testapp)
		Expect(err).Should(BeNil())
		Expect(testapp.Status.LatestRevision != nil).Should(BeTrue())

		By("[TEST] get a application revision")
		appRevName := testapp.Status.LatestRevision.Name
		apprev := &v1beta1.ApplicationRevision{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: appRevName, Namespace: app.Namespace}, apprev)
		Expect(err).Should(BeNil())

		By("[TEST] verify that the revision is exist and set correctly")
		applabel, exist := apprev.Labels["app.oam.dev/name"]
		Expect(exist).Should(BeTrue())
		Expect(strings.Compare(applabel, app.Name) == 0).Should(BeTrue())
	})
})

var _ = Describe("Test deleter resource", func() {
	It("Test delete resource will remove ref from reference", func() {
		deployName := "test-del-resource-workload"
		namespace := "test-del-resource-namespace"
		ctx := context.Background()
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).Should(BeNil())
		deploy := appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      deployName,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: pointer.Int32(3),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "test",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "test",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test-container",
								Image: "test-image",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, &deploy)).Should(BeNil())
		u := unstructured.Unstructured{}
		u.SetAPIVersion("apps/v1")
		u.SetKind("Deployment")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: deployName, Namespace: namespace}, &u)).Should(BeNil())
		appliedRsc := []common.ClusterObjectReference{
			{
				Creator: common.WorkflowResourceCreator,
				ObjectReference: corev1.ObjectReference{
					Kind:       u.GetKind(),
					APIVersion: u.GetAPIVersion(),
					Namespace:  u.GetNamespace(),
					Name:       deployName,
				},
			},
			{
				Creator: common.WorkflowResourceCreator,
				ObjectReference: corev1.ObjectReference{
					Kind:       "StatefulSet",
					APIVersion: "apps/v1",
					Namespace:  "test-namespace",
					Name:       "test-sts",
				},
			},
		}
		h, err := NewAppHandler(ctx, reconciler, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"}}, nil)
		Expect(err).Should(Succeed())
		h.appliedResources = appliedRsc
		Expect(h.Delete(ctx, "", common.WorkflowResourceCreator, &u))
		checkDeploy := unstructured.Unstructured{}
		checkDeploy.SetAPIVersion("apps/v1")
		checkDeploy.SetKind("Deployment")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: deployName, Namespace: namespace}, &u)).Should(SatisfyAny(util.NotFoundMatcher{}))
		Expect(len(h.appliedResources)).Should(BeEquivalentTo(1))
		Expect(h.appliedResources[0].Kind).Should(BeEquivalentTo("StatefulSet"))
		Expect(h.appliedResources[0].Name).Should(BeEquivalentTo("test-sts"))
	})
})

func TestDeleteAppliedResourceFunc(t *testing.T) {
	h := AppHandler{appliedResources: []common.ClusterObjectReference{
		{
			ObjectReference: corev1.ObjectReference{
				Name: "wl-1",
				Kind: "Deployment",
			},
		},
		{
			ObjectReference: corev1.ObjectReference{
				Name: "wl-2",
				Kind: "Deployment",
			},
		},
		{
			ObjectReference: corev1.ObjectReference{
				Name: "wl-1",
				Kind: "StatefulSet",
			},
		},
		{
			Cluster: "runtime-cluster",
			ObjectReference: corev1.ObjectReference{
				Name: "wl-1",
				Kind: "StatefulSet",
			},
		},
	}}
	deleteResc_1 := common.ClusterObjectReference{ObjectReference: corev1.ObjectReference{Name: "wl-1", Kind: "StatefulSet"}, Cluster: "runtime-cluster"}
	deleteResc_2 := common.ClusterObjectReference{ObjectReference: corev1.ObjectReference{Name: "wl-2", Kind: "Deployment"}}
	h.deleteAppliedResource(deleteResc_1)
	h.deleteAppliedResource(deleteResc_2)
	if len(h.appliedResources) != 2 {
		t.Errorf("applied length error acctually %d", len(h.appliedResources))
	}
	if h.appliedResources[0].Name != "wl-1" || h.appliedResources[0].Kind != "Deployment" {
		t.Errorf("resource missmatch")
	}
	if h.appliedResources[1].Name != "wl-1" || h.appliedResources[1].Kind != "StatefulSet" {
		t.Errorf("resource missmatch")
	}

	preDelResc := common.ClusterObjectReference{ObjectReference: corev1.ObjectReference{Name: "wl-3", Kind: "StatefulSet"}, Cluster: "runtime-cluster"}
	h.deleteAppliedResource(preDelResc)
	h.addAppliedResource(true, preDelResc)
	if len(h.appliedResources) != 2 {
		t.Errorf("applied length error acctually %d", len(h.appliedResources))
	}
}

var _ = Describe("Test Application health check", func() {
	const (
		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 500
	)

	var (
		ctx context.Context
		ns  corev1.Namespace
	)

	BeforeEach(func() {
		ctx = context.TODO()
		namespaceName := "health-check-test-" + strconv.Itoa(time.Now().Second()) + "-" + strconv.Itoa(time.Now().Nanosecond())
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}
		By("By create the Namespace for test")
		Expect(k8sClient.Create(ctx, &ns)).Should(Succeed())
	})

	AfterEach(func() {
		By("By clean up resources after an integration test")
		Expect(k8sClient.Delete(ctx, &ns)).Should(Succeed())
	})

	Context("Terraform components", func() {
		var (
			componentNamespace = "vela-system"
			componentName      string
			comp               v1beta1.ComponentDefinition

			applicationName = "test-tf-health-app"
		)

		BeforeEach(func() {
			componentName = "demo-hello-tf" + strconv.Itoa(time.Now().Second()) + "-" + strconv.Itoa(time.Now().Nanosecond())
			comp = v1beta1.ComponentDefinition{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.ComponentDefinitionKindAPIVersion,
					Kind:       v1beta1.ComponentDefinitionKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      componentName,
					Namespace: componentNamespace,
					Labels: map[string]string{
						"type": "terraform",
					},
				},
				Spec: v1beta1.ComponentDefinitionSpec{
					Workload: common.WorkloadTypeDescriptor{
						Definition: common.WorkloadGVK{
							APIVersion: "terraform.core.oam.dev/v1beta2",
							Kind:       "Configuration",
						},
					},
					Schematic: &common.Schematic{
						Terraform: &common.Terraform{
							Configuration: `
							terraform {}

							variable "name" {
							  type        = string
							  description = "your name"
							}
							
							output "message" {
							  value = "hello, ${var.name}"
							}`,
						},
					},
				},
			}
			By("By create terraform component")
			Expect(k8sClient.Create(ctx, &comp)).Should(Succeed())
		})

		AfterEach(func() {
			By("By clean terraform component")
			Expect(k8sClient.Delete(ctx, &comp)).Should(Succeed())
		})

		It("Should terraform components stand ready and the latest when a new application is running", func() {
			By("By creating a new app")
			tfCompName := applicationName + "-comp"
			app := v1beta1.Application{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.ApplicationKindAPIVersion,
					Kind:       v1beta1.ApplicationKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      applicationName,
					Namespace: ns.Name,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: tfCompName,
							Type: componentName,
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"name": "vela!"}`),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, &app)).Should(Succeed())

			applicationLoopupKey := types.NamespacedName{Name: applicationName, Namespace: ns.Name}
			testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: applicationLoopupKey})

			configurationLookupKey := types.NamespacedName{Name: tfCompName, Namespace: ns.Name}
			configuration := &terraformapi.Configuration{}

			Eventually(func() error {
				return k8sClient.Get(ctx, configurationLookupKey, configuration)
			}, timeout, interval).Should(Succeed())

			By("By checking the Application status is not running")
			Consistently(func() (common.ApplicationPhase, error) {
				err := k8sClient.Get(ctx, applicationLoopupKey, &app)
				if err != nil {
					return "", err
				}
				return app.Status.Phase, nil
			}, duration, interval).ShouldNot(Equal(common.ApplicationRunning))

			By("By configuration is ready")
			configuration.Status = terraformapi.ConfigurationStatus{
				ObservedGeneration: configuration.Generation,
				Apply: terraformapi.ConfigurationApplyStatus{
					State: terraformtypes.Available,
				},
			}
			Expect(k8sClient.Status().Update(ctx, configuration)).Should(Succeed())

			By("By checking that the Application status is running")
			Eventually(func() common.ApplicationPhase {
				testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: applicationLoopupKey})
				err := k8sClient.Get(ctx, applicationLoopupKey, &app)
				if err != nil {
					return ""
				}
				return app.Status.Phase
			}, timeout, interval).Should(Equal(common.ApplicationRunning))

			By("By update the Application and check that the status is not running")
			app.Spec.Components[0].Properties.Raw = []byte(`{"name": "terraform!"}`)
			Expect(k8sClient.Update(ctx, &app)).Should(Succeed())

			Consistently(func() (common.ApplicationPhase, error) {
				testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: applicationLoopupKey})
				if err := k8sClient.Get(ctx, applicationLoopupKey, &app); err != nil {
					return "", err
				}
				return app.Status.Phase, nil
			}, duration, interval).ShouldNot(Equal(common.ApplicationRunning))
		})
	})
})
