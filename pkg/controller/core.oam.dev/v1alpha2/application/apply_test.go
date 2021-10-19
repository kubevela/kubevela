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
	"time"

	"github.com/oam-dev/kubevela/pkg/oam/testutil"

	terraformtypes "github.com/oam-dev/terraform-controller/api/types"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
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

var _ = Describe("Test statusAggregate", func() {
	It("the component is Terraform type", func() {
		var (
			ctx           = context.TODO()
			componentName = "sample-oss"
			ns            = "default"
			h             = &AppHandler{r: reconciler, app: &v1beta1.Application{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{Namespace: ns},
			}}
			appFile = &appfile.Appfile{
				Workloads: []*appfile.Workload{
					{
						Name: componentName,
						FullTemplate: &appfile.Template{
							Reference: common.WorkloadTypeDescriptor{
								Definition: common.WorkloadGVK{APIVersion: "v1", Kind: "A1"},
							},
						},
						CapabilityCategory: velatypes.TerraformCategory,
					},
				},
			}
		)

		By("aggregate status")
		statuses, healthy, err := h.aggregateHealthStatus(appFile)
		Expect(statuses).Should(BeNil())
		Expect(healthy).Should(Equal(false))
		Expect(err).Should(HaveOccurred())

		By("create Terraform configuration")
		configuration := terraformapi.Configuration{
			TypeMeta:   metav1.TypeMeta{APIVersion: "terraform.core.oam.dev/v1beta1", Kind: "Configuration"},
			ObjectMeta: metav1.ObjectMeta{Name: componentName, Namespace: ns},
		}
		k8sClient.Create(ctx, &configuration)

		By("aggregate status again")
		statuses, healthy, err = h.aggregateHealthStatus(appFile)
		Expect(len(statuses)).Should(Equal(1))
		Expect(healthy).Should(Equal(false))
		Expect(err).Should(BeNil())

		By("set status for Terraform configuration")
		var gotConfiguration terraformapi.Configuration
		k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: componentName}, &gotConfiguration)
		gotConfiguration.Status.Apply.State = terraformtypes.Available
		k8sClient.Status().Update(ctx, &gotConfiguration)

		By("aggregate status one more time")
		statuses, healthy, err = h.aggregateHealthStatus(appFile)
		Expect(len(statuses)).Should(Equal(1))
		Expect(healthy).Should(Equal(true))
		Expect(err).Should(BeNil())
	})
})
