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
	"encoding/json"
	"fmt"

	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/testutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Test application controller finalizer logic", func() {
	ctx := context.TODO()
	namespace := "cross-namespace"

	cd := &v1beta1.ComponentDefinition{}
	cDDefJson, _ := yaml.YAMLToJSON([]byte(crossCompDefYaml))

	ncd := &v1beta1.ComponentDefinition{}
	ncdDefJson, _ := yaml.YAMLToJSON([]byte(normalCompDefYaml))

	badCD := &v1beta1.ComponentDefinition{}
	badCDJson, _ := yaml.YAMLToJSON([]byte(badCompDefYaml))

	BeforeEach(func() {
		ns := v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(cDDefJson, cd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, cd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(ncdDefJson, ncd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ncd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(badCDJson, badCD)).Should(BeNil())
		Expect(k8sClient.Create(ctx, badCD.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("[TEST] Clean up resources after an integration test")
		Expect(k8sClient.DeleteAllOf(ctx, &appsv1.Deployment{}, client.InNamespace(namespace)))
		Expect(k8sClient.DeleteAllOf(ctx, &appsv1.ControllerRevision{}, client.InNamespace(namespace)))
	})

	It("Test error occurs in the middle of dispatching", func() {
		appName := "bad-app"
		appKey := types.NamespacedName{Namespace: namespace, Name: appName}
		app := getApp(appName, namespace, "bad-worker")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		By("Create a bad workload app")
		checkApp := &v1beta1.Application{}
		testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())

		// though error occurs in the middle of dispatching
		// resource tracker for v1 is also created and recorded in app status
		By("Verify latest app revision is also recorded in status")
		Expect(checkApp.Status.LatestRevision).ShouldNot(BeNil())

		By("Delete Application")
		Expect(k8sClient.Delete(ctx, checkApp)).Should(BeNil())
		testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
	})

	It("Test cross namespace workload, then delete the app", func() {
		appName := "app-2"
		appKey := types.NamespacedName{Namespace: namespace, Name: appName}
		app := getApp(appName, namespace, "cross-worker")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		By("Create a cross workload app")
		testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(Equal(common.ApplicationRunning))
		Expect(len(checkApp.Finalizers)).Should(BeEquivalentTo(1))
		rt := &v1beta1.ResourceTracker{}
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name, "v1"), rt)).Should(BeNil())
		testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
		checkApp = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(len(checkApp.Finalizers)).Should(BeEquivalentTo(1))
		Expect(checkApp.Finalizers[0]).Should(BeEquivalentTo(oam.FinalizerResourceTracker))
		By("delete this cross workload app")
		Expect(k8sClient.Delete(ctx, checkApp)).Should(BeNil())
		By("delete app will delete resourceTracker")
		// reconcile will delete resourceTracker and unset app's finalizer
		testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, ctrl.Request{NamespacedName: appKey})
		checkApp = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(util.NotFoundMatcher{})
		checkRt := new(v1beta1.ResourceTracker)
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name, "v1"), checkRt)).Should(util.NotFoundMatcher{})
	})

	It("Test cross namespace workload, then update the app to change the namespace", func() {
		appName := "app-3"
		appKey := types.NamespacedName{Namespace: namespace, Name: appName}
		app := getApp(appName, namespace, "cross-worker")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		By("Create a cross workload app")
		testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(Equal(common.ApplicationRunning))
		Expect(len(checkApp.Finalizers)).Should(BeEquivalentTo(1))
		rt := &v1beta1.ResourceTracker{}
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name, "v1"), rt)).Should(BeNil())
		testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
		checkApp = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(len(checkApp.Finalizers)).Should(BeEquivalentTo(1))
		Expect(checkApp.Finalizers[0]).Should(BeEquivalentTo(oam.FinalizerResourceTracker))
		Expect(len(rt.Spec.ManagedResources)).Should(BeEquivalentTo(1))
		By("Update the app, set type to normal-worker")
		checkApp.Spec.Components[0].Type = "normal-worker"
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
		checkApp = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name, "v2"), rt)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, checkApp)).Should(BeNil())
		testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
	})
})

func getApp(appName, namespace, comptype string) *v1beta1.Application {
	return &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "comp1",
					Type:       comptype,
					Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
				},
			},
		},
	}
}

func getTrackerKey(namespace, name, revision string) types.NamespacedName {
	return types.NamespacedName{Name: fmt.Sprintf("%s-%s-%s", name, revision, namespace)}
}

const (
	crossCompDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: cross-worker
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  extension:
    healthPolicy: |
      isHealth: context.output.status.readyReplicas == context.output.status.replicas
    template: |
      output: {
          apiVersion: "apps/v1"
          kind:       "Deployment"
          metadata: {
              namespace: "cross-namespace"
          }
          spec: {
              replicas: 0
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
      }
`

	normalCompDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: normal-worker
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  extension:
    healthPolicy: |
      isHealth: context.output.status.readyReplicas == context.output.status.replicas
    template: |
      output: {
          apiVersion: "apps/v1"
          kind:       "Deployment"
          spec: {
              replicas: 0
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
      }
`
	badCompDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: bad-worker
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "It will make dispatching failed"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  extension:
    template: |
      output: {
          apiVersion: "apps/v1"
          kind:       "Deployment"
          spec: {
              replicas: 0
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
      }
`
)
