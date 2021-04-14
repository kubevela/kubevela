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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/ghodss/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("Test application controller finalizer logic", func() {
	ctx := context.TODO()
	namespace := "cross-ns-namespace"

	cd := &v1beta1.ComponentDefinition{}
	cDDefJson, _ := yaml.YAMLToJSON([]byte(crossCompDefYaml))

	ncd := &v1beta1.ComponentDefinition{}
	ncdDefJson, _ := yaml.YAMLToJSON([]byte(normalCompDefYaml))

	td := &v1beta1.TraitDefinition{}
	tdDefJson, _ := yaml.YAMLToJSON([]byte(crossNsTdYaml))

	BeforeEach(func() {
		ns := v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(cDDefJson, cd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, cd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(tdDefJson, td)).Should(BeNil())
		Expect(k8sClient.Create(ctx, td.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(ncdDefJson, ncd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ncd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("[TEST] Clean up resources after an integration test")
	})

	It("Test component have normal workload", func() {
		appName := "app-1"
		appKey := types.NamespacedName{Namespace: namespace, Name: appName}
		app := getApp(appName, namespace, "normal-worker")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		By("Create a normal workload app")
		checkApp := &v1beta1.Application{}
		_, err := reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(Equal(common.ApplicationRunning))
		Expect(len(checkApp.Finalizers)).Should(BeEquivalentTo(0))

		rt := &v1beta1.ResourceTracker{}
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name), rt)).Should(util.NotFoundMatcher{})

		By("add a cross namespace trait for application")
		updateApp := checkApp.DeepCopy()
		updateApp.Spec.Components[0].Traits = []v1beta1.ApplicationTrait{
			{
				Type:       "cross-scaler",
				Properties: runtime.RawExtension{Raw: []byte(`{"replicas": 1}`)},
			},
		}
		Expect(k8sClient.Update(ctx, updateApp)).Should(BeNil())
		// first reconcile will create resourceTracker and set resourceTracker for app status
		_, err = reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		checkApp = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name), rt)).Should(BeNil())
		Expect(checkApp.Status.ResourceTracker.UID).Should(BeEquivalentTo(rt.UID))
		Expect(len(checkApp.Finalizers)).Should(BeEquivalentTo(0))

		// second reconcile will set finalizer for app
		_, err = reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		checkApp = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name), rt)).Should(BeNil())
		Expect(err).Should(BeNil())
		Expect(len(checkApp.Finalizers)).Should(BeEquivalentTo(1))
		Expect(checkApp.Finalizers[0]).Should(BeEquivalentTo(resourceTrackerFinalizer))

		By("update app by delete cross namespace trait, will delete resourceTracker and the status of app will flush")
		checkApp = &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		updateApp = checkApp.DeepCopy()
		updateApp.Spec.Components[0].Traits = nil
		Expect(k8sClient.Update(ctx, updateApp)).Should(BeNil())
		_, err = reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		checkApp = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name), rt)).Should(util.NotFoundMatcher{})
		Expect(checkApp.Status.ResourceTracker).Should(BeNil())
	})

	It("Test cross namespace workload, then delete the app", func() {
		appName := "app-2"
		appKey := types.NamespacedName{Namespace: namespace, Name: appName}
		app := getApp(appName, namespace, "cross-worker")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		By("Create a cross workload app")
		_, err := reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(Equal(common.ApplicationRunning))
		Expect(len(checkApp.Finalizers)).Should(BeEquivalentTo(0))
		rt := &v1beta1.ResourceTracker{}
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name), rt)).Should(BeNil())
		_, err = reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		checkApp = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(len(checkApp.Finalizers)).Should(BeEquivalentTo(1))
		Expect(checkApp.Finalizers[0]).Should(BeEquivalentTo(resourceTrackerFinalizer))
		By("delete this cross workload app")
		Expect(k8sClient.Delete(ctx, checkApp)).Should(BeNil())
		By("delete app will delete resourceTracker")
		// reconcile will delete resourceTracker and unset app's finalizer
		_, err = reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		checkApp = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(util.NotFoundMatcher{})
		checkRt := new(v1beta1.ResourceTracker)
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name), checkRt)).Should(util.NotFoundMatcher{})
	})

	It("Test cross namespace workload, then update the app to change the namespace", func() {
		appName := "app-3"
		appKey := types.NamespacedName{Namespace: namespace, Name: appName}
		app := getApp(appName, namespace, "cross-worker")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		By("Create a cross workload app")
		_, err := reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(Equal(common.ApplicationRunning))
		Expect(len(checkApp.Finalizers)).Should(BeEquivalentTo(0))
		rt := &v1beta1.ResourceTracker{}
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name), rt)).Should(BeNil())
		_, err = reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		checkApp = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(len(checkApp.Finalizers)).Should(BeEquivalentTo(1))
		Expect(checkApp.Finalizers[0]).Should(BeEquivalentTo(resourceTrackerFinalizer))
		Expect(checkApp.Status.ResourceTracker.UID).Should(BeEquivalentTo(rt.UID))
		Expect(len(rt.Status.TrackedResources)).Should(BeEquivalentTo(1))
		By("Update the app, set type to normal-worker")
		checkApp.Spec.Components[0].Type = "normal-worker"
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err = reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		checkApp = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.ResourceTracker).Should(BeNil())
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name), rt)).Should(util.NotFoundMatcher{})
		Expect(k8sClient.Delete(ctx, checkApp)).Should(BeNil())
		_, err = reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
	})

	It("Test cross namespace workload and trait, then update the app to delete trait ", func() {
		appName := "app-4"
		appKey := types.NamespacedName{Namespace: namespace, Name: appName}
		app := getApp(appName, namespace, "cross-worker")
		app.Spec.Components[0].Traits = []v1beta1.ApplicationTrait{
			{
				Type:       "cross-scaler",
				Properties: runtime.RawExtension{Raw: []byte(`{"replicas": 1}`)},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		By("Create a cross workload trait app")
		_, err := reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(Equal(common.ApplicationRunning))
		Expect(len(checkApp.Finalizers)).Should(BeEquivalentTo(0))
		rt := &v1beta1.ResourceTracker{}
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name), rt)).Should(BeNil())
		_, err = reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		checkApp = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(len(checkApp.Finalizers)).Should(BeEquivalentTo(1))
		Expect(checkApp.Finalizers[0]).Should(BeEquivalentTo(resourceTrackerFinalizer))
		Expect(checkApp.Status.ResourceTracker.UID).Should(BeEquivalentTo(rt.UID))
		Expect(len(rt.Status.TrackedResources)).Should(BeEquivalentTo(2))
		By("Update the app, set type to normal-worker")
		checkApp.Spec.Components[0].Traits = nil
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err = reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		rt = &v1beta1.ResourceTracker{}
		checkApp = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name), rt)).Should(BeNil())
		Expect(checkApp.Status.ResourceTracker.UID).Should(BeEquivalentTo(rt.UID))
		Expect(len(rt.Status.TrackedResources)).Should(BeEquivalentTo(1))
		Expect(k8sClient.Delete(ctx, checkApp)).Should(BeNil())
		_, err = reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		Expect(k8sClient.Get(ctx, getTrackerKey(checkApp.Namespace, checkApp.Name), rt)).Should(util.NotFoundMatcher{})
	})
})

