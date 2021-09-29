/*
 Copyright 2021. The KubeVela Authors.

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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v13 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Test multicluster scenario", func() {

	Context("Test vela cluster command", func() {

		It("Test join cluster by X509 kubeconfig, rename it and detach it.", func() {
			const oldClusterName = "test-worker-cluster"
			const newClusterName = "test-cluster-worker"
			_, err := execCommand("cluster", "list")
			Expect(err).Should(Succeed())
			_, err = execCommand("cluster", "join", "/tmp/worker.kubeconfig", "--name", oldClusterName)
			Expect(err).Should(Succeed())
			out, err := execCommand("cluster", "list")
			Expect(err).Should(Succeed())
			Expect(out).Should(ContainSubstring(oldClusterName))
			_, err = execCommand("cluster", "rename", oldClusterName, newClusterName)
			Expect(err).Should(Succeed())
			out, err = execCommand("cluster", "list")
			Expect(err).Should(Succeed())
			Expect(out).Should(ContainSubstring(newClusterName))
			_, err = execCommand("cluster", "detach", newClusterName)
			Expect(err).Should(Succeed())
			out, err = execCommand("cluster", "list")
			Expect(err).Should(Succeed())
			Expect(out).ShouldNot(ContainSubstring(newClusterName))
		})

	})

	Context("Test EnvBinding Application", func() {

		var namespace string
		var hubCtx context.Context
		var workerCtx context.Context

		BeforeEach(func() {
			hubCtx = context.Background()
			workerCtx = multicluster.ContextWithClusterName(hubCtx, WorkerClusterName)
			// initialize test namespace
			namespace = fmt.Sprintf("test-%d", time.Now().UnixNano())
			ns := &v1.Namespace{ObjectMeta: v12.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(hubCtx, ns.DeepCopy())).Should(Succeed())
			Expect(k8sClient.Create(workerCtx, ns.DeepCopy())).Should(Succeed())
		})

		AfterEach(func() {
			// clean up test namespaces
			hubNs := &v1.Namespace{}
			Expect(k8sClient.Get(hubCtx, types.NamespacedName{Name: namespace}, hubNs)).Should(Succeed())
			Expect(k8sClient.Delete(hubCtx, hubNs)).Should(Succeed())
			workerNs := &v1.Namespace{}
			Expect(k8sClient.Get(workerCtx, types.NamespacedName{Name: namespace}, workerNs)).Should(Succeed())
			Expect(k8sClient.Delete(workerCtx, workerNs)).Should(Succeed())
		})

		It("Test create EnvBinding Application", func() {
			// This test is going to cover multiple functions, including
			// 1. Multiple stage deployment for two environment, involving suspend
			// 2. A special cluster: local cluster
			// 3. Component selector.
			app := &v1beta1.Application{}
			Expect(common.ReadYamlToObject("./testdata/app/example-envbinding-app.yaml", app)).Should(BeNil())
			app.SetNamespace(namespace)
			err := k8sClient.Create(hubCtx, app)
			Expect(err).Should(Succeed())
			var hubDeployName string
			Eventually(func(g Gomega) {
				// check deployments in clusters
				deploys := &v13.DeploymentList{}
				g.Expect(k8sClient.List(hubCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(1))
				hubDeployName = deploys.Items[0].Name
				deploys = &v13.DeploymentList{}
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(2))
			}, 2*time.Minute).Should(Succeed())
			Expect(hubDeployName).Should(Equal("data-worker"))
			// delete application
			Expect(k8sClient.Delete(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				// check deployments in clusters
				deploys := &v13.DeploymentList{}
				g.Expect(k8sClient.List(hubCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(0))
				deploys = &v13.DeploymentList{}
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(0))
			}, 2*time.Minute).Should(Succeed())
		})
	})

})
