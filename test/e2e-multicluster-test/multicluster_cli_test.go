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
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test multicluster CLI commands", func() {

	var namespace string
	var hubCtx context.Context
	var workerCtx context.Context
	var app *v1beta1.Application

	BeforeEach(func() {
		hubCtx, workerCtx, namespace = initializeContextAndNamespace()
		app = &v1beta1.Application{}
		bs, err := os.ReadFile("./testdata/app/example-vela-cli-tool-test-app.yaml")
		Expect(err).Should(Succeed())
		appYaml := strings.ReplaceAll(string(bs), "TEST_NAMESPACE", namespace)
		Expect(yaml.Unmarshal([]byte(appYaml), app)).Should(Succeed())
		app.SetNamespace(namespace)
		Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
		Expect(err).Should(Succeed())
		Eventually(func(g Gomega) {
			pods := &v1.PodList{}
			g.Expect(k8sClient.List(workerCtx, pods, client.InNamespace(namespace), client.MatchingLabels(map[string]string{
				"app.oam.dev/name": app.Name,
			}))).Should(Succeed())
			g.Expect(len(pods.Items)).Should(Equal(1))
			g.Expect(pods.Items[0].Status.Phase).Should(Equal(v1.PodRunning))
			g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
			g.Expect(len(app.Status.AppliedResources)).ShouldNot(Equal(0))
		}, 2*time.Minute, time.Second*3).Should(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
		Expect(k8sClient.Delete(hubCtx, app)).Should(Succeed())
		cleanUpNamespace(hubCtx, workerCtx, namespace)
	})

	Context("Test debugging tools in multicluster", func() {

		It("Test vela exec", func() {
			command := exec.Command("vela", "exec", app.Name, "-n", namespace, "-i=false", "-t=false", "--", "pwd")
			outputs, err := command.CombinedOutput()
			Expect(string(outputs)).Should(ContainSubstring("/"))
			Expect(err).Should(Succeed())
		})

		It("Test vela port-forward", func() {
			stopChannel := make(chan struct{}, 1)
			go func() {
				defer GinkgoRecover()
				command := exec.Command("vela", "port-forward", app.Name, "-n", namespace)
				session, err := gexec.Start(command, io.Discard, io.Discard)
				Expect(err).Should(Succeed())
				<-stopChannel
				session.Terminate()
			}()
			defer func() {
				stopChannel <- struct{}{}
			}()
			var resp *http.Response
			var err error
			Eventually(func(g Gomega) {
				resp, err = http.Get("http://127.0.0.1:8000")
				g.Expect(err).Should(Succeed())
			}, time.Minute).Should(Succeed())
			bs := make([]byte, 128)
			_, err = resp.Body.Read(bs)
			Expect(err).Should(Succeed())
			Expect(string(bs)).Should(ContainSubstring("Hello World"))
		})

		It("Test vela status --tree", func() {
			_, err := execCommand("cluster", "alias", WorkerClusterName, "alias-worker-tree")
			Expect(err).Should(Succeed())
			for _, format := range []string{"inline", "wide", "table", "list"} {
				outputs, err := execCommand("status", app.Name, "-n", namespace, "--tree", "--detail", "--detail-format", format)
				Expect(err).Should(Succeed())
				Expect(outputs).Should(SatisfyAll(
					ContainSubstring("alias-worker-tree"),
					ContainSubstring("Deployment/exec-podinfo"),
					ContainSubstring("updated"),
					ContainSubstring("1/1"),
				))
			}
		})

		It("Test vela logs", func() {
			var (
				err         error
				session     *gexec.Session
				waitingTime = 2 * time.Minute
			)
			podViewCMFile, err := os.ReadFile("./testdata/view/component-pod-view.yaml")
			Expect(err).Should(BeNil())
			podViewCM := &v1.ConfigMap{}
			Expect(yaml.Unmarshal(podViewCMFile, podViewCM)).Should(BeNil())
			Expect(k8sClient.Create(hubCtx, podViewCM)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

			stopChannel := make(chan struct{}, 1)
			defer func() {
				stopChannel <- struct{}{}
			}()

			command := exec.Command("vela", "logs", app.Name, "-n", namespace, "--cluster", WorkerClusterName)
			session, err = gexec.Start(command, nil, nil)
			Expect(err).Should(Succeed())
			go func() {
				defer GinkgoRecover()
				<-stopChannel
				session.Terminate()
				Eventually(session, 10*time.Second).Should(gexec.Exit())
			}()
			Expect(err).Should(Succeed())
			Eventually(session, waitingTime).ShouldNot(BeNil())
			Eventually(session, waitingTime).Should(gbytes.Say("exec-podinfo"))
			Eventually(session, waitingTime).Should(gbytes.Say("httpd started"))
		})
	})

})

