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

package applicationconfiguration

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test Deploying ApplicationConfiguration without TraitDefinition", func() {
	const (
		namespace     = "definition-test"
		appName       = "hello"
		componentName = "backend"
	)
	var (
		ctx         = context.Background()
		workload    v1alpha2.ContainerizedWorkload
		component   v1alpha2.Component
		workloadKey = client.ObjectKey{
			Name:      componentName,
			Namespace: namespace,
		}
		appConfig    v1alpha2.ApplicationConfiguration
		appConfigKey = client.ObjectKey{
			Name:      appName,
			Namespace: namespace,
		}
		req = reconcile.Request{NamespacedName: appConfigKey}
		ns  = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
	)

	BeforeEach(func() {})

	It("ManualScalerTrait should work successfully even though its TraitDefinition doesn't exist", func() {
		var componentStr = `
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: backend
  namespace: definition-test
spec:
  workload:
    apiVersion: core.oam.dev/v1alpha2
    kind: ContainerizedWorkload
    spec:
      containers:
        - name: nginx
          image: nginx:1.9.4
          ports:
            - containerPort: 80
              name: nginx
          env:
            - name: TEST_ENV
              value: test
          command: [ "/bin/bash", "-c", "--" ]
          args: [ "while true; do sleep 30; done;" ]
`

		var appConfigStr = `
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: hello
  namespace: definition-test
spec:
  components:
    - componentName: backend
      traits:
        - trait:
            apiVersion: core.oam.dev/v1alpha2
            kind: ManualScalerTrait
            spec:
              replicaCount: 2
              workloadRef:
                apiVersion: core.oam.dev/v1alpha2
                kind: ContainerizedWorkload
                name: backend
`

		By("Create namespace")
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Create Component")
		Expect(yaml.Unmarshal([]byte(componentStr), &component)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &component)).Should(Succeed())
		cmpV1 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV1)).Should(Succeed())

		By("Create ApplicationConfiguration")
		Expect(yaml.Unmarshal([]byte(appConfigStr), &appConfig)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())
		By("Check AppConfig created successfully")
		ac := &v1alpha2.ApplicationConfiguration{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appName}, ac)
		}, 3*time.Second, 300*time.Millisecond).Should(BeNil())

		By("Check workload created successfully")
		Eventually(func() error {
			By("Reconcile")
			reconcileRetry(reconciler, req)
			return k8sClient.Get(ctx, workloadKey, &workload)
		}, 5*time.Second, time.Second).Should(BeNil())

		By("Check reconcile again and no error will happen")
		reconcileRetry(reconciler, req)

		By("Check appConfig condition should not have error")
		Eventually(func() string {
			By("Reconcile again and should not have error")
			reconcileRetry(reconciler, req)
			err := k8sClient.Get(ctx, appConfigKey, &appConfig)
			if err != nil {
				return err.Error()
			}
			if len(appConfig.Status.Conditions) != 1 {
				return "condition len should be 1 but now is " + strconv.Itoa(len(appConfig.Status.Conditions))
			}
			return string(appConfig.Status.Conditions[0].Reason)
		}, 3*time.Second, 300*time.Millisecond).Should(BeEquivalentTo("ReconcileSuccess"))

		By("Check trait CR is created")
		var scaleName string
		scaleList := v1alpha2.ManualScalerTraitList{}
		labels := &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app.oam.dev/component": componentName,
			},
		}
		selector, _ := metav1.LabelSelectorAsSelector(labels)
		err := k8sClient.List(ctx, &scaleList, &client.ListOptions{
			Namespace:     namespace,
			LabelSelector: selector,
		})
		Expect(err).Should(BeNil())
		traitNamePrefix := fmt.Sprintf("%s-trait-", componentName)
		var traitExistFlag bool
		for _, t := range scaleList.Items {
			if strings.HasPrefix(t.Name, traitNamePrefix) {
				traitExistFlag = true
				scaleName = t.Name
			}
		}
		Expect(traitExistFlag).Should(BeTrue())

		By("Update ApplicationConfiguration by changing spec of trait")
		newTrait := &v1alpha2.ManualScalerTrait{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "core.oam.dev/v1alpha2",
				Kind:       "ManualScalerTrait",
			},
			Spec: v1alpha2.ManualScalerTraitSpec{
				ReplicaCount: 3,
				WorkloadReference: v1alpha1.TypedReference{
					APIVersion: "core.oam.dev/v1alpha2",
					Kind:       "ContainerizedWorkload",
					Name:       componentName,
				},
			},
		}
		appConfig.Spec.Components[0].Traits = []v1alpha2.ComponentTrait{{Trait: runtime.RawExtension{Object: newTrait.DeepCopyObject()}}}
		Expect(k8sClient.Update(ctx, &appConfig)).Should(BeNil())

		By("Reconcile")
		reconcileRetry(reconciler, req)

		By("Check again that appConfig condition should not have error")
		Eventually(func() string {
			By("Reconcile again and should not have error")
			reconcileRetry(reconciler, req)
			err := k8sClient.Get(ctx, appConfigKey, &appConfig)
			if err != nil {
				return err.Error()
			}
			if len(appConfig.Status.Conditions) != 1 {
				return "condition len should be 1 but now is " + strconv.Itoa(len(appConfig.Status.Conditions))
			}
			return string(appConfig.Status.Conditions[0].Reason)
		}, 3*time.Second, 300*time.Millisecond).Should(BeEquivalentTo("ReconcileSuccess"))

		By("Check new trait CR is applied")
		scale := v1alpha2.ManualScalerTrait{}
		scaleKey := client.ObjectKey{Name: scaleName, Namespace: namespace}
		Eventually(func() int32 {
			By("Reconcile")
			reconcileRetry(reconciler, req)
			if err := k8sClient.Get(ctx, scaleKey, &scale); err != nil {
				return 0
			}
			return scale.Spec.ReplicaCount
		}, 5*time.Second, time.Second).Should(Equal(int32(3)))
	})

	AfterEach(func() {
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).
			Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
	})
})
