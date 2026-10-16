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

package config_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("Config workflow steps", Label("core-config"), func() {
	ctx := context.Background()

	var namespace string

	BeforeEach(func() {
		namespace = randomNamespaceName("config-e2e-test")
		ns := createNamespace(ctx, namespace)
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground)))).To(Succeed())
		})
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))
	})

	applyApplication := func(manifest string) *v1beta1.Application {
		var app v1beta1.Application
		Expect(yaml.Unmarshal([]byte(manifest), &app)).Should(Succeed())
		app.Namespace = namespace
		Expect(k8sClient.Create(ctx, &app)).Should(Succeed())
		return &app
	}

	waitRunning := func(app *v1beta1.Application) {
		Eventually(func() common.ApplicationPhase {
			var got v1beta1.Application
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), &got); err != nil {
				klog.Errorf("fail to query the app %s", err.Error())
				return ""
			}
			klog.Infof("the application status is %s (%+v)", got.Status.Phase, got.Status.Workflow)
			return got.Status.Phase
		}, time.Minute, 2*time.Second).Should(Equal(common.ApplicationRunning))
	}

	gone := func(obj client.Object) {
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("%T %s is still there", obj, obj.GetName())
		}, time.Minute, 2*time.Second).Should(Succeed())
	}

	It("create-config writes a config the controller renders, read-config reads it, delete-config removes it", func() {
		app := applyApplication(manageConfigApplication)
		waitRunning(app)

		By("read-config's output reached the workflow context")
		var wfContext corev1.ConfigMap
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "workflow-" + app.Name + "-context"}, &wfContext)).Should(Succeed())
		Expect(wfContext.Data["vars"]).Should(ContainSubstring("read-config"))
		Expect(wfContext.Data["vars"]).Should(ContainSubstring("value1"))
		Expect(wfContext.Data["vars"]).Should(ContainSubstring("value5"))

		By("delete-config removed the Config and, through it, the Secret")
		gone(&configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "test"}})
		gone(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "test"}})
	})

	It("a config a workflow leaves behind is a Config the application tracks, rendered into an owned Secret, and goes with the application", func() {
		app := applyApplication(keepConfigApplication)
		waitRunning(app)

		By("the step wrote a Config, stamped as the application's, and the controller rendered it")
		var cfg configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "kept"}, &cfg)).Should(Succeed())
		Expect(cfg.Status.Phase).Should(Equal(configv1alpha1.ConfigPhaseAvailable))
		Expect(cfg.Labels["app.oam.dev/name"]).Should(Equal(app.Name))
		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "kept"}, &secret)).Should(Succeed())
		Expect(string(secret.Data["input-properties"])).Should(MatchJSON(`{"key1":"value1"}`), "a Config with no template stores its properties as one document, as the legacy factory did")
		Expect(metav1.GetControllerOf(&secret)).ShouldNot(BeNil())
		Expect(metav1.GetControllerOf(&secret).UID).Should(Equal(cfg.UID))

		By("the application's ResourceTracker lists the Config")
		var trackers v1beta1.ResourceTrackerList
		Expect(k8sClient.List(ctx, &trackers, client.MatchingLabels{"app.oam.dev/name": app.Name, "app.oam.dev/namespace": namespace})).Should(Succeed())
		var tracked []string
		for _, rt := range trackers.Items {
			for _, mr := range rt.Spec.ManagedResources {
				tracked = append(tracked, mr.Kind+"/"+mr.Name)
			}
		}
		Expect(tracked).Should(ContainElement("Config/kept"))

		By("deleting the application removes the Config and its Secret")
		Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
		gone(&configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "kept"}})
		gone(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "kept"}})
	})
})

var manageConfigApplication = `
kind: Application
apiVersion: core.oam.dev/v1beta1
metadata:
  name: test-config
  namespace: "config-e2e-test"
spec:
  components: []
  workflow:
    steps:
    - name: write-config
      type: create-config
      properties:
        name: test
        config: 
          key1: value1
          key2: 2
          key3: true
          key4: 
            key5: value5
    - name: read-config
      type: read-config
      properties:
        name: test
      outputs:
      - name: read-config
        valueFrom: output.config
    - name: delete-config
      type: delete-config
      properties:
        name: test
`

var keepConfigApplication = `
kind: Application
apiVersion: core.oam.dev/v1beta1
metadata:
  name: keep-config
  namespace: "config-e2e-test"
spec:
  components: []
  workflow:
    steps:
    - name: write-config
      type: create-config
      properties:
        name: kept
        config:
          key1: value1
`
