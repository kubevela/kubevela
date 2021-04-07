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
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test application cross namespace resource", func() {
	ctx := context.Background()
	var (
		namespace      = "app-resource-tracker-test-ns"
		crossNamespace = "cross-namespace"
	)

	BeforeEach(func() {
		crossNs := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: crossNamespace}}
		ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, &crossNs)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Eventually(func() error {
			ns := new(corev1.Namespace)
			return k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns)
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
		Eventually(func() error {
			ns := new(corev1.Namespace)
			return k8sClient.Get(ctx, types.NamespacedName{Name: crossNamespace}, ns)
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: crossNamespace}}, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
		// guarantee namespace have been deleted
		Eventually(func() error {
			ns := new(corev1.Namespace)
			err := k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns)
			if err == nil {
				return fmt.Errorf("namespace still exist")
			}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: crossNamespace}, ns)
			if err == nil {
				return fmt.Errorf("namespace still exist")
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
	})

	It("Test application have  cross-namespace workload", func() {
		// install  component definition
		crossCdJson, _ := yaml.YAMLToJSON([]byte(crossCompDefYaml))
		ccd := new(v1beta1.ComponentDefinition)
		Expect(json.Unmarshal(crossCdJson, ccd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ccd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		var (
			appName       = "test-app-1"
			app           = new(v1beta1.Application)
			componentName = "test-app-1-comp"
		)
		app = &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					v1beta1.ApplicationComponent{
						Name:       componentName,
						Type:       "cross-worker",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		By("check resource tracker has been created and app status ")
		resourceTracker := new(v1beta1.ResourceTracker)
		Eventually(func() error {
			app := new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("app not found %v", err)
			}
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker); err != nil {
				return err
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status is not running")
			}
			if app.Status.ResourceTracker == nil || app.Status.ResourceTracker.UID != resourceTracker.UID {
				return fmt.Errorf("appication status error ")
			}
			return nil
		}, time.Second*300, time.Microsecond*300).Should(BeNil())
		By("check resource is generated correctly")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
		var workload appsv1.Deployment
		Eventually(func() error {
			appContext := &v1alpha2.ApplicationContext{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, appContext); err != nil {
				return fmt.Errorf("cannot generate AppContext %v", err)
			}
			checkRt := new(v1beta1.ResourceTracker)
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), checkRt); err != nil {
				return err
			}
			component := &v1alpha2.Component{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: componentName}, component); err != nil {
				return fmt.Errorf("cannot generate component %v", err)
			}
			if component.ObjectMeta.Labels[oam.LabelAppName] != appName {
				return fmt.Errorf("component error label ")
			}
			depolys := new(appsv1.DeploymentList)
			opts := []client.ListOption{
				client.InNamespace(crossNamespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			err := k8sClient.List(ctx, depolys, opts...)
			if err != nil || len(depolys.Items) != 1 {
				return fmt.Errorf("error workload number %v", err)
			}
			workload = depolys.Items[0]
			if len(workload.OwnerReferences) != 1 || workload.OwnerReferences[0].UID != resourceTracker.UID {
				return fmt.Errorf("wrokload ownerreference error")
			}
			if len(checkRt.Status.TrackedResources) != 1 {
				return fmt.Errorf("resourceTracker status recode trackedResource length missmatch")
			}
			if checkRt.Status.TrackedResources[0].Name != workload.Name {
				return fmt.Errorf("resourceTracker status recode trackedResource name mismatch recorded %s, actually %s", checkRt.Status.TrackedResources[0].Name, workload.Name)
			}
			return nil
		}, time.Second*50, time.Microsecond*300).Should(BeNil())

		By("deleting application will remove resourceTracker and related workload will be removed")
		time.Sleep(3 * time.Second) // wait informer cache to be synced
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
		Eventually(func() error {
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err == nil {
				return fmt.Errorf("resourceTracker still exist")
			}
			if !apierrors.IsNotFound(err) {
				return err
			}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: crossNamespace, Name: workload.GetName()}, &workload)
			if err == nil {
				return fmt.Errorf("wrokload still exist")
			}
			if !apierrors.IsNotFound(err) {
				return err
			}
			return nil
		}, time.Second*30, time.Microsecond*300).Should(BeNil())
	})

	It("Test update application by add  a cross namespace trait resource", func() {
		var (
			appName       = "test-app-2"
			app           = new(v1beta1.Application)
			componentName = "test-app-2-comp"
		)
		// install component definition
		normalCdJson, _ := yaml.YAMLToJSON([]byte(normalCompDefYaml))
		ncd := new(v1beta1.ComponentDefinition)
		Expect(json.Unmarshal(normalCdJson, ncd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ncd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		crossTdJson, err := yaml.YAMLToJSON([]byte(crossNsTdYaml))
		Expect(err).Should(BeNil())
		ctd := new(v1beta1.TraitDefinition)
		Expect(json.Unmarshal(crossTdJson, ctd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ctd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		app = &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					v1beta1.ApplicationComponent{
						Name:       componentName,
						Type:       "normal-worker",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		resourceTracker := new(v1beta1.ResourceTracker)
		By("application contain a normal workload, check application and workload status")
		Eventually(func() error {
			app := new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("error to create application %v", err)
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status not running")
			}
			depolys := new(appsv1.DeploymentList)
			opts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			err := k8sClient.List(ctx, depolys, opts...)
			if err != nil || len(depolys.Items) != 1 {
				return fmt.Errorf("error workload number %v", err)
			}
			workload := depolys.Items[0]
			if len(workload.OwnerReferences) != 1 || workload.OwnerReferences[0].Kind != v1alpha2.ApplicationContextKind {
				return fmt.Errorf("workload owneRefernece err")
			}
			err = k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err == nil {
				return fmt.Errorf("resourceTracker should not be created")
			}
			if !apierrors.IsNotFound(err) {
				return err
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)
			if err != nil {
				return err
			}
			app.Spec.Components[0].Traits = []v1beta1.ApplicationTrait{
				v1beta1.ApplicationTrait{
					Type:       "cross-scaler",
					Properties: runtime.RawExtension{Raw: []byte(`{"replicas": 1}`)},
				},
			}
			return k8sClient.Update(ctx, app)
		}, time.Second*30, time.Microsecond*300).Should(BeNil())

		By("add a cross namespace trait, check resourceTracker and trait status")
		Eventually(func() error {
			app := new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("error to get application %v", err)
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status not running")
			}
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err != nil {
				return fmt.Errorf("resourceTracker not generated %v", err)
			}
			mts := new(v1alpha2.ManualScalerTraitList)
			opts := []client.ListOption{
				client.InNamespace(crossNamespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			err = k8sClient.List(ctx, mts, opts...)
			if err != nil || len(mts.Items) != 1 {
				return fmt.Errorf("failed generate cross namespace trait")
			}
			trait := mts.Items[0]
			if len(trait.OwnerReferences) != 1 || trait.OwnerReferences[0].UID != resourceTracker.UID {
				return fmt.Errorf("trait owner reference missmatch")
			}
			if len(resourceTracker.Status.TrackedResources) != 1 {
				return fmt.Errorf("resourceTracker status recode trackedResource length missmatch")
			}
			if resourceTracker.Status.TrackedResources[0].Name != trait.Name {
				return fmt.Errorf("resourceTracker status recode trackedResource name mismatch recorded %s, actually %s", resourceTracker.Status.TrackedResources[0].Name, trait.Name)
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
	})

	It("Test update application by delete a cross namespace trait resource", func() {
		var (
			appName       = "test-app-3"
			app           = new(v1beta1.Application)
			componentName = "test-app-3-comp"
		)
		By("install component definition")
		normalCdJson, _ := yaml.YAMLToJSON([]byte(normalCompDefYaml))
		ncd := new(v1beta1.ComponentDefinition)
		Expect(json.Unmarshal(normalCdJson, ncd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ncd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		crossTdJson, err := yaml.YAMLToJSON([]byte(crossNsTdYaml))
		Expect(err).Should(BeNil())
		ctd := new(v1beta1.TraitDefinition)
		Expect(json.Unmarshal(crossTdJson, ctd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ctd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		app = &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					v1beta1.ApplicationComponent{
						Name:       componentName,
						Type:       "normal-worker",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						Traits: []v1beta1.ApplicationTrait{
							v1beta1.ApplicationTrait{
								Type:       "cross-scaler",
								Properties: runtime.RawExtension{Raw: []byte(`{"replicas": 1}`)},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		time.Sleep(3 * time.Second) // give informer cache to sync
		resourceTracker := new(v1beta1.ResourceTracker)
		By("create application will create a cross ns trait, and resourceTracker. check those status")
		Eventually(func() error {
			app = new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("error to get application %v", err)
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status not running")
			}
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err != nil {
				return fmt.Errorf("error to get resourceTracker %v", err)
			}
			mts := new(v1alpha2.ManualScalerTraitList)
			opts := []client.ListOption{
				client.InNamespace(crossNamespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			err = k8sClient.List(ctx, mts, opts...)
			if err != nil || len(mts.Items) != 1 {
				return fmt.Errorf("failed generate cross namespace trait")
			}
			trait := mts.Items[0]
			if len(trait.OwnerReferences) != 1 || trait.OwnerReferences[0].UID != resourceTracker.UID {
				return fmt.Errorf("trait owner reference missmatch")
			}
			if len(resourceTracker.Status.TrackedResources) != 1 {
				return fmt.Errorf("resourceTracker status recode trackedResource length missmatch")
			}
			if resourceTracker.Status.TrackedResources[0].Name != trait.Name {
				return fmt.Errorf("resourceTracker status recode trackedResource name mismatch recorded %s, actually %s", resourceTracker.Status.TrackedResources[0].Name, trait.Name)
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())

		By("update application trait by delete cross ns trait, will delete resourceTracker and related trait resource")
		Eventually(func() error {
			app = new(v1beta1.Application)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
			app.Spec.Components[0].Traits = []v1beta1.ApplicationTrait{}
			return k8sClient.Update(ctx, app)
		}, time.Second*30, time.Microsecond*300).Should(BeNil())
		fmt.Println(app.ResourceVersion)
		Eventually(func() error {
			app = new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("error to get application %v", err)
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status not running")
			}
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err == nil {
				return fmt.Errorf("resourceTracker still exist")
			}
			mts := new(v1alpha2.ManualScalerTraitList)
			opts := []client.ListOption{
				client.InNamespace(crossNamespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			err = k8sClient.List(ctx, mts, opts...)
			if err != nil || len(mts.Items) != 0 {
				return fmt.Errorf("cross ns trait still exist")
			}
			if app.Status.ResourceTracker != nil {
				return fmt.Errorf("application status resourceTracker field still exist %s", string(util.JSONMarshal(app.Status.ResourceTracker)))
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
	})

	It("Test application have two different workload", func() {
		var (
			appName        = "test-app-4"
			app            = new(v1beta1.Application)
			component1Name = "test-app-4-comp-1"
			component2Name = "test-app-4-comp-2"
		)
		By("install component definition")
		normalCdJson, _ := yaml.YAMLToJSON([]byte(normalCompDefYaml))
		ncd := new(v1beta1.ComponentDefinition)
		Expect(json.Unmarshal(normalCdJson, ncd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ncd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		crossCdJson, err := yaml.YAMLToJSON([]byte(crossCompDefYaml))
		Expect(err).Should(BeNil())
		ctd := new(v1beta1.ComponentDefinition)
		Expect(json.Unmarshal(crossCdJson, ctd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ctd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		app = &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					v1beta1.ApplicationComponent{
						Name:       component1Name,
						Type:       "normal-worker",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					v1beta1.ApplicationComponent{
						Name:       component2Name,
						Type:       "cross-worker",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		time.Sleep(3 * time.Second) // give informer cache to sync
		resourceTracker := new(v1beta1.ResourceTracker)

		By("create application will generate two workload, and generate resourceTracker")
		Eventually(func() error {
			app = new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("error to get application %v", err)
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status not running")
			}
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err != nil {
				return fmt.Errorf("error to generate resourceTracker %v", err)
			}
			sameOpts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			crossOpts := []client.ListOption{
				client.InNamespace(crossNamespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			same, cross := new(appsv1.DeploymentList), new(appsv1.DeploymentList)
			err = k8sClient.List(ctx, same, sameOpts...)
			if err != nil || len(same.Items) != 1 {
				return fmt.Errorf("failed generate same namespace workload")
			}
			sameDeplpoy := same.Items[0]
			if len(sameDeplpoy.OwnerReferences) != 1 || sameDeplpoy.OwnerReferences[0].Kind != v1alpha2.ApplicationContextKind {
				return fmt.Errorf("same ns deploy have error ownerReference")
			}
			err = k8sClient.List(ctx, cross, crossOpts...)
			if err != nil || len(cross.Items) != 1 {
				return fmt.Errorf("failed generate cross namespace trait")
			}
			crossDeplpoy := cross.Items[0]
			if len(sameDeplpoy.OwnerReferences) != 1 || crossDeplpoy.OwnerReferences[0].UID != resourceTracker.UID {
				return fmt.Errorf("same ns deploy have error ownerReference")
			}
			if app.Status.ResourceTracker == nil || app.Status.ResourceTracker.UID != resourceTracker.UID {
				return fmt.Errorf("app status resourceTracker error")
			}
			if len(resourceTracker.Status.TrackedResources) != 1 {
				return fmt.Errorf("resourceTracker status recode trackedResource length missmatch")
			}
			if resourceTracker.Status.TrackedResources[0].Name != crossDeplpoy.Name {
				return fmt.Errorf("resourceTracker status recode trackedResource name mismatch recorded %s, actually %s", resourceTracker.Status.TrackedResources[0].Name, crossDeplpoy.Name)
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
		By("update application by delete cross namespace workload, resource tracker will be deleted, then check app status")
		Eventually(func() error {
			app = new(v1beta1.Application)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
			app.Spec.Components = app.Spec.Components[:1] // delete a component
			return k8sClient.Update(ctx, app)
		}, time.Second*30, time.Microsecond*300).Should(BeNil())
		Eventually(func() error {
			app = new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("error to get application %v", err)
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status not running")
			}
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err == nil {
				return fmt.Errorf("resourceTracker still exist")
			}
			sameOpts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			crossOpts := []client.ListOption{
				client.InNamespace(crossNamespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			same, cross := new(appsv1.DeploymentList), new(appsv1.DeploymentList)
			err = k8sClient.List(ctx, same, sameOpts...)
			if err != nil || len(same.Items) != 1 {
				return fmt.Errorf("failed generate same namespace workload")
			}
			sameDeplpoy := same.Items[0]
			if len(sameDeplpoy.OwnerReferences) != 1 || sameDeplpoy.OwnerReferences[0].Kind != v1alpha2.ApplicationContextKind {
				return fmt.Errorf("same ns deploy have error ownerReference")
			}
			err = k8sClient.List(ctx, cross, crossOpts...)
			if err != nil || len(cross.Items) != 0 {
				return fmt.Errorf("error : cross namespace workload still exist")
			}
			if app.Status.ResourceTracker != nil {
				return fmt.Errorf("error app status resourceTracker")
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
	})

	It("Update a cross namespace workload of application", func() {
		// install  component definition
		crossCdJson, _ := yaml.YAMLToJSON([]byte(crossCompDefYaml))
		ccd := new(v1beta1.ComponentDefinition)
		Expect(json.Unmarshal(crossCdJson, ccd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ccd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		var (
			appName       = "test-app-5"
			app           = new(v1beta1.Application)
			componentName = "test-app-5-comp"
		)
		app = &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					v1beta1.ApplicationComponent{
						Name:       componentName,
						Type:       "cross-worker",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		By("check resource tracker has been created and app status ")
		resourceTracker := new(v1beta1.ResourceTracker)
		Eventually(func() error {
			app := new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("app not found %v", err)
			}
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker); err != nil {
				return err
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status is not running")
			}
			if app.Status.ResourceTracker == nil || app.Status.ResourceTracker.UID != resourceTracker.UID {
				return fmt.Errorf("appication status error ")
			}
			return nil
		}, time.Second*600, time.Microsecond*300).Should(BeNil())
		By("check resource is generated correctly")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
		var workload appsv1.Deployment
		Eventually(func() error {
			appContext := &v1alpha2.ApplicationContext{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, appContext); err != nil {
				return fmt.Errorf("cannot generate AppContext %v", err)
			}
			component := &v1alpha2.Component{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: componentName}, component); err != nil {
				return fmt.Errorf("cannot generate component %v", err)
			}
			if component.ObjectMeta.Labels[oam.LabelAppName] != appName {
				return fmt.Errorf("component error label ")
			}
			depolys := new(appsv1.DeploymentList)
			opts := []client.ListOption{
				client.InNamespace(crossNamespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			err := k8sClient.List(ctx, depolys, opts...)
			if err != nil || len(depolys.Items) != 1 {
				return fmt.Errorf("error workload number %v", err)
			}
			workload = depolys.Items[0]
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker); err != nil {
				return err
			}
			if len(workload.OwnerReferences) != 1 || workload.OwnerReferences[0].UID != resourceTracker.UID {
				return fmt.Errorf("wrokload ownerreference error")
			}
			if workload.Spec.Template.Spec.Containers[0].Image != "busybox" {
				return fmt.Errorf("container image not match")
			}
			if len(resourceTracker.Status.TrackedResources) != 1 {
				return fmt.Errorf("resourceTracker status recode trackedResource length missmatch")
			}
			if resourceTracker.Status.TrackedResources[0].Name != workload.Name {
				return fmt.Errorf("resourceTracker status recode trackedResource name mismatch recorded %s, actually %s", resourceTracker.Status.TrackedResources[0].Name, workload.Name)
			}
			return nil
		}, time.Second*50, time.Microsecond*300).Should(BeNil())

		By("update application and check resource status")
		Eventually(func() error {
			checkApp := new(v1beta1.Application)
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, checkApp)
			if err != nil {
				return err
			}
			checkApp.Spec.Components[0].Properties = runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"nginx"}`)}
			err = k8sClient.Update(ctx, checkApp)
			if err != nil {
				return err
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())

		Eventually(func() error {
			appContext := &v1alpha2.ApplicationContext{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, appContext); err != nil {
				return fmt.Errorf("cannot generate AppContext %v", err)
			}
			component := &v1alpha2.Component{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: componentName}, component); err != nil {
				return fmt.Errorf("cannot generate component %v", err)
			}
			if component.ObjectMeta.Labels[oam.LabelAppName] != appName {
				return fmt.Errorf("component error label ")
			}
			depolys := new(appsv1.DeploymentList)
			opts := []client.ListOption{
				client.InNamespace(crossNamespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			err := k8sClient.List(ctx, depolys, opts...)
			if err != nil || len(depolys.Items) != 1 {
				return fmt.Errorf("error workload number %v", err)
			}
			workload = depolys.Items[0]
			if len(workload.OwnerReferences) != 1 || workload.OwnerReferences[0].UID != resourceTracker.UID {
				return fmt.Errorf("wrokload ownerreference error")
			}
			if workload.Spec.Template.Spec.Containers[0].Image != "nginx" {
				return fmt.Errorf("container image not match")
			}
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker); err != nil {
				return err
			}
			if len(resourceTracker.Status.TrackedResources) != 1 {
				return fmt.Errorf("resourceTracker status recode trackedResource length missmatch")
			}
			if resourceTracker.Status.TrackedResources[0].Name != workload.Name {
				return fmt.Errorf("resourceTracker status recode trackedResource name mismatch recorded %s, actually %s", resourceTracker.Status.TrackedResources[0].Name, workload.Name)
			}
			return nil
		}, time.Second*60, time.Microsecond*1000).Should(BeNil())

		By("deleting application will remove resourceTracker and related workload will be removed")
		time.Sleep(3 * time.Second) // wait informer cache to be synced
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
		Eventually(func() error {
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err == nil {
				return fmt.Errorf("resourceTracker still exist")
			}
			if !apierrors.IsNotFound(err) {
				return err
			}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: crossNamespace, Name: workload.GetName()}, &workload)
			if err == nil {
				return fmt.Errorf("wrokload still exist")
			}
			if !apierrors.IsNotFound(err) {
				return err
			}
			return nil
		}, time.Second*30, time.Microsecond*300).Should(BeNil())
	})

	It("Test cross-namespace resource gc logic, delete a cross-ns component", func() {
		var (
			appName        = "test-app-6"
			app            = new(v1beta1.Application)
			component1Name = "test-app-6-comp-1"
			component2Name = "test-app-6-comp-2"
		)
		By("install related definition")

		crossCdJson, err := yaml.YAMLToJSON([]byte(crossCompDefYaml))
		Expect(err).Should(BeNil())
		ctd := new(v1beta1.ComponentDefinition)
		Expect(json.Unmarshal(crossCdJson, ctd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ctd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		app = &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					v1beta1.ApplicationComponent{
						Name:       component1Name,
						Type:       "cross-worker",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					v1beta1.ApplicationComponent{
						Name:       component2Name,
						Type:       "cross-worker",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		time.Sleep(3 * time.Second) // give informer cache to sync
		resourceTracker := new(v1beta1.ResourceTracker)

		By("create application will generate two workload, and generate resourceTracker")
		Eventually(func() error {
			app = new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("error to get application %v", err)
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status not running")
			}
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err != nil {
				return fmt.Errorf("error to generate resourceTracker %v", err)
			}
			crossOpts := []client.ListOption{
				client.InNamespace(crossNamespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			//same, cross := new(appsv1.DeploymentList), new(appsv1.DeploymentList)
			workloads := new(appsv1.DeploymentList)
			err = k8sClient.List(ctx, workloads, crossOpts...)
			if err != nil || len(workloads.Items) != 2 {
				return fmt.Errorf("failed get workloads")
			}
			deploy1 := workloads.Items[0]
			if len(deploy1.OwnerReferences) != 1 || deploy1.OwnerReferences[0].Kind != v1beta1.ResourceTrackerKind {
				return fmt.Errorf("deploy1 have error ownerReference")
			}
			deploy2 := workloads.Items[1]
			if len(deploy2.OwnerReferences) != 1 || deploy2.OwnerReferences[0].UID != resourceTracker.UID {
				return fmt.Errorf("deploy2 have error ownerReference")
			}
			if app.Status.ResourceTracker == nil || app.Status.ResourceTracker.UID != resourceTracker.UID {
				return fmt.Errorf("app status resourceTracker error")
			}
			if len(resourceTracker.Status.TrackedResources) != 2 {
				return fmt.Errorf("resourceTracker status recode trackedResource length missmatch")
			}
			if resourceTracker.Status.TrackedResources[0].Namespace != crossNamespace || resourceTracker.Status.TrackedResources[1].Namespace != crossNamespace {
				return fmt.Errorf("resourceTracker recorde namespace mismatch")
			}
			if resourceTracker.Status.TrackedResources[0].Name != deploy1.Name && resourceTracker.Status.TrackedResources[1].Name != deploy1.Name {
				return fmt.Errorf("resourceTracker status recode trackedResource name mismatch recorded %s, actually %s", resourceTracker.Status.TrackedResources[0].Name, deploy1.Name)
			}
			if resourceTracker.Status.TrackedResources[0].Name != deploy2.Name && resourceTracker.Status.TrackedResources[1].Name != deploy2.Name {
				return fmt.Errorf("resourceTracker status recode trackedResource name mismatch recorded %s, actually %s", resourceTracker.Status.TrackedResources[0].Name, deploy2.Name)
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
		By("update application by delete a cross namespace workload, resource tracker will still exist, then check app status")
		Eventually(func() error {
			app = new(v1beta1.Application)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
			app.Spec.Components = app.Spec.Components[:1] // delete a component
			return k8sClient.Update(ctx, app)
		}, time.Second*30, time.Microsecond*300).Should(BeNil())
		Eventually(func() error {
			app = new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("error to get application %v", err)
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status not running")
			}
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err != nil {
				return fmt.Errorf("failed to get resourceTracker %v", err)
			}
			crossOpts := []client.ListOption{
				client.InNamespace(crossNamespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			workloads := new(appsv1.DeploymentList)
			err = k8sClient.List(ctx, workloads, crossOpts...)
			if err != nil || len(workloads.Items) != 1 {
				return fmt.Errorf("failed get cross namespace workload")
			}
			deploy := workloads.Items[0]
			if len(deploy.OwnerReferences) != 1 || deploy.OwnerReferences[0].Kind != v1beta1.ResourceTrackerKind {
				return fmt.Errorf("same ns deploy have error ownerReference")
			}
			checkRt := new(v1beta1.ResourceTracker)
			err = k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), checkRt)
			if err != nil {
				return fmt.Errorf("error get resourceTracker")
			}
			if app.Status.ResourceTracker == nil {
				return fmt.Errorf("app status resourceTracker error")
			}
			if app.Status.ResourceTracker.UID != checkRt.UID {
				return fmt.Errorf("error app status resourceTracker UID")
			}
			if len(checkRt.Status.TrackedResources) != 1 {
				return fmt.Errorf("error resourceTracker  status trackedResource")
			}
			return nil
		}, time.Second*80, time.Microsecond*300).Should(BeNil())

		By("deleting application will remove resourceTracker and related resourceTracker will be removed")
		app = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
		Eventually(func() error {
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err == nil {
				return fmt.Errorf("resourceTracker still exist")
			}
			if !apierrors.IsNotFound(err) {
				return err
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
	})

	It("Test cross-namespace resource gc logic, delete a cross-ns trait", func() {
		var (
			appName       = "test-app-7"
			app           = new(v1beta1.Application)
			componentName = "test-app-7-comp"
		)
		By("install related definition")

		crossCdJson, err := yaml.YAMLToJSON([]byte(crossCompDefYaml))
		Expect(err).Should(BeNil())
		ctd := new(v1beta1.ComponentDefinition)
		Expect(json.Unmarshal(crossCdJson, ctd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ctd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		crossTdJson, err := yaml.YAMLToJSON([]byte(crossNsTdYaml))
		Expect(err).Should(BeNil())
		td := new(v1beta1.TraitDefinition)
		Expect(json.Unmarshal(crossTdJson, td)).Should(BeNil())
		Expect(k8sClient.Create(ctx, td)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		app = &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					v1beta1.ApplicationComponent{
						Name:       componentName,
						Type:       "cross-worker",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						Traits: []v1beta1.ApplicationTrait{
							v1beta1.ApplicationTrait{
								Type:       "cross-scaler",
								Properties: runtime.RawExtension{Raw: []byte(`{"replicas": 0}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		time.Sleep(3 * time.Second) // give informer cache to sync
		resourceTracker := new(v1beta1.ResourceTracker)
		By("create app and check resource and app status")
		Eventually(func() error {
			app = new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("error to get application %v", err)
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status not running")
			}
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err != nil {
				return fmt.Errorf("error to get resourceTracker %v", err)
			}
			mts := new(v1alpha2.ManualScalerTraitList)
			opts := []client.ListOption{
				client.InNamespace(crossNamespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			err = k8sClient.List(ctx, mts, opts...)
			if err != nil || len(mts.Items) != 1 {
				return fmt.Errorf("failed generate cross namespace trait")
			}
			if len(resourceTracker.Status.TrackedResources) != 2 {
				return fmt.Errorf("resourceTracker status recode trackedResource length missmatch")
			}
			trait := mts.Items[0]
			if len(trait.OwnerReferences) != 1 || trait.OwnerReferences[0].UID != resourceTracker.UID {
				return fmt.Errorf("trait owner reference missmatch")
			}
			deploys := new(appsv1.DeploymentList)
			err = k8sClient.List(ctx, deploys, opts...)
			if err != nil || len(deploys.Items) != 1 {
				return fmt.Errorf("error to list deploy")
			}
			deploy := deploys.Items[0]
			if deploy.OwnerReferences[0].UID != resourceTracker.UID {
				return fmt.Errorf("deploy owner reference missmatch")
			}
			for _, resource := range resourceTracker.Status.TrackedResources {
				if resource.Kind == deploy.Kind && resource.Name != deploy.Name {
					return fmt.Errorf("deploy name mismatch ")
				}
				if resource.Kind == trait.Kind && resource.Name != trait.Name {
					return fmt.Errorf("trait name mismatch")
				}
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())

		By("update application trait by delete cross ns trait, resourceTracker will still exist")
		Eventually(func() error {
			app = new(v1beta1.Application)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
			app.Spec.Components[0].Traits = []v1beta1.ApplicationTrait{}
			return k8sClient.Update(ctx, app)
		}, time.Second*30, time.Microsecond*300).Should(BeNil())

		Eventually(func() error {
			app = new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("error to get application %v", err)
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status not running")
			}
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err != nil {
				return fmt.Errorf("error to get resourceTracker %v", err)
			}
			mts := new(v1alpha2.ManualScalerTraitList)
			opts := []client.ListOption{
				client.InNamespace(crossNamespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			err = k8sClient.List(ctx, mts, opts...)
			if err != nil || len(mts.Items) != 0 {
				return fmt.Errorf("cross namespace trait still exist")
			}
			if len(resourceTracker.Status.TrackedResources) != 1 {
				return fmt.Errorf("resourceTracker status recode trackedResource length missmatch")
			}
			deploys := new(appsv1.DeploymentList)
			err = k8sClient.List(ctx, deploys, opts...)
			if err != nil || len(deploys.Items) != 1 {
				return fmt.Errorf("error to list deploy")
			}
			deploy := deploys.Items[0]
			if len(deploy.OwnerReferences) != 1 || deploy.OwnerReferences[0].UID != resourceTracker.UID {
				return fmt.Errorf("deploy owner reference missmatch")
			}
			if resourceTracker.Status.TrackedResources[0].Name != deploy.Name {
				return fmt.Errorf("error to record deploy name in app status")
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
		By("deleting application will remove resourceTracker and related resourceTracker will be removed")
		app = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
		Eventually(func() error {
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err == nil {
				return fmt.Errorf("resourceTracker still exist")
			}
			if !apierrors.IsNotFound(err) {
				return err
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
	})

	It("Test cross-namespace resource gc logic, update a cross-ns workload's namespace", func() {
		// install  related definition
		crossCdJson, _ := yaml.YAMLToJSON([]byte(crossCompDefYaml))
		ccd := new(v1beta1.ComponentDefinition)
		Expect(json.Unmarshal(crossCdJson, ccd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ccd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		normalCdJson, _ := yaml.YAMLToJSON([]byte(normalCompDefYaml))
		ncd := new(v1beta1.ComponentDefinition)
		Expect(json.Unmarshal(normalCdJson, ncd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ncd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		var (
			appName       = "test-app-8"
			app           = new(v1beta1.Application)
			componentName = "test-app-8-comp"
		)
		app = &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					v1beta1.ApplicationComponent{
						Name:       componentName,
						Type:       "cross-worker",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		By("check resource tracker has been created and app status ")
		resourceTracker := new(v1beta1.ResourceTracker)
		Eventually(func() error {
			app := new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("app not found %v", err)
			}
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker); err != nil {
				return err
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status is not running")
			}
			if app.Status.ResourceTracker == nil || app.Status.ResourceTracker.UID != resourceTracker.UID {
				return fmt.Errorf("appication status error ")
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
		By("check resource is generated correctly")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
		var workload appsv1.Deployment
		Eventually(func() error {
			checkRt := new(v1beta1.ResourceTracker)
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), checkRt); err != nil {
				return err
			}
			depolys := new(appsv1.DeploymentList)
			opts := []client.ListOption{
				client.InNamespace(crossNamespace),
				client.MatchingLabels{
					oam.LabelAppName: appName,
				},
			}
			err := k8sClient.List(ctx, depolys, opts...)
			if err != nil || len(depolys.Items) != 1 {
				return fmt.Errorf("error workload number %v", err)
			}
			workload = depolys.Items[0]
			if len(workload.OwnerReferences) != 1 || workload.OwnerReferences[0].UID != resourceTracker.UID {
				return fmt.Errorf("wrokload ownerreference error")
			}
			if len(checkRt.Status.TrackedResources) != 1 {
				return fmt.Errorf("resourceTracker status recode trackedResource length missmatch")
			}
			if checkRt.Status.TrackedResources[0].Name != workload.Name {
				return fmt.Errorf("resourceTracker status recode trackedResource name mismatch recorded %s, actually %s", checkRt.Status.TrackedResources[0].Name, workload.Name)
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())

		By("update application modify workload namespace  will remove resourceTracker and related old workload will be removed")
		time.Sleep(3 * time.Second) // wait informer cache to be synced
		Eventually(func() error {
			app = new(v1beta1.Application)
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)
			if err != nil {
				return err
			}
			app.Spec.Components[0].Type = "normal-worker"
			err = k8sClient.Update(ctx, app)
			if err != nil {
				return err
			}
			return nil
		}, time.Second*30, time.Microsecond).Should(BeNil())
		Eventually(func() error {
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name), resourceTracker)
			if err == nil {
				return fmt.Errorf("resourceTracker still exist")
			}
			if !apierrors.IsNotFound(err) {
				return err
			}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: crossNamespace, Name: workload.GetName()}, &workload)
			if err == nil {
				return fmt.Errorf("wrokload still exist")
			}
			if !apierrors.IsNotFound(err) {
				return err
			}
			newWorkload := new(appsv1.Deployment)
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workload.GetName()}, newWorkload)
			if err != nil {
				return fmt.Errorf("generate same namespace workload error")
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())
	})
})

func generateResourceTrackerKey(namespace string, name string) types.NamespacedName {
	return types.NamespacedName{Name: fmt.Sprintf("%s-%s", namespace, name)}
}

const (
	crossCompDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: cross-worker
  namespace: app-resource-tracker-test-ns
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
  namespace: app-resource-tracker-test-ns
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
	crossNsTdYaml = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the app"
  name: cross-scaler
  namespace: app-resource-tracker-test-ns
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
)
