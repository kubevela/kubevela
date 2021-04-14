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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
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
	var handler appHandler
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
			Components: []v1beta1.ApplicationComponent{{
				Type: "test-worker",
				Name: "test-app",
				Properties: runtime.RawExtension{
					Raw: []byte(`{"image": "oamdev/testapp:v1", "cmd": ["node", "server.js"]}`),
				},
			}},
		}
		handler = appHandler{
			r:      reconciler,
			app:    app,
			logger: reconciler.Log.WithValues("application", "unit-test"),
		}
		By("Create the Namespace for test")
		Expect(k8sClient.Create(ctx, &ns)).Should(Succeed())
	})

	AfterEach(func() {
		By("[TEST] Clean up resources after an integration test")
		Expect(k8sClient.Delete(context.TODO(), &ns)).Should(Succeed())
	})

	It("Test update or create component", func() {
		ctx := context.TODO()
		By("[TEST] Setting up the testing environment")
		imageV1 := "wordpress:4.6.1-apache"
		imageV2 := "wordpress:4.6.2-apache"
		cwV1 := v1alpha2.ContainerizedWorkload{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ContainerizedWorkload",
				APIVersion: "core.oam.dev/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaceName,
			},
			Spec: v1alpha2.ContainerizedWorkloadSpec{
				Containers: []v1alpha2.Container{
					{
						Name:  "wordpress",
						Image: imageV1,
						Ports: []v1alpha2.ContainerPort{
							{
								Name: "wordpress",
								Port: 80,
							},
						},
					},
				},
			},
		}
		component := &v1alpha2.Component{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Component",
				APIVersion: "core.oam.dev/v1alpha2",
			}, ObjectMeta: metav1.ObjectMeta{
				Name:      "myweb",
				Namespace: namespaceName,
				Labels:    map[string]string{"application.oam.dev": "test"},
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: &cwV1,
				},
			}}

		By("[TEST] Creating a component the first time")
		// take a copy so the component's workload still uses object instead of raw data
		// just like the way we use it in prod. The raw data will be filled by the k8s for some reason.
		revision, err := handler.createOrUpdateComponent(ctx, component.DeepCopy())
		By("verify that the revision is the set correctly and newRevision is true")
		Expect(err).ShouldNot(HaveOccurred())
		// verify the revision actually contains the right component
		Expect(utils.CompareWithRevision(ctx, handler.r, logging.NewLogrLogger(handler.logger), component.GetName(),
			component.GetNamespace(), revision, &component.Spec)).Should(BeTrue())
		preRevision := revision

		By("[TEST] update the component without any changes (mimic reconcile behavior)")
		revision, err = handler.createOrUpdateComponent(ctx, component.DeepCopy())
		By("verify that the revision is the same and newRevision is false")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(revision).Should(BeIdenticalTo(preRevision))

		By("[TEST] update the component")
		// modify the component spec through object
		cwV2 := cwV1.DeepCopy()
		cwV2.Spec.Containers[0].Image = imageV2
		component.Spec.Workload.Object = cwV2
		revision, err = handler.createOrUpdateComponent(ctx, component.DeepCopy())
		By("verify that the revision is changed and newRevision is true")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(revision).ShouldNot(BeIdenticalTo(preRevision))
		Expect(utils.CompareWithRevision(ctx, handler.r, logging.NewLogrLogger(handler.logger), component.GetName(),
			component.GetNamespace(), revision, &component.Spec)).Should(BeTrue())
		// revision increased
		Expect(strings.Compare(revision, preRevision) > 0).Should(BeTrue())
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
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace}})
		testapp := v1beta1.Application{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, &testapp)
		fmt.Printf("%+v", testapp)
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
