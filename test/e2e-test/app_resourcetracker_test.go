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
	"github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test application cross namespace resource", func() {
	ctx := context.Background()
	var namespace, crossNamespace string

	BeforeEach(func() {
		namespace = randomNamespaceName("app-resource-tracker-e2e")
		ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		crossNamespace = randomNamespaceName("cross-namespace")
		crossNs := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: crossNamespace}}
		Expect(k8sClient.Create(ctx, &crossNs)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Eventually(func() error {
			ns := new(corev1.Namespace)
			return k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns)
		}, time.Second*5, time.Millisecond*300).Should(BeNil())
		Eventually(func() error {
			ns := new(corev1.Namespace)
			return k8sClient.Get(ctx, types.NamespacedName{Name: crossNamespace}, ns)
		}, time.Second*5, time.Millisecond*300).Should(BeNil())
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.WorkloadDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.TraitDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))

		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: crossNamespace}}, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	})

	It("Test application containing cluster-scoped trait", func() {
		By("Install TraitDefinition")
		traitDef := &v1beta1.TraitDefinition{}
		Expect(yaml.Unmarshal([]byte(fmt.Sprintf(clusterScopeTraitDefYAML, namespace)), traitDef)).Should(Succeed())
		Expect(k8sClient.Create(ctx, traitDef)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Create Application")
		var (
			appName       = "cluster-scope-trait-app"
			app           = new(v1beta1.Application)
			componentName = "cluster-scope-trait-comp"
		)
		app = &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       componentName,
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"image": "nginx:latest"}`)},
						Traits: []common.ApplicationTrait{{
							Type:       "cluster-scope-trait",
							Properties: &runtime.RawExtension{Raw: []byte("{}")},
						}},
					},
				},
			},
		}
		Eventually(func() error {
			return k8sClient.Create(ctx, app)
		}, 20*time.Second, 2*time.Second).Should(Succeed())

		By("Verify the trait is created")
		// sample cluster-scoped trait is PersistentVolume
		pv := &corev1.PersistentVolume{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "pv-" + componentName, Namespace: namespace}, pv)
		}, 20*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Delete Application")
		Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
		By("Verify cluster-scoped trait is deleted cascadingly")
		Eventually(func() error {
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: "pv-" + componentName, Namespace: namespace}, pv); err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
				return errors.Wrap(err, "PersistentVolume has not deleted")
			}
			if ctrlutil.ContainsFinalizer(pv, "kubernetes.io/pv-protection") {
				return nil
			}
			return errors.New("PersistentVolume has not deleted")
		}, 20*time.Second, 500*time.Millisecond).Should(BeNil())
	})

	It("Test application have cross-namespace workload", func() {
		// install  component definition
		crossCdJson, _ := yaml.YAMLToJSON([]byte(fmt.Sprintf(crossCompDefYaml, namespace, crossNamespace)))
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
				Components: []common.ApplicationComponent{
					{
						Name:       componentName,
						Type:       "cross-worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
			},
		}
		Eventually(func() error {
			return k8sClient.Create(ctx, app)
		}, 15*time.Second, 300*time.Microsecond).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		By("check resource tracker has been created and app status ")
		resourceTracker := new(v1beta1.ResourceTracker)
		Eventually(func() error {
			app := new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("app not found %v", err)
			}
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 1), resourceTracker); err != nil {
				return err
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status is not running")
			}
			return nil
		}, time.Second*5, time.Millisecond*300).Should(BeNil())
		By("check resource is generated correctly")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
		var workload appsv1.Deployment
		Eventually(func() error {
			checkRt := new(v1beta1.ResourceTracker)
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 1), checkRt); err != nil {
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
			if len(checkRt.Spec.ManagedResources) != 1 {
				return fmt.Errorf("resourceTracker status recode trackedResource length missmatch")
			}
			if checkRt.Spec.ManagedResources[0].Name != workload.Name {
				return fmt.Errorf("resourceTracker status recode trackedResource name mismatch recorded %s, actually %s", checkRt.Spec.ManagedResources[0].Name, workload.Name)
			}
			return nil
		}, time.Second*5, time.Millisecond*300).Should(BeNil())

		By("deleting application will remove resourceTracker and related workload will be removed")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
		Eventually(func() error {
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 1), resourceTracker)
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
		}, time.Second*5, time.Millisecond*300).Should(BeNil())
	})

	It("Test application have two different workload", func() {
		var (
			appName        = "test-app-4"
			app            = new(v1beta1.Application)
			component1Name = "test-app-4-comp-1"
			component2Name = "test-app-4-comp-2"
		)
		By("install component definition")
		normalCdJson, _ := yaml.YAMLToJSON([]byte(fmt.Sprintf(normalCompDefYaml, namespace)))
		ncd := new(v1beta1.ComponentDefinition)
		Expect(json.Unmarshal(normalCdJson, ncd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ncd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		crossCdJson, err := yaml.YAMLToJSON([]byte(fmt.Sprintf(crossCompDefYaml, namespace, crossNamespace)))
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
				Components: []common.ApplicationComponent{
					{
						Name:       component1Name,
						Type:       "normal-worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       component2Name,
						Type:       "cross-worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
			},
		}

		Eventually(func() error {
			return k8sClient.Create(ctx, app)
		}, 15*time.Second, 300*time.Microsecond).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
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
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 1), resourceTracker)
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
			err = k8sClient.List(ctx, cross, crossOpts...)
			if err != nil || len(cross.Items) != 1 {
				return fmt.Errorf("failed generate cross namespace trait")
			}
			if len(resourceTracker.Spec.ManagedResources) != 2 {
				return fmt.Errorf("expect track %q resources, but got %q", 2, len(resourceTracker.Spec.ManagedResources))
			}
			return nil
		}, time.Second*5, time.Millisecond*500).Should(BeNil())
		By("update application by delete cross namespace workload")
		Eventually(func() error {
			app = new(v1beta1.Application)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
			app.Spec.Components = app.Spec.Components[:1] // delete a component
			return k8sClient.Update(ctx, app)
		}, time.Second*5, time.Millisecond*300).Should(BeNil())
		Eventually(func() error {
			app = new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("error to get application %v", err)
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status not running")
			}
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 2), resourceTracker); err != nil {
				return err
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
			err = k8sClient.List(ctx, cross, crossOpts...)
			if err != nil || len(cross.Items) != 0 {
				return fmt.Errorf("error : cross namespace workload still exist")
			}
			return nil
		}, time.Second*5, time.Millisecond*300).Should(BeNil())
	})

	It("Update a cross namespace workload of application", func() {
		// install  component definition
		crossCdJson, _ := yaml.YAMLToJSON([]byte(fmt.Sprintf(crossCompDefYaml, namespace, crossNamespace)))
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
				Components: []common.ApplicationComponent{
					{
						Name:       componentName,
						Type:       "cross-worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
			},
		}
		Eventually(func() error {
			return k8sClient.Create(ctx, app)
		}, 15*time.Second, 300*time.Microsecond).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		By("check resource tracker has been created and app status ")
		resourceTracker := new(v1beta1.ResourceTracker)
		Eventually(func() error {
			app := new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("app not found %v", err)
			}
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 1), resourceTracker); err != nil {
				return err
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status is not running")
			}
			return nil
		}, time.Second*5, time.Millisecond*300).Should(BeNil())
		By("check resource is generated correctly")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
		var workload appsv1.Deployment
		Eventually(func() error {
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
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 1), resourceTracker); err != nil {
				return err
			}
			if workload.Spec.Template.Spec.Containers[0].Image != "busybox" {
				return fmt.Errorf("container image not match")
			}
			if len(resourceTracker.Spec.ManagedResources) != 1 {
				return fmt.Errorf("expect track %q resources, but got %q", 1, len(resourceTracker.Spec.ManagedResources))
			}
			return nil
		}, time.Second*50, time.Millisecond*300).Should(BeNil())

		By("update application and check resource status")
		Eventually(func() error {
			checkApp := new(v1beta1.Application)
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, checkApp)
			if err != nil {
				return err
			}
			checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"nginx"}`)}
			err = k8sClient.Update(ctx, checkApp)
			if err != nil {
				return err
			}
			return nil
		}, time.Second*5, time.Millisecond*300).Should(BeNil())

		Eventually(func() error {
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 2), resourceTracker); err != nil {
				return err
			}
			if len(resourceTracker.Spec.ManagedResources) != 1 {
				return fmt.Errorf("expect track %q resources, but got %q", 1, len(resourceTracker.Spec.ManagedResources))
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
			if workload.Spec.Template.Spec.Containers[0].Image != "nginx" {
				return fmt.Errorf("container image not match")
			}
			return nil
		}, time.Second*5, time.Millisecond*500).Should(BeNil())

		By("deleting application will remove resourceTracker and related workload will be removed")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
		Eventually(func() error {
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 2), resourceTracker)
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
		}, time.Second*5, time.Millisecond*500).Should(BeNil())
	})

	It("Test cross-namespace resource gc logic, delete a cross-ns component", func() {
		var (
			appName        = "test-app-6"
			app            = new(v1beta1.Application)
			component1Name = "test-app-6-comp-1"
			component2Name = "test-app-6-comp-2"
		)
		By("install related definition")

		crossCdJson, err := yaml.YAMLToJSON([]byte(fmt.Sprintf(crossCompDefYaml, namespace, crossNamespace)))
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
				Components: []common.ApplicationComponent{
					{
						Name:       component1Name,
						Type:       "cross-worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       component2Name,
						Type:       "cross-worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
			},
		}

		Eventually(func() error {
			return k8sClient.Create(ctx, app)
		}, 15*time.Second, 300*time.Microsecond).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
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
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 1), resourceTracker)
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
			deploy2 := workloads.Items[1]
			if len(resourceTracker.Spec.ManagedResources) != 2 {
				return fmt.Errorf("expect track %q resources, but got %q", 2, len(resourceTracker.Spec.ManagedResources))
			}
			if resourceTracker.Spec.ManagedResources[0].Namespace != crossNamespace || resourceTracker.Spec.ManagedResources[1].Namespace != crossNamespace {
				return fmt.Errorf("resourceTracker recorde namespace mismatch")
			}
			if resourceTracker.Spec.ManagedResources[0].Name != deploy1.Name && resourceTracker.Spec.ManagedResources[1].Name != deploy1.Name {
				return fmt.Errorf("resourceTracker status recode trackedResource name mismatch recorded %s, actually %s", resourceTracker.Spec.ManagedResources[0].Name, deploy1.Name)
			}
			if resourceTracker.Spec.ManagedResources[0].Name != deploy2.Name && resourceTracker.Spec.ManagedResources[1].Name != deploy2.Name {
				return fmt.Errorf("resourceTracker status recode trackedResource name mismatch recorded %s, actually %s", resourceTracker.Spec.ManagedResources[0].Name, deploy2.Name)
			}
			return nil
		}, time.Second*5, time.Millisecond*300).Should(BeNil())
		By("update application by delete a cross namespace workload")
		Eventually(func() error {
			app = new(v1beta1.Application)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
			app.Spec.Components = app.Spec.Components[:1] // delete a component
			return k8sClient.Update(ctx, app)
		}, time.Second*5, time.Millisecond*300).Should(BeNil())
		Eventually(func() error {
			app = new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("error to get application %v", err)
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status not running")
			}
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 2), resourceTracker)
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
			checkRt := new(v1beta1.ResourceTracker)
			err = k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 2), checkRt)
			if err != nil {
				return fmt.Errorf("error get resourceTracker")
			}
			if len(checkRt.Spec.ManagedResources) != 1 {
				return fmt.Errorf("expect track %q resources, but got %q", 1, len(checkRt.Spec.ManagedResources))
			}
			return nil
		}, time.Second*5, time.Millisecond*500).Should(BeNil())

		By("deleting application will remove resourceTracker")
		app = new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
		Eventually(func() error {
			err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 2), resourceTracker)
			if err == nil {
				return fmt.Errorf("resourceTracker still exist")
			}
			if !apierrors.IsNotFound(err) {
				return err
			}
			return nil
		}, time.Second*5, time.Millisecond*500).Should(BeNil())
	})

	It("Test cross-namespace resource gc logic, update a cross-ns workload's namespace", func() {
		// install  related definition
		crossCdJson, _ := yaml.YAMLToJSON([]byte(fmt.Sprintf(crossCompDefYaml, namespace, crossNamespace)))
		ccd := new(v1beta1.ComponentDefinition)
		Expect(json.Unmarshal(crossCdJson, ccd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ccd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		normalCdJson, _ := yaml.YAMLToJSON([]byte(fmt.Sprintf(normalCompDefYaml, namespace)))
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
				Components: []common.ApplicationComponent{
					{
						Name:       componentName,
						Type:       "cross-worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
			},
		}
		Eventually(func() error {
			return k8sClient.Create(ctx, app)
		}, 15*time.Second, 300*time.Microsecond).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		By("check resource tracker has been created and app status ")
		resourceTracker := new(v1beta1.ResourceTracker)
		Eventually(func() error {
			app := new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app); err != nil {
				return fmt.Errorf("app not found %v", err)
			}
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 1), resourceTracker); err != nil {
				return err
			}
			if app.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application status is not running")
			}
			return nil
		}, time.Second*5, time.Millisecond*500).Should(BeNil())
		By("check resource is generated correctly")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appName}, app)).Should(BeNil())
		var workload appsv1.Deployment
		Eventually(func() error {
			checkRt := new(v1beta1.ResourceTracker)
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 1), checkRt); err != nil {
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
			if len(checkRt.Spec.ManagedResources) != 1 {
				return fmt.Errorf("resourceTracker status recode trackedResource length missmatch")
			}
			if checkRt.Spec.ManagedResources[0].Name != workload.Name {
				return fmt.Errorf("resourceTracker status recode trackedResource name mismatch recorded %s, actually %s", checkRt.Spec.ManagedResources[0].Name, workload.Name)
			}
			return nil
		}, time.Second*5, time.Millisecond*500).Should(BeNil())

		By("update application modify workload namespace")
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
		}, time.Second*5, time.Millisecond*500).Should(BeNil())
		Eventually(func() error {
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(app.Namespace, app.Name, 2), resourceTracker); err != nil {
				return err
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: crossNamespace, Name: workload.GetName()}, &workload)
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
		}, time.Second*5, time.Millisecond*500).Should(BeNil())
	})
})

func generateResourceTrackerKey(namespace string, appName string, revision int) types.NamespacedName {
	return types.NamespacedName{Name: fmt.Sprintf("%s-v%d-%s", appName, revision, namespace)}
}

const (
	crossCompDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: cross-worker
  namespace: %s
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
              namespace: "%s"
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
  namespace: %s
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

	clusterScopeTraitDefYAML = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: cluster-scope-trait
  namespace: %s
spec:
  appliesToWorkloads:
    - deployments.apps
  extension:
    template: |-
      outputs: pv: { 
        apiVersion: "v1"
        kind:       "PersistentVolume"
        metadata: name: "pv-\(context.name)"
        spec: {
           accessModes: ["ReadWriteOnce"]
           capacity: storage: "5Gi"
           persistentVolumeReclaimPolicy: "Retain"
           storageClassName:              "test-sc"
           csi: {
           	driver:       "gcs.csi.ofek.dev"
           	volumeHandle: "csi-gcs"
           	nodePublishSecretRef: {
           	    name:      "bucket-sa-\(context.name)"
           	    namespace: context.namespace
           	}
           }
        }
      }
`
)
