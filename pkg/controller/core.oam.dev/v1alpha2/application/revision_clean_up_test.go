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

	"github.com/oam-dev/kubevela/pkg/oam/testutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test application controller clean up ", func() {
	ctx := context.TODO()
	var namespace string
	var ns v1.Namespace

	cd := &v1beta1.ComponentDefinition{}
	cdDefJson, _ := yaml.YAMLToJSON([]byte(normalCompDefYaml))

	BeforeEach(func() {
		namespace = randomNamespaceName("clean-up-revision-test")
		ns = v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(cdDefJson, cd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, cd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("[TEST] Clean up resources after an integration test")
		Expect(k8sClient.Delete(ctx, &ns)).Should(SatisfyAny(BeNil()))
	})

	PIt("Test clean up appRevision", func() {
		appName := "app-1"
		appKey := types.NamespacedName{Namespace: namespace, Name: appName}
		app := getApp(appName, namespace, "normal-worker")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		checkApp := new(v1beta1.Application)
		for i := 0; i < appRevisionLimit+1; i++ {
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			property := fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, i)
			checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
			Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
			testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
		}
		listOpts := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{
				oam.LabelAppName: appName,
			},
		}
		appRevisionList := new(v1beta1.ApplicationRevisionList)
		Eventually(func() error {
			err := k8sClient.List(ctx, appRevisionList, listOpts...)
			if err != nil {
				return err
			}
			if len(appRevisionList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error appRevison number wants %d, actually %d", appRevisionLimit+1, len(appRevisionList.Items))
			}
			return nil
		}, time.Second*30, time.Microsecond*300).Should(BeNil())

		By("create new appRevision will remove appRevison1")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		property := fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, 6)
		checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err := reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		deletedRevison := new(v1beta1.ApplicationRevision)
		revKey := types.NamespacedName{Namespace: namespace, Name: appName + "-v1"}
		Eventually(func() error {
			if _, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey}); err != nil {
				return err
			}
			err := k8sClient.List(ctx, appRevisionList, listOpts...)
			if err != nil {
				return err
			}
			if len(appRevisionList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error appRevison number wants %d, actually %d", appRevisionLimit+1, len(appRevisionList.Items))
			}
			err = k8sClient.Get(ctx, revKey, deletedRevison)
			if err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("haven't clean up the oldest revision")
			}
			return nil
		}, time.Second*30, time.Microsecond*300).Should(BeNil())

		By("update app again will gc appRevision2")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		property = fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, 7)
		checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		Eventually(func() error {
			if _, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey}); err != nil {
				return err
			}
			err := k8sClient.List(ctx, appRevisionList, listOpts...)
			if err != nil {
				return err
			}
			if len(appRevisionList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error appRevison number wants %d, actually %d", appRevisionLimit+1, len(appRevisionList.Items))
			}
			revKey = types.NamespacedName{Namespace: namespace, Name: appName + "-v2"}
			err = k8sClient.Get(ctx, revKey, deletedRevison)
			if err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("haven't clean up the  revision-2")
			}
			return nil
		}, time.Second*30, time.Microsecond*300).Should(BeNil())
	})

	PIt("Test clean up component revision", func() {
		appName := "app-1"
		appKey := types.NamespacedName{Namespace: namespace, Name: appName}
		app := getApp(appName, namespace, "normal-worker")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		checkApp := new(v1beta1.Application)
		for i := 0; i < appRevisionLimit+1; i++ {
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			property := fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, i)
			checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
			Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
			testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
		}
		listOpts := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{
				oam.LabelControllerRevisionComponent: "comp1",
			},
		}
		crList := new(appsv1.ControllerRevisionList)
		Eventually(func() error {
			err := k8sClient.List(ctx, crList, listOpts...)
			if err != nil {
				return err
			}
			if len(crList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error comp revision number wants %d, actually %d", appRevisionLimit+1, len(crList.Items))
			}
			return nil
		}, time.Second*10, time.Millisecond*500).Should(BeNil())

		By("create new appRevision will remove revision v1")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		property := fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, 6)
		checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err := reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		deletedRevison := new(v1beta1.ApplicationRevision)
		revKey := types.NamespacedName{Namespace: namespace, Name: "comp1-v1"}
		Eventually(func() error {
			if _, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey}); err != nil {
				return err
			}
			err := k8sClient.List(ctx, crList, listOpts...)
			if err != nil {
				return err
			}
			if len(crList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error comp revision number wants %d, actually %d", appRevisionLimit+1, len(crList.Items))
			}
			err = k8sClient.Get(ctx, revKey, deletedRevison)
			if err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("haven't clean up the oldest revision")
			}
			return nil
		}, time.Second*10, time.Millisecond*500).Should(BeNil())

		By("update app again will gc revision v2")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		property = fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, 7)
		checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		revKey = types.NamespacedName{Namespace: namespace, Name: "comp1-v2"}
		Eventually(func() error {
			if _, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey}); err != nil {
				return err
			}
			err := k8sClient.List(ctx, crList, listOpts...)
			if err != nil {
				return err
			}
			if len(crList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error comp revision number wants %d, actually %d", appRevisionLimit+1, len(crList.Items))
			}
			err = k8sClient.Get(ctx, revKey, deletedRevison)
			if err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("haven't clean up the oldest revision")
			}
			return nil
		}, time.Second*10, time.Millisecond*500).Should(BeNil())

		By("update app with comp as latest revision will not gc revision v3")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		property = fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, 6)
		checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		revKey = types.NamespacedName{Namespace: namespace, Name: "comp1-v3"}
		Eventually(func() error {
			if _, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey}); err != nil {
				return err
			}
			err := k8sClient.List(ctx, crList, listOpts...)
			if err != nil {
				return err
			}
			if len(crList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error comp revision number wants %d, actually %d", appRevisionLimit+1, len(crList.Items))
			}
			return k8sClient.Get(ctx, revKey, &appsv1.ControllerRevision{})
		}, time.Second*10, time.Millisecond*500).Should(BeNil())
	})

	It("Test clean up rollout component revision", func() {
		appName := "app-2"
		appKey := types.NamespacedName{Namespace: namespace, Name: appName}
		app := getApp(appName, namespace, "normal-worker")
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationAppRollout, "true")
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationRollingComponent, "comp1")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		checkApp := new(v1beta1.Application)
		for i := 0; i < appRevisionLimit+1; i++ {
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			property := fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, i)
			checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
			Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
			testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
		}
		listOpts := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{
				oam.LabelControllerRevisionComponent: "comp1",
			},
		}
		crList := new(appsv1.ControllerRevisionList)
		Eventually(func() error {
			err := k8sClient.List(ctx, crList, listOpts...)
			if err != nil {
				return err
			}
			if len(crList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error comp revision number wants %d, actually %d", appRevisionLimit+1, len(crList.Items))
			}
			return nil
		}, time.Second*10, time.Millisecond*500).Should(BeNil())

		By("create new appRevision will remove revision v1")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		property := fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, 6)
		checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err := reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		deletedRevison := new(v1beta1.ApplicationRevision)
		revKey := types.NamespacedName{Namespace: namespace, Name: "comp1-v1"}
		Eventually(func() error {
			if _, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey}); err != nil {
				return err
			}
			err := k8sClient.List(ctx, crList, listOpts...)
			if err != nil {
				return err
			}
			if len(crList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error comp revision number wants %d, actually %d", appRevisionLimit+1, len(crList.Items))
			}
			err = k8sClient.Get(ctx, revKey, deletedRevison)
			if err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("haven't clean up the oldest revision")
			}
			return nil
		}, time.Second*10, time.Millisecond*500).Should(BeNil())

		By("update app again will gc revision v2")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		property = fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, 7)
		checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		revKey = types.NamespacedName{Namespace: namespace, Name: "comp1-v2"}
		Eventually(func() error {
			if _, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey}); err != nil {
				return err
			}
			err := k8sClient.List(ctx, crList, listOpts...)
			if err != nil {
				return err
			}
			if len(crList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error comp revision number wants %d, actually %d", appRevisionLimit+1, len(crList.Items))
			}
			err = k8sClient.Get(ctx, revKey, deletedRevison)
			if err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("haven't clean up the oldest revision")
			}
			return nil
		}, time.Second*10, time.Millisecond*500).Should(BeNil())

		By("update app with comp as latest revision will not gc revision v3")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		property = fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, 6)
		checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		revKey = types.NamespacedName{Namespace: namespace, Name: "comp1-v3"}
		Eventually(func() error {
			if _, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey}); err != nil {
				return err
			}
			err := k8sClient.List(ctx, crList, listOpts...)
			if err != nil {
				return err
			}
			if len(crList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error comp revision number wants %d, actually %d", appRevisionLimit+1, len(crList.Items))
			}
			return k8sClient.Get(ctx, revKey, &appsv1.ControllerRevision{})
		}, time.Second*10, time.Millisecond*500).Should(BeNil())
	})

	It("Test clean up rollout appRevision", func() {
		appName := "app-2"
		appKey := types.NamespacedName{Namespace: namespace, Name: appName}
		app := getApp(appName, namespace, "normal-worker")
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationAppRollout, "true")
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationRollingComponent, "comp1")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		checkApp := new(v1beta1.Application)
		for i := 0; i < appRevisionLimit+1; i++ {
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			property := fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, i)
			checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
			Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
			testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
		}
		listOpts := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{
				oam.LabelAppName: appName,
			},
		}
		appRevisionList := new(v1beta1.ApplicationRevisionList)
		Eventually(func() error {
			err := k8sClient.List(ctx, appRevisionList, listOpts...)
			if err != nil {
				return err
			}
			if len(appRevisionList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error appRevison number wants %d, actually %d", appRevisionLimit+1, len(appRevisionList.Items))
			}
			return nil
		}, time.Second*30, time.Microsecond*300).Should(BeNil())

		By("create new appRevision will remove appRevison1")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		property := fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, 6)
		checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err := reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		deletedRevison := new(v1beta1.ApplicationRevision)
		revKey := types.NamespacedName{Namespace: namespace, Name: appName + "-v1"}
		Eventually(func() error {
			if _, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey}); err != nil {
				return err
			}
			err := k8sClient.List(ctx, appRevisionList, listOpts...)
			if err != nil {
				return err
			}
			if len(appRevisionList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error appRevison number wants %d, actually %d", appRevisionLimit+1, len(appRevisionList.Items))
			}
			err = k8sClient.Get(ctx, revKey, deletedRevison)
			if err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("haven't clean up the oldest revision")
			}
			if res, err := util.CheckAppRevision(appRevisionList.Items, []int{2, 3, 4, 5, 6, 7}); err != nil || !res {
				return fmt.Errorf("appRevision collection mismatch")
			}
			return nil
		}, time.Second*10, time.Second*2).Should(BeNil())

		By("update app again will gc appRevision2")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		property = fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, 7)
		checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		Eventually(func() error {
			if _, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey}); err != nil {
				return err
			}
			err := k8sClient.List(ctx, appRevisionList, listOpts...)
			if err != nil {
				return err
			}
			if len(appRevisionList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error appRevison number wants %d, actually %d", appRevisionLimit+1, len(appRevisionList.Items))
			}
			revKey = types.NamespacedName{Namespace: namespace, Name: appName + "-v2"}
			err = k8sClient.Get(ctx, revKey, deletedRevison)
			if err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("haven't clean up the  revision-2")
			}
			if res, err := util.CheckAppRevision(appRevisionList.Items, []int{3, 4, 5, 6, 7, 8}); err != nil || !res {
				return fmt.Errorf("appRevision collection mismatch")
			}
			return nil
		}, time.Second*30, time.Microsecond*300).Should(BeNil())
	})

	It("Test clean up rollout appRevision", func() {
		appName := "app-2"
		appKey := types.NamespacedName{Namespace: namespace, Name: appName}
		app := getApp(appName, namespace, "normal-worker")
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationAppRollout, "true")
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationRollingComponent, "comp1")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		checkApp := new(v1beta1.Application)
		for i := 0; i < appRevisionLimit+1; i++ {
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			property := fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, i)
			checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
			Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
			testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
		}
		listOpts := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{
				oam.LabelAppName: appName,
			},
		}
		appRevisionList := new(v1beta1.ApplicationRevisionList)
		Eventually(func() error {
			err := k8sClient.List(ctx, appRevisionList, listOpts...)
			if err != nil {
				return err
			}
			if len(appRevisionList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error appRevison number wants %d, actually %d", appRevisionLimit+1, len(appRevisionList.Items))
			}
			return nil
		}, time.Second*30, time.Microsecond*300).Should(BeNil())

		By("create new appRevision will remove appRevison1")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		property := fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, 6)
		checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err := reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		deletedRevison := new(v1beta1.ApplicationRevision)
		revKey := types.NamespacedName{Namespace: namespace, Name: appName + "-v1"}
		Eventually(func() error {
			if _, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey}); err != nil {
				return err
			}
			err := k8sClient.List(ctx, appRevisionList, listOpts...)
			if err != nil {
				return err
			}
			if len(appRevisionList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error appRevison number wants %d, actually %d", appRevisionLimit+1, len(appRevisionList.Items))
			}
			err = k8sClient.Get(ctx, revKey, deletedRevison)
			if err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("haven't clean up the oldest revision")
			}
			if res, err := util.CheckAppRevision(appRevisionList.Items, []int{2, 3, 4, 5, 6, 7}); err != nil || !res {
				return fmt.Errorf("appRevision collection mismatch")
			}
			return nil
		}, time.Second*10, time.Second*2).Should(BeNil())

		By("update app again will gc appRevision2")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		property = fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, 7)
		checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		Eventually(func() error {
			if _, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey}); err != nil {
				return err
			}
			err := k8sClient.List(ctx, appRevisionList, listOpts...)
			if err != nil {
				return err
			}
			if len(appRevisionList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error appRevison number wants %d, actually %d", appRevisionLimit+1, len(appRevisionList.Items))
			}
			revKey = types.NamespacedName{Namespace: namespace, Name: appName + "-v2"}
			err = k8sClient.Get(ctx, revKey, deletedRevison)
			if err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("haven't clean up the  revision-2")
			}
			if res, err := util.CheckAppRevision(appRevisionList.Items, []int{3, 4, 5, 6, 7, 8}); err != nil || !res {
				return fmt.Errorf("appRevision collection mismatch")
			}
			return nil
		}, time.Second*30, time.Microsecond*300).Should(BeNil())
	})

	It("Test clean up appDeployment using appRevision", func() {
		appName := "app-4"
		appKey := types.NamespacedName{Namespace: namespace, Name: appName}
		app := getApp(appName, namespace, "normal-worker")
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationAppRollout, "true")
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationRollingComponent, "comp1")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		checkApp := new(v1beta1.Application)
		for i := 0; i < appRevisionLimit+1; i++ {
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			property := fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, i)
			checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
			Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
			testutil.ReconcileOnceAfterFinalizer(reconciler, ctrl.Request{NamespacedName: appKey})
		}
		listOpts := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{
				oam.LabelAppName: appName,
			},
		}
		appRevisionList := new(v1beta1.ApplicationRevisionList)
		Eventually(func() error {
			err := k8sClient.List(ctx, appRevisionList, listOpts...)
			if err != nil {
				return err
			}
			if len(appRevisionList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error appRevison number wants %d, actually %d", appRevisionLimit+1, len(appRevisionList.Items))
			}
			return nil
		}, time.Second*30, time.Microsecond*300).Should(BeNil())

		By("create new appRevision will remove appRevison1")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		property := fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, 6)
		checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err := reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		deletedRevison := new(v1beta1.ApplicationRevision)
		revKey := types.NamespacedName{Namespace: namespace, Name: appName + "-v1"}
		Eventually(func() error {
			if _, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey}); err != nil {
				return err
			}
			err := k8sClient.List(ctx, appRevisionList, listOpts...)
			if err != nil {
				return err
			}
			if len(appRevisionList.Items) != appRevisionLimit+1 {
				return fmt.Errorf("error appRevison number wants %d, actually %d", appRevisionLimit+1, len(appRevisionList.Items))
			}
			err = k8sClient.Get(ctx, revKey, deletedRevison)
			if err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("haven't clean up the oldest revision")
			}
			if res, err := util.CheckAppRevision(appRevisionList.Items, []int{2, 3, 4, 5, 6, 7}); err != nil || !res {
				return fmt.Errorf("appRevision collection mismatch")
			}
			return nil
		}, time.Second*10, time.Second*2).Should(BeNil())

		By("update create appDeploy check gc logic")
		appDeploy := &v1beta1.AppDeployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1beta1.AppDeploymentKindAPIVersion,
				Kind:       v1beta1.AppDeploymentKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "app-deploy",
			},
			Spec: v1beta1.AppDeploymentSpec{
				AppRevisions: []v1beta1.AppRevision{
					{
						RevisionName: appName + "-v2",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, appDeploy)).Should(BeNil())
		// give informer some time to cache
		time.Sleep(2 * time.Second)
		for i := 7; i < 9; i++ {
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			property = fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, i)
			checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(property)}
			Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
			_, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey})
			Expect(err).Should(BeNil())
		}
		Eventually(func() error {
			if _, err = reconciler.Reconcile(context.TODO(), ctrl.Request{NamespacedName: appKey}); err != nil {
				return err
			}
			err := k8sClient.List(ctx, appRevisionList, listOpts...)
			if err != nil {
				return err
			}
			if len(appRevisionList.Items) != appRevisionLimit+2 {
				return fmt.Errorf("error appRevison number wants %d, actually %d", appRevisionLimit+2, len(appRevisionList.Items))
			}
			revKey = types.NamespacedName{Namespace: namespace, Name: appName + "-v3"}
			err = k8sClient.Get(ctx, revKey, deletedRevison)
			if err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("haven't clean up the  revision-3")
			}
			if res, err := util.CheckAppRevision(appRevisionList.Items, []int{2, 4, 5, 6, 7, 8, 9}); err != nil || !res {
				return fmt.Errorf("appRevision collection mismatch")
			}
			return nil
		}, time.Second*30, time.Microsecond*300).Should(BeNil())
	})
})