var _ = Describe("Test kube commands", func() {

	Context("Test apply command", func() {

		var namespace string
		var hubCtx context.Context
		var workerCtx context.Context

		BeforeEach(func() {
			hubCtx, workerCtx, namespace = initializeContextAndNamespace()
		})

		AfterEach(func() {
			cleanUpNamespace(hubCtx, workerCtx, namespace)
		})

		It("Test vela kube apply & delete", func() {
			_, err := execCommand("kube", "apply",
				"--cluster", types.ClusterLocalName, "--cluster", WorkerClusterName, "-n", namespace,
				"-f", "./testdata/kube",
				"-f", "https://gist.githubusercontent.com/Somefive/b189219a9222eaa70b8908cf4379402b/raw/e603987b3e0989e01e50f69ebb1e8bb436461326/example-busybox-deployment.yaml",
			)
			Expect(err).Should(Succeed())
			Expect(k8sClient.Get(hubCtx, apitypes.NamespacedName{Namespace: namespace, Name: "busybox"}, &appsv1.Deployment{})).Should(Succeed())
			Expect(k8sClient.Get(workerCtx, apitypes.NamespacedName{Namespace: namespace, Name: "busybox"}, &appsv1.Deployment{})).Should(Succeed())
			Expect(k8sClient.Get(hubCtx, apitypes.NamespacedName{Namespace: namespace, Name: "busybox"}, &v1.Service{})).Should(Succeed())
			Expect(k8sClient.Get(workerCtx, apitypes.NamespacedName{Namespace: namespace, Name: "busybox"}, &v1.Service{})).Should(Succeed())
			Expect(k8sClient.Get(hubCtx, apitypes.NamespacedName{Namespace: namespace, Name: "busybox-1"}, &v1.ConfigMap{})).Should(Succeed())
			Expect(k8sClient.Get(workerCtx, apitypes.NamespacedName{Namespace: namespace, Name: "busybox-1"}, &v1.ConfigMap{})).Should(Succeed())
			Expect(k8sClient.Get(hubCtx, apitypes.NamespacedName{Namespace: namespace, Name: "busybox-2"}, &v1.ConfigMap{})).Should(Succeed())
			Expect(k8sClient.Get(workerCtx, apitypes.NamespacedName{Namespace: namespace, Name: "busybox-2"}, &v1.ConfigMap{})).Should(Succeed())
			_, err = execCommand("kube", "delete",
				"--cluster", types.ClusterLocalName, "--cluster", WorkerClusterName, "-n", namespace,
				"deployment", "busybox",
			)
			Expect(err).Should(Succeed())
			Expect(apierrors.IsNotFound(k8sClient.Get(hubCtx, apitypes.NamespacedName{Namespace: namespace, Name: "busybox"}, &appsv1.Deployment{}))).Should(BeTrue())
			Expect(apierrors.IsNotFound(k8sClient.Get(workerCtx, apitypes.NamespacedName{Namespace: namespace, Name: "busybox"}, &appsv1.Deployment{}))).Should(BeTrue())
			_, err = execCommand("kube", "delete",
				"--cluster", types.ClusterLocalName, "--cluster", WorkerClusterName, "-n", namespace,
				"configmap", "--all",
			)
			Expect(err).Should(Succeed())
			Expect(apierrors.IsNotFound(k8sClient.Get(hubCtx, apitypes.NamespacedName{Namespace: namespace, Name: "busybox-1"}, &v1.ConfigMap{}))).Should(BeTrue())
			Expect(apierrors.IsNotFound(k8sClient.Get(workerCtx, apitypes.NamespacedName{Namespace: namespace, Name: "busybox-1"}, &v1.ConfigMap{}))).Should(BeTrue())
			Expect(apierrors.IsNotFound(k8sClient.Get(hubCtx, apitypes.NamespacedName{Namespace: namespace, Name: "busybox-2"}, &v1.ConfigMap{}))).Should(BeTrue())
			Expect(apierrors.IsNotFound(k8sClient.Get(workerCtx, apitypes.NamespacedName{Namespace: namespace, Name: "busybox-2"}, &v1.ConfigMap{}))).Should(BeTrue())
		})

	})

})

var _ = Describe("Test delete commands", func() {

	Context("Test delete command", func() {

		var namespace string
		var hubCtx context.Context
		var workerCtx context.Context

		BeforeEach(func() {
			hubCtx, workerCtx, namespace = initializeContextAndNamespace()
		})

		AfterEach(func() {
			cleanUpNamespace(hubCtx, workerCtx, namespace)
		})

		It("Test delete with orphan option", func() {
			bs, err := os.ReadFile("./testdata/app/app-orphan-delete.yaml")
			Expect(err).Should(Succeed())
			app := &v1beta1.Application{}
			Expect(yaml.Unmarshal(bs, app)).Should(Succeed())
			app.SetNamespace(namespace)
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)
			cmKey := apitypes.NamespacedName{Namespace: namespace, Name: "orphan-cm"}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, key, app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
				g.Expect(k8sClient.Get(workerCtx, cmKey, &v1.ConfigMap{})).To(Succeed())
			}).WithTimeout(20 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
			_, err = execCommand("delete", key.Name, "-n", key.Namespace, "--orphan", "-y")
			Expect(err).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(apierrors.IsNotFound(k8sClient.Get(hubCtx, key, app))).Should(BeTrue())
			}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
			Expect(k8sClient.Get(workerCtx, cmKey, &v1.ConfigMap{})).To(Succeed())
		})

	})

})
