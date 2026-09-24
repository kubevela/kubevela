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

package e2e_multicluster_test

import (
	"context"
	"os"
	"time"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("Test deploy-components workflow step", func() {

	var namespace string
	var hubCtx context.Context
	var workerCtx context.Context

	BeforeEach(func() {
		hubCtx, workerCtx, namespace = initializeContextAndNamespace()
	})

	AfterEach(func() {
		cleanUpNamespace(hubCtx, workerCtx, namespace)
	})

	It("Test deploying each component to the cluster resolved from its own topology policy", func() {
		app := &v1beta1.Application{}
		bs, err := os.ReadFile("./testdata/app/app-deploy-components.yaml")
		Expect(err).Should(Succeed())
		Expect(yaml.Unmarshal(bs, app)).Should(Succeed())
		app.SetNamespace(namespace)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Create(context.Background(), app)).Should(Succeed())
		}).WithPolling(2 * time.Second).WithTimeout(5 * time.Second).Should(Succeed())

		appKey := client.ObjectKeyFromObject(app)
		Eventually(func(g Gomega) {
			_app := &v1beta1.Application{}
			g.Expect(k8sClient.Get(context.Background(), appKey, _app)).Should(Succeed())
			g.Expect(_app.Status.Phase).Should(Equal(common.ApplicationRunning))
		}).WithPolling(2 * time.Second).WithTimeout(20 * time.Second).Should(Succeed())

		By("component mapped to the local topology policy only lands on the hub cluster")
		Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "cm-on-local"}, &corev1.ConfigMap{})).Should(Succeed())
		Expect(kerrors.IsNotFound(k8sClient.Get(workerCtx, types.NamespacedName{Namespace: namespace, Name: "cm-on-local"}, &corev1.ConfigMap{}))).Should(BeTrue())

		By("component mapped to the worker topology policy only lands on the worker cluster")
		Expect(k8sClient.Get(workerCtx, types.NamespacedName{Namespace: namespace, Name: "cm-on-worker"}, &corev1.ConfigMap{})).Should(Succeed())
		Expect(kerrors.IsNotFound(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "cm-on-worker"}, &corev1.ConfigMap{}))).Should(BeTrue())

		By("Deleting")
		_app := &v1beta1.Application{}
		Expect(k8sClient.Get(context.Background(), appKey, _app)).Should(Succeed())
		Expect(k8sClient.Delete(context.Background(), _app)).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(kerrors.IsNotFound(k8sClient.Get(context.Background(), appKey, _app))).Should(BeTrue())
		}).WithPolling(2 * time.Second).WithTimeout(20 * time.Second).Should(Succeed())
		Expect(kerrors.IsNotFound(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "cm-on-local"}, &corev1.ConfigMap{}))).Should(BeTrue())
		Expect(kerrors.IsNotFound(k8sClient.Get(workerCtx, types.NamespacedName{Namespace: namespace, Name: "cm-on-worker"}, &corev1.ConfigMap{}))).Should(BeTrue())
	})

	It("Test deploy-components fails fast with a clear message when a component name is wrong", func() {
		app := &v1beta1.Application{}
		bs, err := os.ReadFile("./testdata/app/app-deploy-components-missing.yaml")
		Expect(err).Should(Succeed())
		Expect(yaml.Unmarshal(bs, app)).Should(Succeed())
		app.SetNamespace(namespace)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Create(context.Background(), app)).Should(Succeed())
		}).WithPolling(2 * time.Second).WithTimeout(5 * time.Second).Should(Succeed())

		appKey := client.ObjectKeyFromObject(app)
		Eventually(func(g Gomega) {
			_app := &v1beta1.Application{}
			g.Expect(k8sClient.Get(context.Background(), appKey, _app)).Should(Succeed())
			g.Expect(_app.Status.Workflow).ShouldNot(BeNil())
			g.Expect(len(_app.Status.Workflow.Steps)).ShouldNot(Equal(0))
			g.Expect(_app.Status.Workflow.Steps[0].Phase).Should(Equal(workflowv1alpha1.WorkflowStepPhaseFailed))
			g.Expect(_app.Status.Workflow.Steps[0].Message).Should(ContainSubstring("component(s) not found in application: cm-on-lcal"))
		}).WithPolling(2 * time.Second).WithTimeout(20 * time.Second).Should(Succeed())
	})
})
