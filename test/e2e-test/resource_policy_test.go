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
	v13 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/rand"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Application Resource-Related Policy Tests", func() {
	ctx := context.Background()
	var namespace string
	BeforeEach(func() {
		namespace = "test-resource-policy-" + rand.RandomString(4)
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: v12.ObjectMeta{Name: namespace}})).Should(Succeed())
	})

	AfterEach(func() {
		ns := &v1.Namespace{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, ns)).Should(Succeed())
	})

	It("Test ApplyOnce Policy", func() {
		By("create apply-once app(apply-once disabled)")
		app := &v1beta1.Application{}
		Expect(common.ReadYamlToObject("testdata/app/app_apply_once.yaml", app)).Should(BeNil())
		app.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		appKey := client.ObjectKeyFromObject(app)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			g.Expect(app.Status.Phase).Should(Equal(common2.ApplicationRunning))
		}, 30*time.Second, time.Second*3).Should(Succeed())

		By("test state-keep")
		deploy := &v13.Deployment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "hello-world"}, deploy)).Should(Succeed())
			deploy.Spec.Replicas = pointer.Int32(0)
			g.Expect(k8sClient.Update(ctx, deploy)).Should(Succeed())
		}, 10*time.Second, time.Second*2).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			app.Status.SetConditions(condition.Condition{Type: "StateKeep", Status: "True", Reason: condition.ReasonAvailable, LastTransitionTime: v12.Now()})
			g.Expect(k8sClient.Status().Update(ctx, app)).Should(Succeed())
		}, 10*time.Second, time.Second*2).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(deploy), deploy)).Should(Succeed())
			g.Expect(deploy.Spec.Replicas).Should(Equal(pointer.Int32(1)))
		}, 30*time.Second, time.Second*3).Should(Succeed())

		By("test apply-once policy(apply-once enabled)")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			app.Spec.Policies[0].Properties = &runtime.RawExtension{Raw: []byte(`{"enable":true}`)}
			g.Expect(k8sClient.Update(ctx, app)).Should(Succeed())
		}, 10*time.Second).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			g.Expect(app.Status.ObservedGeneration).Should(Equal(app.Generation))
			g.Expect(app.Status.Phase).Should(Equal(common2.ApplicationRunning))
		}, 30*time.Second).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "hello-world"}, deploy)).Should(Succeed())
			deploy.Spec.Replicas = pointer.Int32(0)
			g.Expect(k8sClient.Update(ctx, deploy)).Should(Succeed())
		}, 10*time.Second).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			app.Status.SetConditions(condition.Condition{Type: "ApplyOnce", Status: "True", Reason: condition.ReasonAvailable, LastTransitionTime: v12.Now()})
			g.Expect(k8sClient.Status().Update(ctx, app)).Should(Succeed())
		}, 10*time.Second).Should(Succeed())
		time.Sleep(30 * time.Second)
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(deploy), deploy)).Should(Succeed())
		Expect(deploy.Spec.Replicas).Should(Equal(pointer.Int32(0)))
	})

	It("Test GarbageCollect Policy", func() {
		By("create garbage-collect app")
		app := &v1beta1.Application{}
		Expect(common.ReadYamlToObject("testdata/app/app_garbage_collect.yaml", app)).Should(BeNil())
		app.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		appKey := client.ObjectKeyFromObject(app)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			g.Expect(app.Status.Phase).Should(Equal(common2.ApplicationRunning))
		}, 30*time.Second).Should(Succeed())

		By("upgrade to v2 (same component)")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			app.Spec.Components[0].Traits[0].Properties = &runtime.RawExtension{Raw: []byte(`{"port":[8001]}`)}
			g.Expect(k8sClient.Update(ctx, app)).Should(Succeed())
		}, 10*time.Second).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			g.Expect(app.Status.ObservedGeneration).Should(Equal(app.Generation))
			g.Expect(app.Status.Phase).Should(Equal(common2.ApplicationRunning))
		}, 30*time.Second).Should(Succeed())

		By("upgrade to v3 (new component)")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			app.Spec.Components[0].Name = "hello-world-new"
			g.Expect(k8sClient.Update(ctx, app)).Should(Succeed())
		}, 10*time.Second).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			g.Expect(app.Status.ObservedGeneration).Should(Equal(app.Generation))
			g.Expect(app.Status.Phase).Should(Equal(common2.ApplicationRunning))
		}, 30*time.Second).Should(Succeed())

		By("upgrade to v4 (new component)")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			app.Spec.Components[0].Name = "hello-world-latest"
			g.Expect(k8sClient.Update(ctx, app)).Should(Succeed())
		}, 10*time.Second).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			g.Expect(app.Status.ObservedGeneration).Should(Equal(app.Generation))
			g.Expect(app.Status.Phase).Should(Equal(common2.ApplicationRunning))
		}, 30*time.Second).Should(Succeed())

		deployments := &v13.DeploymentList{}
		Expect(k8sClient.List(ctx, deployments, client.InNamespace(namespace))).Should(Succeed())
		Expect(len(deployments.Items)).Should(Equal(3))
		services := &v1.ServiceList{}
		Expect(k8sClient.List(ctx, services, client.InNamespace(namespace))).Should(Succeed())
		Expect(len(services.Items)).Should(Equal(3))

		By("delete v3")
		rt := &v1beta1.ResourceTracker{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-v3-%s", app.Name, namespace)}, rt)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, rt)).Should(Succeed())
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-v3-%s", app.Name, namespace)}, rt)
			g.Expect(errors.IsNotFound(err)).Should(BeTrue())
		}, 15*time.Second).Should(Succeed())
		Expect(k8sClient.List(ctx, deployments, client.InNamespace(namespace))).Should(Succeed())
		Expect(len(deployments.Items)).Should(Equal(2))
		Expect(k8sClient.List(ctx, services, client.InNamespace(namespace))).Should(Succeed())
		Expect(len(services.Items)).Should(Equal(3))

		By("delete latest deploy, auto gc rt v4")
		deploy := &v13.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "hello-world-latest"}, deploy)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, deploy)).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			app.Status.SetConditions(condition.Condition{Type: "GC", Status: "True", Reason: condition.ReasonAvailable, LastTransitionTime: v12.Now()})
			g.Expect(k8sClient.Status().Update(ctx, app)).Should(Succeed())
		}, 10*time.Second).Should(Succeed())
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-%s-v4", app.Name, namespace)}, rt)
			g.Expect(errors.IsNotFound(err)).Should(BeTrue())
		}, 15*time.Second).Should(Succeed())
		Expect(k8sClient.List(ctx, deployments, client.InNamespace(namespace))).Should(Succeed())
		Expect(len(deployments.Items)).Should(Equal(1))
		Expect(k8sClient.List(ctx, services, client.InNamespace(namespace))).Should(Succeed())
		Expect(len(services.Items)).Should(Equal(3))

		By("delete application")
		Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, appKey, app)
			g.Expect(errors.IsNotFound(err)).Should(BeTrue())
		}, 15*time.Second).Should(Succeed())
		Expect(k8sClient.List(ctx, deployments, client.InNamespace(namespace))).Should(Succeed())
		Expect(len(deployments.Items)).Should(Equal(0))
		Expect(k8sClient.List(ctx, services, client.InNamespace(namespace))).Should(Succeed())
		Expect(len(services.Items)).Should(Equal(0))
	})

	It("Test state keep during suspending", func() {
		By("create suspending app")
		app := &v1beta1.Application{}
		Expect(common.ReadYamlToObject("testdata/app/app_suspending.yaml", app)).Should(BeNil())
		app.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		appKey := client.ObjectKeyFromObject(app)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			g.Expect(app.Status.Phase).Should(Equal(common2.ApplicationWorkflowSuspending))
		}, 30*time.Second).Should(Succeed())

		By("test suspending app state-keep")
		deploy := &v13.Deployment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "busybox"}, deploy)).Should(Succeed())
			deploy.Spec.Replicas = pointer.Int32(0)
			g.Expect(k8sClient.Update(ctx, deploy)).Should(Succeed())
		}, 10*time.Second).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			app.Status.SetConditions(condition.Condition{Type: "StateKeep", Status: "True", Reason: condition.ReasonAvailable, LastTransitionTime: v12.Now()})
			g.Expect(k8sClient.Status().Update(ctx, app)).Should(Succeed())
		}, 10*time.Second).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(deploy), deploy)).Should(Succeed())
			g.Expect(deploy.Spec.Replicas).Should(Equal(pointer.Int32(1)))
		}, 30*time.Second).Should(Succeed())
	})
})
