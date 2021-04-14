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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ghodss/yaml"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test application controller clean up ", func() {
	ctx := context.TODO()
	namespace := "clean-up-revision"

	cd := &v1beta1.ComponentDefinition{}
	cdDefJson, _ := yaml.YAMLToJSON([]byte(normalCompDefYaml))

	BeforeEach(func() {
		ns := v1.Namespace{
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
	})

	It("Test clean up appRevision", func() {
		appName := "app-1"
		appKey := types.NamespacedName{Namespace: namespace, Name: appName}
		app := getApp(appName, namespace, "normal-worker")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		checkApp := new(v1beta1.Application)
		for i := 0; i < appRevisionLimit+1; i++ {
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			property := fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, i)
			checkApp.Spec.Components[0].Properties = runtime.RawExtension{Raw: []byte(property)}
			Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
			_, err := reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
			Expect(err).Should(BeNil())
		}
		appContext := new(v1alpha2.ApplicationContext)
		Expect(k8sClient.Get(ctx, appKey, appContext)).Should(BeNil())
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
		checkApp.Spec.Components[0].Properties = runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err := reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		deletedRevison := new(v1beta1.ApplicationRevision)
		revKey := types.NamespacedName{Namespace: namespace, Name: appName + "-v1"}
		Eventually(func() error {
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
		checkApp.Spec.Components[0].Properties = runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err = reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		Eventually(func() error {
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
			checkApp.Spec.Components[0].Properties = runtime.RawExtension{Raw: []byte(property)}
			Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
			_, err := reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
			Expect(err).Should(BeNil())
		}
		appContext := new(v1alpha2.ApplicationContext)
		Expect(k8sClient.Get(ctx, appKey, appContext)).Should(util.NotFoundMatcher{})
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
		checkApp.Spec.Components[0].Properties = runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err := reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		deletedRevison := new(v1beta1.ApplicationRevision)
		revKey := types.NamespacedName{Namespace: namespace, Name: appName + "-v1"}
		Eventually(func() error {
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
		}, time.Second*30, time.Microsecond*300).Should(BeNil())

		By("update app again will gc appRevision2")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		property = fmt.Sprintf(`{"cmd":["sleep","1000"],"image":"busybox:%d"}`, 7)
		checkApp.Spec.Components[0].Properties = runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err = reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		Eventually(func() error {
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
			checkApp.Spec.Components[0].Properties = runtime.RawExtension{Raw: []byte(property)}
			Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
			_, err := reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
			Expect(err).Should(BeNil())
		}
		appContext := new(v1alpha2.ApplicationContext)
		Expect(k8sClient.Get(ctx, appKey, appContext)).Should(util.NotFoundMatcher{})
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
		checkApp.Spec.Components[0].Properties = runtime.RawExtension{Raw: []byte(property)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		_, err := reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())
		deletedRevison := new(v1beta1.ApplicationRevision)
		revKey := types.NamespacedName{Namespace: namespace, Name: appName + "-v1"}
		Eventually(func() error {
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
		}, time.Second*30, time.Microsecond*300).Should(BeNil())

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
			checkApp.Spec.Components[0].Properties = runtime.RawExtension{Raw: []byte(property)}
			Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
			_, err = reconciler.Reconcile(ctrl.Request{NamespacedName: appKey})
			Expect(err).Should(BeNil())
		}
		Eventually(func() error {
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

var _ = Describe("Test gatherUsingAppRevision func", func() {
	ctx := context.TODO()
	namespace := "clean-up-revision"

	cd := &v1beta1.ComponentDefinition{}
	cdDefJson, _ := yaml.YAMLToJSON([]byte(normalCompDefYaml))

	BeforeEach(func() {
		ns := v1.Namespace{
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
	})

	It("get gatherUsingAppRevision func logic", func() {
		appName := "app-3"
		app := getApp(appName, namespace, "normal-worker")
		app.Status.LatestRevision = &common.Revision{
			Name: appName + "-v1",
		}
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appContext := getAppContext(namespace, appName+"-ctx", appName+"-v2")
		appContext.Labels = map[string]string{
			oam.LabelAppName: appName,
		}
		Expect(k8sClient.Create(ctx, appContext)).Should(BeNil())
		handler := appHandler{
			r:      reconciler,
			app:    app,
			logger: reconciler.Log.WithValues("application", "gatherUsingAppRevision-func-test"),
		}
		Eventually(func() error {
			using, err := gatherUsingAppRevision(ctx, &handler)
			if err != nil {
				return err
			}
			if len(using) != 2 {
				return fmt.Errorf("wrong revision number")
			}
			if !using[appName+"-v1"] {
				return fmt.Errorf("revison1 not include")
			}
			if !using[appName+"-v2"] {
				return fmt.Errorf("revison2 not include")
			}
			return nil
		}, time.Second*60, time.Microsecond).Should(BeNil())
	})
})

func getAppContext(namespace, name string, pointingRev string) *v1alpha2.ApplicationContext {
	return &v1alpha2.ApplicationContext{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.ApplicationContextKindAPIVersion,
			Kind:       v1alpha2.ApplicationContextKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "app-rollout",
		},
		Spec: v1alpha2.ApplicationContextSpec{
			ApplicationRevisionName: pointingRev,
		},
	}
}
