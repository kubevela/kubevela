/*
 Copyright 2021. The KubeVela Authors.

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
	"github.com/pkg/errors"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/testutil"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test Application with apply-once policy", func() {
	ctx := context.Background()

	initReplicas := int32(2)
	targetReplicas := int32(5)

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "apply-once-policy-test",
		},
	}

	baseApp := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "baseApp",
			Namespace: ns.Name,
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "baseComp",
					Type:       "worker",
					Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					Traits: []common.ApplicationTrait{{
						Type:       "scale",
						Properties: &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"replicas": %d}`, initReplicas))},
					}},
				},
			},
			Policies: []v1beta1.AppPolicy{{
				Name:       "basePolicy",
				Type:       "apply-once",
				Properties: &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"enable": true,"rules": [{"selector": {  "resourceTypes": ["Deployment"] }, "strategy": {"affect":"%s", "path": ["spec.replicas"] }}]}`, ""))},
			}},
		},
	}

	worker := &v1beta1.ComponentDefinition{}
	workerCdDefJson, _ := yaml.YAMLToJSON([]byte(componentDefYaml))

	scaleTrait := &v1beta1.TraitDefinition{}
	scaleTdDefJson, _ := yaml.YAMLToJSON([]byte(scaleTraitDefYaml))

	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, ns.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(json.Unmarshal(workerCdDefJson, worker)).Should(BeNil())
		Expect(k8sClient.Create(ctx, worker.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(json.Unmarshal(scaleTdDefJson, scaleTrait)).Should(BeNil())
		Expect(k8sClient.Create(ctx, scaleTrait.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	Context("Test Application with apply-once policy in different affect stage", func() {

		It(" Affect not set or affect is empty , test effective globally", func() {
			app := baseApp.DeepCopy()
			app.SetName("apply-once-app-1")
			app.Spec.Components[0].Name = "apply-once-comp-1"

			By("step 1. Create app , replicas: 2")
			Expect(k8sClient.Create(ctx, app)).Should(BeNil())
			Eventually(waitAppRunning(ctx, app), 3*time.Second, 300*time.Second).Should(BeNil())

			By("step 2. Update deployment to replicas: 5 ")
			Eventually(updateDeployReplicas(ctx, app, targetReplicas), time.Second*3, time.Microsecond*300).Should(BeNil())

			By("step 3. Check OnUpdate, e.g. update app's component with new properties, replicas should be 5 ")
			for i := 0; i <= 3; i++ {
				properties := &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"cmd":["sleep","%d"],"image":"busybox"}`, i*1000))}
				Eventually(updateApp(ctx, app, properties), time.Second*3, time.Microsecond*300).Should(BeNil())
				testutil.ReconcileRetry(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
				Eventually(waitAppRunning(ctx, app), 3*time.Second, 300*time.Second).Should(BeNil())

				deploy := new(v1.Deployment)
				deployObjKey := client.ObjectKey{Name: app.Spec.Components[0].Name, Namespace: app.Namespace}
				Expect(k8sClient.Get(ctx, deployObjKey, deploy)).Should(BeNil())
				Expect(*deploy.Spec.Replicas).Should(Equal(targetReplicas))
			}

			By("step 4. Check OnStateKeep, replicas also should be 5 ")
			rk, err := resourcekeeper.NewResourceKeeper(context.Background(), k8sClient, app)
			Expect(err).Should(BeNil())
			for i := 0; i <= 3; i++ {
				// state keep :5
				Expect(rk.StateKeep(context.Background())).Should(BeNil())
				deploy := new(v1.Deployment)
				deployObjKey := client.ObjectKey{Name: app.Spec.Components[0].Name, Namespace: app.Namespace}
				Expect(k8sClient.Get(ctx, deployObjKey, deploy)).Should(BeNil())
				Expect(*deploy.Spec.Replicas).Should(Equal(targetReplicas))
			}

		})

		It("Affect: onStateKeep, test only effective when state keep", func() {

			By("step 1. Create app , replicas: 2")
			app := baseApp.DeepCopy()
			app.SetName("apply-once-app-2")
			app.Spec.Components[0].Name = "apply-once-comp-2"
			app.Spec.Policies[0].Properties = &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"enable": true,"rules": [{"selector": {  "resourceTypes": ["Deployment"] }, "strategy": {"affect":"%s", "path": ["spec.replicas"] }}]}`, v1alpha1.ApplyOnceStrategyOnAppStateKeep))}
			Expect(k8sClient.Create(ctx, app)).Should(BeNil())
			Eventually(waitAppRunning(ctx, app), 3*time.Second, 300*time.Second).Should(BeNil())

			By("step 2. Update deployment, replicas: 5 ")
			Eventually(updateDeployReplicas(ctx, app, targetReplicas), time.Second*3, time.Microsecond*300).Should(BeNil())

			By("step 3. Check OnStateKeep, replicas should be 5 ")
			rk, err := resourcekeeper.NewResourceKeeper(context.Background(), k8sClient, app)
			Expect(err).Should(BeNil())
			for i := 0; i <= 3; i++ {
				// state keep : use newest replicas
				Expect(rk.StateKeep(context.Background())).Should(BeNil())
				deploy := new(v1.Deployment)
				deployObjKey := client.ObjectKey{Name: app.Spec.Components[0].Name, Namespace: app.Namespace}
				Expect(k8sClient.Get(ctx, deployObjKey, deploy)).Should(BeNil())
				Expect(*deploy.Spec.Replicas).Should(Equal(targetReplicas))
			}
			By("step 4. Check OnUpdate, e.g. update app's component with new properties, replicas should be 2 ")
			for i := 0; i <= 3; i++ {
				// onupdate: not use newest replicas
				properties := &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"cmd":["sleep","%d"],"image":"busybox"}`, i*1000))}
				Eventually(updateApp(ctx, app, properties), time.Second*3, time.Microsecond*300).Should(BeNil())
				testutil.ReconcileRetry(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
				Eventually(waitAppRunning(ctx, app), 3*time.Second, 300*time.Second).Should(BeNil())
				deploy := new(v1.Deployment)
				deployObjKey := client.ObjectKey{Name: app.Spec.Components[0].Name, Namespace: app.Namespace}
				Expect(k8sClient.Get(ctx, deployObjKey, deploy)).Should(BeNil())
				Expect(*deploy.Spec.Replicas).Should(Equal(initReplicas))
			}
		})

		It("Affect: onUpdate , test only effective when updating the app", func() {

			By("step 1. Create app , replicas: 2")
			app := baseApp.DeepCopy()
			app.SetName("apply-once-app-3")
			app.Spec.Components[0].Name = "apply-once-comp-3"
			app.Spec.Policies[0].Properties = &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"enable": true,"rules": [{"selector": {  "resourceTypes": ["Deployment"] }, "strategy": {"affect":"%s", "path": ["spec.replicas"] }}]}`, v1alpha1.ApplyOnceStrategyOnAppUpdate))}
			Expect(k8sClient.Create(ctx, app)).Should(BeNil())
			Eventually(waitAppRunning(ctx, app), 3*time.Second, 300*time.Second).Should(BeNil())

			By("step 2. Update deployment, replicas: 5 ")
			Eventually(updateDeployReplicas(ctx, app, targetReplicas), time.Second*3, time.Microsecond*300).Should(BeNil())

			By("step 3. Check OnUpdate, e.g. update app's component with new properties, replicas should be 5 ")
			for i := 0; i <= 3; i++ {
				// onUpdate : use newest replicas
				properties := &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"cmd":["sleep","%d"],"image":"busybox"}`, i*1000))}
				Eventually(updateApp(ctx, app, properties), time.Second*3, time.Microsecond*300).Should(BeNil())
				testutil.ReconcileRetry(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
				Eventually(waitAppRunning(ctx, app), 3*time.Second, 300*time.Second).Should(BeNil())
				deploy := new(v1.Deployment)
				deployObjKey := client.ObjectKey{Name: app.Spec.Components[0].Name, Namespace: app.Namespace}
				Expect(k8sClient.Get(ctx, deployObjKey, deploy)).Should(BeNil())
				Expect(*deploy.Spec.Replicas).Should(Equal(initReplicas))
			}

			By("step 4. Check OnStateKeep, replicas should be 2 ")
			rk, err := resourcekeeper.NewResourceKeeper(context.Background(), k8sClient, app)
			Expect(err).Should(BeNil())
			for i := 0; i <= 3; i++ {
				// state keep : not use newest replicas
				Expect(rk.StateKeep(context.Background())).Should(BeNil())
				deploy := new(v1.Deployment)
				deployObjKey := client.ObjectKey{Name: app.Spec.Components[0].Name, Namespace: app.Namespace}
				Expect(k8sClient.Get(ctx, deployObjKey, deploy)).Should(BeNil())
				Expect(*deploy.Spec.Replicas).Should(Equal(initReplicas))
			}
		})
	})
})

func updateDeployReplicas(ctx context.Context, app *v1beta1.Application, targetReplicas int32) func() error {
	return func() error {
		deploy := new(v1.Deployment)
		deployKey := client.ObjectKey{Name: app.Spec.Components[0].Name, Namespace: app.Namespace}
		Expect(k8sClient.Get(ctx, deployKey, deploy)).Should(BeNil())
		deploy.Spec.Replicas = &targetReplicas
		return k8sClient.Update(ctx, deploy)
	}
}

func waitAppRunning(ctx context.Context, app *v1beta1.Application) func() error {
	return func() error {
		appV1 := new(v1beta1.Application)
		_, err := testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
		if err != nil {
			return err
		}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), appV1); err != nil {
			return err
		}
		if appV1.Status.Phase != common.ApplicationRunning {
			return errors.New("app is not in running status")
		}
		return nil
	}
}

func updateApp(ctx context.Context, app *v1beta1.Application, properties *runtime.RawExtension) func() error {
	return func() error {
		oldApp := new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), oldApp)).Should(BeNil())
		newApp := oldApp.DeepCopy()
		newApp.Spec.Components[0].Properties = properties
		return k8sClient.Update(ctx, newApp)
	}
}

const (
	scaleTraitDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: Manually scale K8s pod for your workload which follows the pod spec in path 'spec.template'.
  name: scale
  namespace: vela-system
spec:
  appliesToWorkloads:
    - deployments.apps
    - statefulsets.apps
  podDisruptive: false
  schematic:
    cue:
      template: |
        parameter: {
        	// +usage=Specify the number of workload
        	replicas: *1 | int
        }
        // +patchStrategy=retainKeys
        patch: spec: replicas: parameter.replicas
`
)