var _ = Describe("Test finalizer related func", func() {
	ctx := context.TODO()
	namespace := "cross-ns-namespace"
	var handler appHandler

	BeforeEach(func() {
		ns := v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("[TEST] Clean up resources after an integration test")
	})

	It("Test finalizeResourceTracker func with need update ", func() {
		app := getApp("app-3", namespace, "worker")
		rt := &v1beta1.ResourceTracker{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace + "-" + app.GetName(),
			},
		}
		Expect(k8sClient.Create(ctx, rt)).Should(BeNil())
		app.Status.ResourceTracker = &runtimev1alpha1.TypedReference{
			Name:       rt.Name,
			Kind:       v1beta1.ResourceTrackerGroupKind,
			APIVersion: v1beta1.ResourceTrackerKindAPIVersion,
			UID:        rt.UID}
		meta.AddFinalizer(&app.ObjectMeta, resourceTrackerFinalizer)
		handler = appHandler{
			r:      reconciler,
			app:    app,
			logger: reconciler.Log.WithValues("application", "finalizer-func-test"),
		}
		need, err := handler.removeResourceTracker(ctx)
		Expect(err).Should(BeNil())
		Expect(need).Should(BeEquivalentTo(true))
		Eventually(func() error {
			err := k8sClient.Get(ctx, getTrackerKey(namespace, app.Name), rt)
			if err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("resourceTracker still exsit")
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
		Expect(app.Status.ResourceTracker).Should(BeNil())
		Expect(meta.FinalizerExists(app, resourceTrackerFinalizer)).Should(BeEquivalentTo(false))
	})

	It("Test finalizeResourceTracker func without need ", func() {
		app := getApp("app-4", namespace, "worker")
		handler = appHandler{
			r:      reconciler,
			app:    app,
			logger: reconciler.Log.WithValues("application", "finalizer-func-test"),
		}
		need, err := handler.removeResourceTracker(ctx)
		Expect(err).Should(BeNil())
		Expect(need).Should(BeEquivalentTo(false))
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
			Components: []v1beta1.ApplicationComponent{
				{
					Name:       "comp1",
					Type:       comptype,
					Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
				},
			},
		},
	}
}

func getTrackerKey(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Name: fmt.Sprintf("%s-%s", namespace, name)}
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

	crossNsTdYaml = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the app"
  name: cross-scaler
  namespace: vela-system
spec:
  appliesToWorkloads:
    - webservice
    - worker
  definitionRef:
    name: manualscalertraits.core.oam.dev
  workloadRefPath: spec.workloadRef
  extension:
    template: |-
      outputs: scaler: {
      	apiVersion: "core.oam.dev/v1alpha2"
      	kind:       "ManualScalerTrait"
        metadata: {
            namespace: "cross-namespace"
        }
      	spec: {
      		replicaCount: parameter.replicas
      	}
      }
      parameter: {
      	//+short=r
      	replicas: *1 | int
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
)
