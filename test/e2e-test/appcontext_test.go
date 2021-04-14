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

package controllers_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test applicationContext reconcile", func() {
	ctx := context.Background()
	var (
		namespace      = "appcontext-test-ns"
		acName1        = "applicationconfig1"
		acName2        = "applicationconfig2"
		compName1      = "component1"
		compName2      = "component2"
		containerName  = "test-container"
		containerImage = "notarealimage"
		cwName1        = "appcontext-test-cw1"
		cwName2        = "appcontext-test-cw2"
		arName1        = "ar1"
		arName2        = "ar2"
		appContextName = "appcontext1"
		traitName1     = "trait1"
		traitName2     = "trait2"
		key            = types.NamespacedName{Namespace: namespace, Name: appContextName}
	)
	var ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

	workload1 := cw(
		cwWithName(cwName1),
		cwWithContainers([]v1alpha2.Container{
			{
				Name:    containerName,
				Image:   containerImage,
				Command: []string{"sleep", "30s"},
				Ports: []v1alpha2.ContainerPort{
					v1alpha2.ContainerPort{
						Name: "http",
						Port: 80,
					},
				},
			},
		}),
	)

	rawWorkload1 := runtime.RawExtension{Object: workload1}
	co1 := comp(
		compWithName(compName1),
		compWithNamespace(namespace),
		compWithWorkload(rawWorkload1),
	)

	ac1 := ac(
		acWithName(acName1),
		acWithNamspace(namespace),
		acWithComps([]v1alpha2.ApplicationConfigurationComponent{
			{
				ComponentName: compName1,
				Traits:        []v1alpha2.ComponentTrait{},
			},
		}),
	)
	workload2 := cw(
		cwWithName(cwName2),
		cwWithContainers([]v1alpha2.Container{
			{
				Name:    containerName,
				Image:   containerImage,
				Command: []string{"sleep", "30s"},
				Ports: []v1alpha2.ContainerPort{
					v1alpha2.ContainerPort{
						Name: "http",
						Port: 80,
					},
				},
			},
		}),
	)
	rawWorkload2 := runtime.RawExtension{Object: workload2}
	co2 := comp(
		compWithName(compName2),
		compWithNamespace(namespace),
		compWithWorkload(rawWorkload2),
	)
	dummyApp := &v1alpha2.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dummy",
			Namespace: namespace,
		},
		Spec: v1alpha2.ApplicationSpec{
			Components: []v1alpha2.ApplicationComponent{},
		},
	}
	ar1 := &v1alpha2.ApplicationRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      arName1,
			Namespace: namespace,
		},
		Spec: v1alpha2.ApplicationRevisionSpec{
			Components: application.ConvertComponent2RawRevision([]*v1alpha2.Component{co1}),

			ApplicationConfiguration: util.Object2RawExtension(ac1),
			Application:              *dummyApp,
		}}
	trait2 := &v1alpha2.ManualScalerTrait{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.ManualScalerTraitKind,
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      traitName2,
			Namespace: namespace,
		},
		Spec: v1alpha2.ManualScalerTraitSpec{
			ReplicaCount: 3,
		},
	}
	rawTrait2 := runtime.RawExtension{Object: trait2}
	ac2 := ac(
		acWithName(acName2),
		acWithNamspace(namespace),
		acWithComps([]v1alpha2.ApplicationConfigurationComponent{
			{
				ComponentName: compName2,
				Traits: []v1alpha2.ComponentTrait{
					v1alpha2.ComponentTrait{
						Trait: rawTrait2,
					},
				},
			},
		}),
	)
	ar2 := &v1alpha2.ApplicationRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      arName2,
			Namespace: namespace,
		},
		Spec: v1alpha2.ApplicationRevisionSpec{
			ApplicationConfiguration: util.Object2RawExtension(ac2),
			Components:               application.ConvertComponent2RawRevision([]*v1alpha2.Component{co2}),
			Application:              *dummyApp,
		}}
	appContext := &v1alpha2.ApplicationContext{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appContextName,
			Namespace: namespace,
		},
		Spec: v1alpha2.ApplicationContextSpec{
			ApplicationRevisionName: arName1,
		},
	}

	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil()))
		Eventually(
			func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(BeNil())

		Expect(k8sClient.Create(ctx, co1)).Should(Succeed())
		Expect(k8sClient.Create(ctx, co2)).Should(Succeed())
		Expect(k8sClient.Create(ctx, ar1)).Should(Succeed())
		Expect(k8sClient.Create(ctx, ar2)).Should(Succeed())
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
		time.Sleep(15 * time.Second)
	})

	It("Test appContext reconcile logic ", func() {
		By("Test AppRevision1 only have 1 workload on trait")
		Expect(k8sClient.Create(ctx, appContext)).Should(Succeed())
		updateTime := time.Now()
		Eventually(func() error {
			appCtx := new(v1alpha2.ApplicationContext)
			err := k8sClient.Get(ctx, key, appCtx)
			if err != nil {
				return err
			}
			now := time.Now()
			if now.Sub(updateTime) > 4*time.Second {
				requestReconcileNow(ctx, appCtx)
				updateTime = now
			}
			if len(appCtx.Status.Workloads) != 1 {
				return fmt.Errorf("appContext status error:the number of workloads not right")
			}
			if appCtx.Status.Workloads[0].ComponentName != compName1 {
				return fmt.Errorf("appContext status error:the name of component not right, expect %s", compName1)
			}
			cw := new(v1alpha2.ContainerizedWorkload)
			return k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cwName1}, cw)
		}, time.Second*60, time.Millisecond*300).Should(BeNil())

		By("Test revision have both workload and trait , switch AppContext to revision2")
		Eventually(func() error {
			updateContext := new(v1alpha2.ApplicationContext)
			err := k8sClient.Get(ctx, key, updateContext)
			if err != nil {
				return err
			}
			updateContext.Spec.ApplicationRevisionName = arName2
			err = k8sClient.Update(ctx, updateContext)
			if err != nil {
				return err
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
		updateTime = time.Now()
		Eventually(func() error {
			appCtx := new(v1alpha2.ApplicationContext)
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appContextName}, appCtx)
			if err != nil {
				return err
			}
			now := time.Now()
			if now.Sub(updateTime) > 4*time.Second {
				requestReconcileNow(ctx, appCtx)
				updateTime = now
			}
			if len(appCtx.Status.Workloads) != 1 {
				return fmt.Errorf("appContext status error:the number of workloads not right")
			}
			if appCtx.Status.Workloads[0].ComponentName != compName2 {
				return fmt.Errorf("appContext status error:the name of component not right, expect %s", compName2)
			}
			cw := new(v1alpha2.ContainerizedWorkload)
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cwName2}, cw)
			if err != nil {
				return fmt.Errorf("cannot get workload 2 %v", err)
			}
			if len(appCtx.Status.Workloads[0].Traits) != 1 {
				return fmt.Errorf("appContext status error:the number of traits status not right")
			}
			mt := new(v1alpha2.ManualScalerTrait)
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: traitName2}, mt)
			if err != nil {
				return fmt.Errorf("cannot get component trait %v", err)
			}
			return nil
		}, time.Second*60, time.Millisecond*300).Should(BeNil())

		By("Test add trait in AppRevision1, and switch context to AppRevision1")

		trait1 := &v1alpha2.ManualScalerTrait{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha2.ManualScalerTraitKind,
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      traitName1,
				Namespace: namespace,
			},
			Spec: v1alpha2.ManualScalerTraitSpec{
				ReplicaCount: 2,
			},
		}
		rawTrait1 := runtime.RawExtension{Object: trait1}
		ac1.Spec.Components[0].Traits = []v1alpha2.ComponentTrait{
			v1alpha2.ComponentTrait{
				Trait: rawTrait1,
			},
		}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: arName1}, ar1)).Should(BeNil())
		ar1.Spec.ApplicationConfiguration = util.Object2RawExtension(ac1)
		Expect(k8sClient.Update(ctx, ar1)).Should(Succeed())
		Eventually(func() error {
			updateContext := new(v1alpha2.ApplicationContext)
			err := k8sClient.Get(ctx, key, updateContext)
			if err != nil {
				return err
			}
			updateContext.Spec.ApplicationRevisionName = arName1
			err = k8sClient.Update(ctx, updateContext)
			if err != nil {
				return err
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
		Eventually(func() error {
			mt := new(v1alpha2.ManualScalerTrait)
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: traitName1}, mt)
			if err != nil {
				return err
			}
			if mt.Spec.ReplicaCount != 2 {
				return fmt.Errorf("repica number missmatch , actual: %d", mt.Spec.ReplicaCount)
			}
			return nil
		}, time.Second*60, time.Millisecond*300).Should(BeNil())
		ac1.Spec.Components[0].Traits = []v1alpha2.ComponentTrait{}

		By("Test delete trait in AppRevision2, and switch context to AppRevision2")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: arName2}, ar2)).Should(BeNil())
		ac2.Spec.Components[0].Traits = []v1alpha2.ComponentTrait{}
		ar1.Spec.ApplicationConfiguration = util.Object2RawExtension(ac2)
		Expect(k8sClient.Update(ctx, ar2)).Should(BeNil())
		Eventually(func() error {
			updateContext := new(v1alpha2.ApplicationContext)
			err := k8sClient.Get(ctx, key, updateContext)
			if err != nil {
				return err
			}
			updateContext.Spec.ApplicationRevisionName = arName2
			err = k8sClient.Update(ctx, updateContext)
			if err != nil {
				return err
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
		Eventually(func() error {
			mt := new(v1alpha2.ManualScalerTrait)
			return k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: traitName2}, mt)
		}, time.Second*60, time.Millisecond*300).Should(util.NotFoundMatcher{})
	})
})
