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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v13 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	v14 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
)

func initializeContext() (hubCtx context.Context, workerCtx context.Context) {
	hubCtx = context.Background()
	workerCtx = multicluster.ContextWithClusterName(hubCtx, WorkerClusterName)
	return
}

func initializeContextAndNamespace() (hubCtx context.Context, workerCtx context.Context, namespace string) {
	hubCtx, workerCtx = initializeContext()
	// initialize test namespace
	namespace = fmt.Sprintf("test-%d", time.Now().UnixNano())
	ns := &v1.Namespace{ObjectMeta: v12.ObjectMeta{Name: namespace}}
	Expect(k8sClient.Create(hubCtx, ns.DeepCopy())).Should(Succeed())
	Expect(k8sClient.Create(workerCtx, ns.DeepCopy())).Should(Succeed())
	return
}

func cleanUpNamespace(hubCtx context.Context, workerCtx context.Context, namespace string) {
	hubNs := &v1.Namespace{}
	Expect(k8sClient.Get(hubCtx, types.NamespacedName{Name: namespace}, hubNs)).Should(Succeed())
	Expect(k8sClient.Delete(hubCtx, hubNs)).Should(Succeed())
	workerNs := &v1.Namespace{}
	Expect(k8sClient.Get(workerCtx, types.NamespacedName{Name: namespace}, workerNs)).Should(Succeed())
	Expect(k8sClient.Delete(workerCtx, workerNs)).Should(Succeed())
}

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

		It("Test detach cluster with application use", func() {
			const testClusterName = "test-cluster"
			_, err := execCommand("cluster", "join", "/tmp/worker.kubeconfig", "--name", testClusterName)
			Expect(err).Should(Succeed())
			app := &v1beta1.Application{}
			bs, err := ioutil.ReadFile("./testdata/app/example-lite-envbinding-app.yaml")
			Expect(err).Should(Succeed())
			appYaml := strings.ReplaceAll(string(bs), "TEST_CLUSTER_NAME", testClusterName)
			Expect(yaml.Unmarshal([]byte(appYaml), app)).Should(Succeed())
			ctx := context.Background()
			err = k8sClient.Create(ctx, app)
			Expect(err).Should(Succeed())
			namespacedName := client.ObjectKeyFromObject(app)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, namespacedName, app)).Should(Succeed())
				g.Expect(len(app.Status.PolicyStatus)).ShouldNot(Equal(0))
			}, 30*time.Second).Should(Succeed())
			_, err = execCommand("cluster", "detach", testClusterName)
			Expect(err).ShouldNot(Succeed())
			err = k8sClient.Delete(ctx, app)
			Expect(err).Should(Succeed())
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, namespacedName, app)
				g.Expect(kerrors.IsNotFound(err)).Should(BeTrue())
			}, 30*time.Second).Should(Succeed())
			_, err = execCommand("cluster", "detach", testClusterName)
			Expect(err).Should(Succeed())
		})

		It("Test generate service account kubeconfig", func() {
			_, workerCtx := initializeContext()
			// create service account kubeconfig in worker cluster
			key := time.Now().UnixNano()
			serviceAccountName := fmt.Sprintf("test-service-account-%d", key)
			serviceAccount := &v1.ServiceAccount{
				ObjectMeta: v12.ObjectMeta{Namespace: "kube-system", Name: serviceAccountName},
			}
			Expect(k8sClient.Create(workerCtx, serviceAccount)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Get(workerCtx, types.NamespacedName{Namespace: "kube-system", Name: serviceAccountName}, serviceAccount)).Should(Succeed())
				Expect(k8sClient.Delete(workerCtx, serviceAccount)).Should(Succeed())
			}()
			clusterRoleBindingName := fmt.Sprintf("test-cluster-role-binding-%d", key)
			clusterRoleBinding := &v14.ClusterRoleBinding{
				ObjectMeta: v12.ObjectMeta{Name: clusterRoleBindingName},
				Subjects:   []v14.Subject{{Kind: "ServiceAccount", Name: serviceAccountName, Namespace: "kube-system"}},
				RoleRef:    v14.RoleRef{Name: "cluster-admin", APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole"},
			}
			Expect(k8sClient.Create(workerCtx, clusterRoleBinding)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Get(workerCtx, types.NamespacedName{Namespace: "kube-system", Name: clusterRoleBindingName}, clusterRoleBinding)).Should(Succeed())
				Expect(k8sClient.Delete(workerCtx, clusterRoleBinding)).Should(Succeed())
			}()
			serviceAccount = &v1.ServiceAccount{}
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(workerCtx, types.NamespacedName{Name: serviceAccountName, Namespace: "kube-system"}, serviceAccount)).Should(Succeed())
				Expect(len(serviceAccount.Secrets)).Should(Equal(1))
			}, time.Second*30).Should(Succeed())
			secret := &v1.Secret{}
			Expect(k8sClient.Get(workerCtx, types.NamespacedName{Name: serviceAccount.Secrets[0].Name, Namespace: "kube-system"}, secret)).Should(Succeed())
			token, ok := secret.Data["token"]
			Expect(ok).Should(BeTrue())
			config, err := clientcmd.LoadFromFile(WorkerClusterKubeConfigPath)
			Expect(err).Should(Succeed())
			currentContext, ok := config.Contexts[config.CurrentContext]
			Expect(ok).Should(BeTrue())
			authInfo, ok := config.AuthInfos[currentContext.AuthInfo]
			Expect(ok).Should(BeTrue())
			authInfo.Token = string(token)
			authInfo.ClientKeyData = nil
			authInfo.ClientCertificateData = nil
			kubeconfigFilePath := fmt.Sprintf("/tmp/worker.sa-%d.kubeconfig", key)
			Expect(clientcmd.WriteToFile(*config, kubeconfigFilePath)).Should(Succeed())
			defer func() {
				Expect(os.Remove(kubeconfigFilePath)).Should(Succeed())
			}()
			// try to join cluster with service account token based kubeconfig
			clusterName := fmt.Sprintf("cluster-sa-%d", key)
			_, err = execCommand("cluster", "join", kubeconfigFilePath, "--name", clusterName)
			Expect(err).Should(Succeed())
			_, err = execCommand("cluster", "detach", clusterName)
			Expect(err).Should(Succeed())
		})

	})

	Context("Test EnvBinding Application", func() {

		var namespace string
		var testNamespace string
		var prodNamespace string
		var hubCtx context.Context
		var workerCtx context.Context

		BeforeEach(func() {
			hubCtx, workerCtx, namespace = initializeContextAndNamespace()
			_, _, testNamespace = initializeContextAndNamespace()
			_, _, prodNamespace = initializeContextAndNamespace()
		})

		AfterEach(func() {
			cleanUpNamespace(hubCtx, workerCtx, namespace)
			cleanUpNamespace(hubCtx, workerCtx, testNamespace)
			cleanUpNamespace(hubCtx, workerCtx, prodNamespace)
		})

		It("Test create EnvBinding Application", func() {
			// This test is going to cover multiple functions, including
			// 1. Multiple stage deployment for three environment
			// 2. Namespace selector.
			// 3. A special cluster: local cluster
			// 4. Component selector.
			By("apply application")
			app := &v1beta1.Application{}
			bs, err := ioutil.ReadFile("./testdata/app/example-envbinding-app.yaml")
			Expect(err).Should(Succeed())
			appYaml := strings.ReplaceAll(strings.ReplaceAll(string(bs), "TEST_NAMESPACE", testNamespace), "PROD_NAMESPACE", prodNamespace)
			Expect(yaml.Unmarshal([]byte(appYaml), app)).Should(Succeed())
			app.SetNamespace(namespace)
			err = k8sClient.Create(hubCtx, app)
			Expect(err).Should(Succeed())
			var hubDeployName string
			By("wait application resource ready")
			Eventually(func(g Gomega) {
				// check deployments in clusters
				deploys := &v13.DeploymentList{}
				g.Expect(k8sClient.List(hubCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(1))
				hubDeployName = deploys.Items[0].Name
				deploys = &v13.DeploymentList{}
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(2))
				deploys = &v13.DeploymentList{}
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(prodNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(2))
				// check component revision
				compRevs := &v13.ControllerRevisionList{}
				g.Expect(k8sClient.List(workerCtx, compRevs, client.InNamespace(prodNamespace))).Should(Succeed())
				g.Expect(len(compRevs.Items)).Should(Equal(2))
			}, time.Minute).Should(Succeed())
			Expect(hubDeployName).Should(Equal("data-worker"))
			// delete application
			By("delete application")
			Expect(k8sClient.Delete(hubCtx, app)).Should(Succeed())
			By("wait application resource delete")
			Eventually(func(g Gomega) {
				// check deployments in clusters
				deploys := &v13.DeploymentList{}
				g.Expect(k8sClient.List(hubCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(0))
				deploys = &v13.DeploymentList{}
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(0))
				// check component revision
				compRevs := &v13.ControllerRevisionList{}
				g.Expect(k8sClient.List(workerCtx, compRevs, client.InNamespace(prodNamespace))).Should(Succeed())
				g.Expect(len(compRevs.Items)).Should(Equal(0))
			}, time.Minute).Should(Succeed())
		})

		It("Test create EnvBinding Application with trait disable and without workflow, delete env, change env and add env", func() {
			// This test is going to cover multiple functions, including
			// 1. disable trait
			// 2. auto deploy2env workflow
			// 3. delete env
			// 4. change cluster in env
			// 5. add env
			By("apply application")
			app := &v1beta1.Application{}
			bs, err := ioutil.ReadFile("./testdata/app/example-envbinding-app-wo-workflow.yaml")
			Expect(err).Should(Succeed())
			appYaml := strings.ReplaceAll(string(bs), "TEST_NAMESPACE", testNamespace)
			Expect(yaml.Unmarshal([]byte(appYaml), app)).Should(Succeed())
			app.SetNamespace(testNamespace)
			namespacedName := client.ObjectKeyFromObject(app)
			err = k8sClient.Create(hubCtx, app)
			Expect(err).Should(Succeed())
			By("wait application resource ready")
			Eventually(func(g Gomega) {
				// check deployments in clusters
				deploys := &v13.DeploymentList{}
				g.Expect(k8sClient.List(hubCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(1))
				g.Expect(int(*deploys.Items[0].Spec.Replicas)).Should(Equal(2))
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(1))
				g.Expect(int(*deploys.Items[0].Spec.Replicas)).Should(Equal(1))
			}, time.Minute).Should(Succeed())
			By("test delete env")
			spec := &v1alpha1.EnvBindingSpec{}
			Expect(json.Unmarshal(app.Spec.Policies[0].Properties.Raw, spec)).Should(Succeed())
			envs := spec.Envs
			bs, err = json.Marshal(&v1alpha1.EnvBindingSpec{Envs: []v1alpha1.EnvConfig{envs[0]}})
			Expect(err).Should(Succeed())
			Expect(k8sClient.Get(hubCtx, namespacedName, app)).Should(Succeed())
			app.Spec.Policies[0].Properties.Raw = bs
			Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				deploys := &v13.DeploymentList{}
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(0))
			}, time.Minute).Should(Succeed())
			By("test change env cluster name")
			envs[0].Placement.ClusterSelector.Name = WorkerClusterName
			bs, err = json.Marshal(&v1alpha1.EnvBindingSpec{Envs: []v1alpha1.EnvConfig{envs[0]}})
			Expect(err).Should(Succeed())
			Expect(k8sClient.Get(hubCtx, namespacedName, app)).Should(Succeed())
			app.Spec.Policies[0].Properties.Raw = bs
			Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				deploys := &v13.DeploymentList{}
				g.Expect(k8sClient.List(hubCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(0))
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(1))
			}, time.Minute).Should(Succeed())
			By("test add env")
			envs[1].Placement.ClusterSelector.Name = multicluster.ClusterLocalName
			bs, err = json.Marshal(&v1alpha1.EnvBindingSpec{Envs: envs})
			Expect(err).Should(Succeed())
			Expect(k8sClient.Get(hubCtx, namespacedName, app)).Should(Succeed())
			app.Spec.Policies[0].Properties.Raw = bs
			Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				deploys := &v13.DeploymentList{}
				g.Expect(k8sClient.List(hubCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(1))
				g.Expect(int(*deploys.Items[0].Spec.Replicas)).Should(Equal(1))
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(1))
				g.Expect(int(*deploys.Items[0].Spec.Replicas)).Should(Equal(2))
			}, time.Minute).Should(Succeed())
			// delete application
			By("delete application")
			Expect(k8sClient.Delete(hubCtx, app)).Should(Succeed())
			By("wait application resource delete")
			Eventually(func(g Gomega) {
				// check deployments in clusters
				deploys := &v13.DeploymentList{}
				g.Expect(k8sClient.List(hubCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(0))
				deploys = &v13.DeploymentList{}
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(0))
			}, time.Minute).Should(Succeed())
		})
	})

})
