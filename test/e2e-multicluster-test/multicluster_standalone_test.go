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
	"fmt"
	"io/ioutil"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("Test multicluster standalone scenario", func() {

	var namespace string
	var hubCtx context.Context
	var workerCtx context.Context

	BeforeEach(func() {
		hubCtx, workerCtx, namespace = initializeContextAndNamespace()
	})

	AfterEach(func() {
		cleanUpNamespace(hubCtx, workerCtx, namespace)
	})

	It("Test standalone app", func() {
		applyFile := func(filename string) {
			bs, err := ioutil.ReadFile("./testdata/app/standalone/" + filename)
			Expect(err).Should(Succeed())
			un := &unstructured.Unstructured{}
			Expect(yaml.Unmarshal(bs, un)).Should(Succeed())
			un.SetNamespace(namespace)
			Expect(k8sClient.Create(context.Background(), un)).Should(Succeed())
		}
		By("Apply resources")
		applyFile("deployment.yaml")
		applyFile("configmap-1.yaml")
		applyFile("configmap-2.yaml")
		applyFile("workflow.yaml")
		applyFile("policy.yaml")
		applyFile("app.yaml")

		Eventually(func(g Gomega) {
			deploys := &v1.DeploymentList{}
			g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
			g.Expect(len(deploys.Items)).Should(Equal(1))
			g.Expect(deploys.Items[0].Spec.Replicas).Should(Equal(pointer.Int32(3)))
			cms := &corev1.ConfigMapList{}
			g.Expect(k8sClient.List(workerCtx, cms, client.InNamespace(namespace), client.MatchingLabels(map[string]string{"app": "podinfo"}))).Should(Succeed())
			g.Expect(len(cms.Items)).Should(Equal(2))
		}, 30*time.Second).Should(Succeed())

		Eventually(func(g Gomega) {
			app := &v1beta1.Application{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: "podinfo"}, app)).Should(Succeed())
			Expect(k8sClient.Delete(context.Background(), app)).Should(Succeed())
		}, 15*time.Second).Should(Succeed())

		Eventually(func(g Gomega) {
			deploys := &v1.DeploymentList{}
			g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
			g.Expect(len(deploys.Items)).Should(Equal(0))
			cms := &corev1.ConfigMapList{}
			g.Expect(k8sClient.List(workerCtx, cms, client.InNamespace(namespace), client.MatchingLabels(map[string]string{"app": "podinfo"}))).Should(Succeed())
			g.Expect(len(cms.Items)).Should(Equal(0))
		}, 30*time.Second).Should(Succeed())
	})

	It("Test large application parallel apply and delete", func() {
		newApp := &v1beta1.Application{ObjectMeta: v12.ObjectMeta{Namespace: namespace, Name: "large-app"}}
		size := 30
		for i := 0; i < size; i++ {
			newApp.Spec.Components = append(newApp.Spec.Components, oamcomm.ApplicationComponent{
				Name:       fmt.Sprintf("comp-%d", i),
				Type:       "webservice",
				Properties: &runtime.RawExtension{Raw: []byte(`{"image":"busybox","imagePullPolicy":"IfNotPresent","cmd":["sleep","86400"]}`)},
			})
		}
		newApp.Spec.Policies = append(newApp.Spec.Policies, v1beta1.AppPolicy{
			Name:       "topology-deploy",
			Type:       "topology",
			Properties: &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"clusters":["%s"]}`, WorkerClusterName))},
		})
		newApp.Spec.Workflow = &v1beta1.Workflow{
			Steps: []v1beta1.WorkflowStep{{
				Name:       "deploy",
				Type:       "deploy",
				Properties: &runtime.RawExtension{Raw: []byte(`{"policies":["topology-deploy"],"parallelism":10}`)},
			}},
		}
		Expect(k8sClient.Create(context.Background(), newApp)).Should(Succeed())
		Eventually(func(g Gomega) {
			deploys := &v1.DeploymentList{}
			g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
			g.Expect(len(deploys.Items)).Should(Equal(size))
		}, 2*time.Minute).Should(Succeed())

		Eventually(func(g Gomega) {
			app := &v1beta1.Application{}
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(newApp), app)).Should(Succeed())
			g.Expect(k8sClient.Delete(context.Background(), app)).Should(Succeed())
		}, 15*time.Second).Should(Succeed())

		Eventually(func(g Gomega) {
			app := &v1beta1.Application{}
			err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(newApp), app)
			g.Expect(errors.IsNotFound(err)).Should(BeTrue())
		}, time.Minute).Should(Succeed())
	})
})
