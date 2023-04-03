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
	"os"
	"strings"
	"time"

	"github.com/kubevela/pkg/controller/reconciler"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/pkg/util/rand"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	kubevelatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func initializeContext() (hubCtx context.Context, workerCtx context.Context) {
	hubCtx = context.Background()
	workerCtx = multicluster.ContextWithClusterName(hubCtx, WorkerClusterName)
	return
}

func initializeContextAndNamespace() (hubCtx context.Context, workerCtx context.Context, namespace string) {
	hubCtx, workerCtx = initializeContext()
	// initialize test namespace
	namespace = "test-mc-" + rand.RandomString(4)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	Expect(k8sClient.Create(hubCtx, ns.DeepCopy())).Should(Succeed())
	Expect(k8sClient.Create(workerCtx, ns.DeepCopy())).Should(Succeed())
	return
}

func cleanUpNamespace(hubCtx context.Context, workerCtx context.Context, namespace string) {
	hubNs := &corev1.Namespace{}
	Expect(k8sClient.Get(hubCtx, types.NamespacedName{Name: namespace}, hubNs)).Should(Succeed())
	Expect(k8sClient.Delete(hubCtx, hubNs)).Should(Succeed())
	workerNs := &corev1.Namespace{}
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
			_, err = execCommand("cluster", "join", "/tmp/worker.kubeconfig", "--name", oldClusterName, "-y")
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

		It("Test manage labels for cluster", func() {
			_, err := execCommand("cluster", "labels", "add", WorkerClusterName, "purpose=test,creator=e2e")
			Expect(err).Should(Succeed())
			out, err := execCommand("cluster", "list")
			Expect(err).Should(Succeed())
			Expect(out).Should(ContainSubstring("purpose"))
			_, err = execCommand("cluster", "labels", "del", WorkerClusterName, "purpose")
			Expect(err).Should(Succeed())
			out, err = execCommand("cluster", "list")
			Expect(err).Should(Succeed())
			Expect(out).ShouldNot(ContainSubstring("purpose"))
		})

		It("Test alias for cluster", func() {
			_, err := execCommand("cluster", "alias", WorkerClusterName, "alias-worker")
			Expect(err).Should(Succeed())
			out, err := execCommand("cluster", "list")
			Expect(err).Should(Succeed())
			Expect(out).Should(ContainSubstring("alias-worker"))
		})

		It("Test detach cluster with application use", func() {
			const testClusterName = "test-cluster"
			_, err := execCommand("cluster", "join", "/tmp/worker.kubeconfig", "--name", testClusterName)
			Expect(err).Should(Succeed())
			app := &v1beta1.Application{}
			bs, err := os.ReadFile("./testdata/app/example-lite-envbinding-app.yaml")
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
			By("create service account kubeconfig in worker cluster")
			key := time.Now().UnixNano()
			serviceAccountName := fmt.Sprintf("test-service-account-%d", key)
			serviceAccountNamespace := "kube-system"
			serviceAccount := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Namespace: serviceAccountNamespace, Name: serviceAccountName},
			}
			Expect(k8sClient.Create(workerCtx, serviceAccount)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Get(workerCtx, types.NamespacedName{Namespace: "kube-system", Name: serviceAccountName}, serviceAccount)).Should(Succeed())
				Expect(k8sClient.Delete(workerCtx, serviceAccount)).Should(Succeed())
			}()
			clusterRoleBindingName := fmt.Sprintf("test-cluster-role-binding-%d", key)
			clusterRoleBinding := &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: clusterRoleBindingName},
				Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: serviceAccountName, Namespace: serviceAccountNamespace}},
				RoleRef:    rbacv1.RoleRef{Name: "cluster-admin", APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole"},
			}
			Expect(k8sClient.Create(workerCtx, clusterRoleBinding)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Get(workerCtx, types.NamespacedName{Namespace: serviceAccountNamespace, Name: clusterRoleBindingName}, clusterRoleBinding)).Should(Succeed())
				Expect(k8sClient.Delete(workerCtx, clusterRoleBinding)).Should(Succeed())
			}()
			serviceAccount = &corev1.ServiceAccount{}
			By("Generating a token for SA")
			tr := &v1.TokenRequest{}
			token, err := k8sCli.CoreV1().ServiceAccounts(serviceAccountNamespace).CreateToken(workerCtx, serviceAccountName, tr, metav1.CreateOptions{})
			Expect(err).Should(BeNil())
			config, err := clientcmd.LoadFromFile(WorkerClusterKubeConfigPath)
			Expect(err).Should(Succeed())
			currentContext, ok := config.Contexts[config.CurrentContext]
			Expect(ok).Should(BeTrue())
			authInfo, ok := config.AuthInfos[currentContext.AuthInfo]
			Expect(ok).Should(BeTrue())
			authInfo.Token = token.Status.Token
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

		It("Test vela cluster export-config", func() {
			out, err := execCommand("cluster", "export-config")
			Expect(err).Should(Succeed())
			Expect(out).Should(ContainSubstring("name: " + WorkerClusterName))
		})

	})

	Context("Test multi-cluster Application", func() {

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
			bs, err := os.ReadFile("./testdata/app/example-envbinding-app.yaml")
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
				deploys := &appsv1.DeploymentList{}
				g.Expect(k8sClient.List(hubCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(1))
				hubDeployName = deploys.Items[0].Name
				deploys = &appsv1.DeploymentList{}
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(2))
				deploys = &appsv1.DeploymentList{}
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(prodNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(2))
			}, time.Minute).Should(Succeed())
			Expect(hubDeployName).Should(Equal("data-worker"))
			// delete application
			By("delete application")
			Expect(k8sClient.Delete(hubCtx, app)).Should(Succeed())
			By("wait application resource delete")
			Eventually(func(g Gomega) {
				// check deployments in clusters
				deploys := &appsv1.DeploymentList{}
				g.Expect(k8sClient.List(hubCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(0))
				deploys = &appsv1.DeploymentList{}
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(0))
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
			bs, err := os.ReadFile("./testdata/app/example-envbinding-app-wo-workflow.yaml")
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
				deploys := &appsv1.DeploymentList{}
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
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, namespacedName, app)).Should(Succeed())
				app.Spec.Policies[0].Properties.Raw = bs
				g.Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			}, 15*time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				deploys := &appsv1.DeploymentList{}
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(0))
			}, time.Minute).Should(Succeed())
			By("test change env cluster name")
			envs[0].Placement.ClusterSelector.Name = WorkerClusterName
			bs, err = json.Marshal(&v1alpha1.EnvBindingSpec{Envs: []v1alpha1.EnvConfig{envs[0]}})
			Expect(err).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, namespacedName, app)).Should(Succeed())
				app.Spec.Policies[0].Properties.Raw = bs
				g.Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			}, 15*time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				deploys := &appsv1.DeploymentList{}
				g.Expect(k8sClient.List(hubCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(0))
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(1))
			}, time.Minute).Should(Succeed())
			By("test add env")
			envs[1].Placement.ClusterSelector.Name = multicluster.ClusterLocalName
			bs, err = json.Marshal(&v1alpha1.EnvBindingSpec{Envs: envs})
			Expect(err).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, namespacedName, app)).Should(Succeed())
				app.Spec.Policies[0].Properties.Raw = bs
				g.Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			}, 15*time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				deploys := &appsv1.DeploymentList{}
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
				deploys := &appsv1.DeploymentList{}
				g.Expect(k8sClient.List(hubCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(0))
				deploys = &appsv1.DeploymentList{}
				g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(testNamespace))).Should(Succeed())
				g.Expect(len(deploys.Items)).Should(Equal(0))
			}, time.Minute).Should(Succeed())
		})

		It("Test deploy multi-cluster application with target", func() {
			By("apply application")
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "test-app-target"},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:       "test-busybox",
						Type:       "webservice",
						Properties: &runtime.RawExtension{Raw: []byte(`{"image":"busybox","cmd":["sleep","86400"]}`)},
					}},
					Policies: []v1beta1.AppPolicy{{
						Name:       "topology-local",
						Type:       "topology",
						Properties: &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"clusters":["local"],"namespace":"%s"}`, testNamespace))},
					}, {
						Name:       "topology-remote",
						Type:       "topology",
						Properties: &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"clusters":["%s"],"namespace":"%s"}`, WorkerClusterName, prodNamespace))},
					}},
				},
			}
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Name: "test-busybox", Namespace: testNamespace}, &appsv1.Deployment{})).Should(Succeed())
				g.Expect(k8sClient.Get(workerCtx, types.NamespacedName{Name: "test-busybox", Namespace: prodNamespace}, &appsv1.Deployment{})).Should(Succeed())
			}, time.Minute).Should(Succeed())
		})

		It("Test re-deploy application with old revisions", func() {
			By("apply application")
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "test-app-target"},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:       "test-busybox",
						Type:       "webservice",
						Properties: &runtime.RawExtension{Raw: []byte(`{"image":"busybox","cmd":["sleep","86400"]}`)},
					}},
					Policies: []v1beta1.AppPolicy{{
						Name:       "topology-local",
						Type:       "topology",
						Properties: &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"clusters":["local"],"namespace":"%s"}`, testNamespace))},
					},
					}}}
			oam.SetPublishVersion(app, "alpha")
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Name: "test-busybox", Namespace: testNamespace}, &appsv1.Deployment{})).Should(Succeed())
			}, time.Minute).Should(Succeed())

			By("update application to new version")
			appKey := client.ObjectKeyFromObject(app)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, appKey, app)).Should(Succeed())
				app.Spec.Components[0].Name = "test-busybox-v2"
				oam.SetPublishVersion(app, "beta")
				g.Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			}, 15*time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Name: "test-busybox-v2", Namespace: testNamespace}, &appsv1.Deployment{})).Should(Succeed())
				err := k8sClient.Get(hubCtx, types.NamespacedName{Name: "test-busybox", Namespace: testNamespace}, &appsv1.Deployment{})
				g.Expect(kerrors.IsNotFound(err)).Should(BeTrue())
			}, time.Minute).Should(Succeed())

			By("Re-publish application to v1")
			_, err := execCommand("up", appKey.Name, "-n", appKey.Namespace, "--revision", appKey.Name+"-v1", "--publish-version", "v1.0")
			Expect(err).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Name: "test-busybox", Namespace: testNamespace}, &appsv1.Deployment{})).Should(Succeed())
				err := k8sClient.Get(hubCtx, types.NamespacedName{Name: "test-busybox-v2", Namespace: testNamespace}, &appsv1.Deployment{})
				g.Expect(kerrors.IsNotFound(err)).Should(BeTrue())
			}, 2*time.Minute).Should(Succeed())
		})

		It("Test applications sharing resources", func() {
			createApp := func(name string) *v1beta1.Application {
				return &v1beta1.Application{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
					Spec: v1beta1.ApplicationSpec{
						Components: []common.ApplicationComponent{{
							Name:       "shared-resource-" + name,
							Type:       "k8s-objects",
							Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"shared"},"data":{"key":"value"}}]}`)},
						}, {
							Name:       "no-shared-resource-" + name,
							Type:       "k8s-objects",
							Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"non-shared-` + name + `"},"data":{"key":"value"}}]}`)},
						}},
						Policies: []v1beta1.AppPolicy{{
							Type:       "shared-resource",
							Name:       "shared-resource",
							Properties: &runtime.RawExtension{Raw: []byte(`{"rules":[{"selector":{"componentNames":["shared-resource-` + name + `"]}}]}`)},
						}},
					},
				}
			}
			app1 := createApp("app1")
			Expect(k8sClient.Create(hubCtx, app1)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app1), app1)).Should(Succeed())
				g.Expect(app1.Status.Phase).Should(Equal(common.ApplicationRunning))
			}, 10*time.Second).Should(Succeed())
			app2 := createApp("app2")
			Expect(k8sClient.Create(hubCtx, app2)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app2), app2)).Should(Succeed())
				g.Expect(app2.Status.Phase).Should(Equal(common.ApplicationRunning))
			}, 10*time.Second).Should(Succeed())
			app3 := createApp("app3")
			Expect(k8sClient.Create(hubCtx, app3)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app3), app3)).Should(Succeed())
				g.Expect(app3.Status.Phase).Should(Equal(common.ApplicationRunning))
			}, 10*time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				cm := &corev1.ConfigMap{}
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "shared"}, cm)).Should(Succeed())
				g.Expect(cm.GetAnnotations()[oam.AnnotationAppSharedBy]).Should(SatisfyAll(ContainSubstring("app1"), ContainSubstring("app2"), ContainSubstring("app3")))
				g.Expect(cm.GetLabels()[oam.LabelAppName]).Should(SatisfyAny(Equal("app1"), Equal("app2"), Equal("app3")))
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "non-shared-app1"}, &corev1.ConfigMap{})).Should(Succeed())
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "non-shared-app2"}, &corev1.ConfigMap{})).Should(Succeed())
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "non-shared-app3"}, &corev1.ConfigMap{})).Should(Succeed())
			}, 45*time.Second).Should(Succeed())
			Expect(k8sClient.Delete(hubCtx, app2)).Should(Succeed())
			Eventually(func(g Gomega) {
				cm := &corev1.ConfigMap{}
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "shared"}, cm)).Should(Succeed())
				g.Expect(cm.GetAnnotations()[oam.AnnotationAppSharedBy]).Should(SatisfyAll(ContainSubstring("app1"), ContainSubstring("app3")))
				g.Expect(cm.GetAnnotations()[oam.AnnotationAppSharedBy]).ShouldNot(SatisfyAny(ContainSubstring("app2")))
				g.Expect(cm.GetLabels()[oam.LabelAppName]).Should(SatisfyAny(Equal("app1"), Equal("app3")))
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "non-shared-app1"}, &corev1.ConfigMap{})).Should(Succeed())
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "non-shared-app2"}, &corev1.ConfigMap{})).Should(Satisfy(kerrors.IsNotFound))
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "non-shared-app3"}, &corev1.ConfigMap{})).Should(Succeed())
			}, 10*time.Second).Should(Succeed())
			Expect(k8sClient.Delete(hubCtx, app1)).Should(Succeed())
			Eventually(func(g Gomega) {
				cm := &corev1.ConfigMap{}
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "shared"}, cm)).Should(Succeed())
				g.Expect(cm.GetAnnotations()[oam.AnnotationAppSharedBy]).Should(SatisfyAll(ContainSubstring("app3")))
				g.Expect(cm.GetAnnotations()[oam.AnnotationAppSharedBy]).ShouldNot(SatisfyAny(ContainSubstring("app1"), ContainSubstring("app2")))
				g.Expect(cm.GetLabels()[oam.LabelAppName]).Should(Equal("app3"))
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "non-shared-app1"}, &corev1.ConfigMap{})).Should(Satisfy(kerrors.IsNotFound))
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "non-shared-app3"}, &corev1.ConfigMap{})).Should(Succeed())
			}, 10*time.Second).Should(Succeed())
			Expect(k8sClient.Delete(hubCtx, app3)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "shared"}, &corev1.ConfigMap{})).Should(Satisfy(kerrors.IsNotFound))
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "non-shared-app3"}, &corev1.ConfigMap{})).Should(Satisfy(kerrors.IsNotFound))
			}, 10*time.Second).Should(Succeed())
		})

		It("Test applications with bad resource", func() {
			bs, err := os.ReadFile("./testdata/app/app-bad-resource.yaml")
			Expect(err).Should(Succeed())
			appYaml := strings.ReplaceAll(string(bs), "TEST_NAMESPACE", testNamespace)
			app := &v1beta1.Application{}
			Expect(yaml.Unmarshal([]byte(appYaml), app)).Should(Succeed())
			ctx := context.Background()
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunningWorkflow))
				g.Expect(len(app.Status.Workflow.Steps) > 0).Should(BeTrue())
				g.Expect(app.Status.Workflow.Steps[0].Message).Should(ContainSubstring("is invalid"))
				rts := &v1beta1.ResourceTrackerList{}
				g.Expect(k8sClient.List(hubCtx, rts, client.MatchingLabels{oam.LabelAppName: app.Name, oam.LabelAppNamespace: app.Namespace})).Should(Succeed())
				g.Expect(len(rts.Items)).Should(Equal(0))
			}, 20*time.Second).Should(Succeed())
			Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Satisfy(kerrors.IsNotFound))
			}, 10*time.Second).Should(Succeed())
		})

		It("Test applications with env and storage trait", func() {
			bs, err := os.ReadFile("./testdata/app/app-with-env-and-storage.yaml")
			Expect(err).Should(Succeed())
			appYaml := strings.ReplaceAll(string(bs), "TEST_NAMESPACE", testNamespace)
			app := &v1beta1.Application{}
			Expect(yaml.Unmarshal([]byte(appYaml), app)).Should(Succeed())
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			}, 20*time.Second).Should(Succeed())
		})

		It("Test applications with gc policy change (onAppUpdate -> never)", func() {
			bs, err := os.ReadFile("./testdata/app/app-gc-policy-change.yaml")
			Expect(err).Should(Succeed())
			appYaml := strings.ReplaceAll(string(bs), "TEST_NAMESPACE", testNamespace)
			app := &v1beta1.Application{}
			Expect(yaml.Unmarshal([]byte(appYaml), app)).Should(Succeed())
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			}, 20*time.Second).Should(Succeed())

			By("update gc policy to never")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app))
				gcPolicy := &v1alpha1.GarbageCollectPolicySpec{}
				g.Expect(json.Unmarshal(app.Spec.Policies[0].Properties.Raw, gcPolicy)).Should(Succeed())
				gcPolicy.Rules[0].Strategy = v1alpha1.GarbageCollectStrategyNever
				bs, err = json.Marshal(gcPolicy)
				g.Expect(err).Should(Succeed())
				app.Spec.Policies[0].Properties = &runtime.RawExtension{Raw: bs}
				g.Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			}, 10*time.Second).Should(Succeed())

			By("check app updated and resource still exists")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
				g.Expect(app.Status.ObservedGeneration).Should(Equal(int64(2)))
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: testNamespace, Name: "gc-policy-test"}, &corev1.ConfigMap{})).Should(Succeed())
			}, 20*time.Second).Should(Succeed())

			By("update app to new object")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app))
				app.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"another"},"data":{"key":"new-val"}}]}`)}
				g.Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			}).Should(Succeed())

			By("check app updated and resource still exists")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
				g.Expect(app.Status.ObservedGeneration).Should(Equal(int64(3)))
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: testNamespace, Name: "gc-policy-test"}, &corev1.ConfigMap{})).Should(Succeed())
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: testNamespace, Name: "another"}, &corev1.ConfigMap{})).Should(Succeed())
			}, 20*time.Second).Should(Succeed())

			By("delete app and check resource")
			Eventually(func(g Gomega) {
				g.Expect(client.IgnoreNotFound(k8sClient.Delete(hubCtx, app))).Should(Succeed())
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: testNamespace, Name: "gc-policy-test"}, &corev1.ConfigMap{})).Should(Succeed())
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: testNamespace, Name: "another"}, &corev1.ConfigMap{})).Should(Satisfy(kerrors.IsNotFound))
			})
		})

		It("Test Application with env in webservice and labels & storage trait", func() {
			bs, err := os.ReadFile("./testdata/app/app-with-env-labels-storage.yaml")
			Expect(err).Should(Succeed())
			app := &v1beta1.Application{}
			Expect(yaml.Unmarshal(bs, app)).Should(Succeed())
			app.SetNamespace(namespace)
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			deploy := &appsv1.Deployment{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "test"}, deploy)).Should(Succeed())
			}, 15*time.Second).Should(Succeed())
			Expect(deploy.GetLabels()["key"]).Should(Equal("val"))
			Expect(len(deploy.Spec.Template.Spec.Containers[0].Env)).Should(Equal(1))
			Expect(deploy.Spec.Template.Spec.Containers[0].Env[0].Name).Should(Equal("testKey"))
			Expect(deploy.Spec.Template.Spec.Containers[0].Env[0].Value).Should(Equal("testValue"))
			Expect(len(deploy.Spec.Template.Spec.Volumes)).Should(Equal(1))
		})

		It("Test application with collect-service-endpoint and export-data", func() {
			By("create application")
			bs, err := os.ReadFile("./testdata/app/app-collect-service-endpoint-and-export.yaml")
			Expect(err).Should(Succeed())
			app := &v1beta1.Application{}
			Expect(yaml.Unmarshal(bs, app)).Should(Succeed())
			app.SetNamespace(testNamespace)
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			}, 20*time.Second).Should(Succeed())

			By("test dispatched resource")
			svc := &corev1.Service{}
			Expect(k8sClient.Get(hubCtx, client.ObjectKey{Namespace: testNamespace, Name: "busybox"}, svc)).Should(Succeed())
			host := "busybox." + testNamespace
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(hubCtx, client.ObjectKey{Namespace: testNamespace, Name: app.Name}, cm)).Should(Succeed())
			Expect(cm.Data["host"]).Should(Equal(host))
			Expect(k8sClient.Get(workerCtx, client.ObjectKey{Namespace: testNamespace, Name: app.Name}, cm)).Should(Succeed())
			Expect(cm.Data["host"]).Should(Equal(host))
		})

		It("Test application with workflow change will rerun", func() {
			By("create application")
			bs, err := os.ReadFile("./testdata/app/app-lite-with-workflow.yaml")
			Expect(err).Should(Succeed())
			app := &v1beta1.Application{}
			Expect(yaml.Unmarshal(bs, app)).Should(Succeed())
			app.SetNamespace(testNamespace)
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			}, 20*time.Second).Should(Succeed())
			Expect(k8sClient.Get(hubCtx, client.ObjectKey{Namespace: testNamespace, Name: "data-worker"}, &appsv1.Deployment{})).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
				app.Spec.Workflow.Steps[0].Properties = &runtime.RawExtension{Raw: []byte(`{"policies":["worker"]}`)}
				g.Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			}, 10*time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKey{Namespace: testNamespace, Name: "data-worker"}, &appsv1.Deployment{})).Should(Satisfy(kerrors.IsNotFound))
				g.Expect(k8sClient.Get(workerCtx, client.ObjectKey{Namespace: testNamespace, Name: "data-worker"}, &appsv1.Deployment{})).Should(Succeed())
			}, 20*time.Second).Should(Succeed())
		})

		It("Test application with apply-component and cluster", func() {
			By("create application")
			bs, err := os.ReadFile("./testdata/app/app-component-with-cluster.yaml")
			Expect(err).Should(Succeed())
			app := &v1beta1.Application{}
			Expect(yaml.Unmarshal(bs, app)).Should(Succeed())
			app.SetNamespace(testNamespace)
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			}, 20*time.Second).Should(Succeed())
			Expect(k8sClient.Get(workerCtx, client.ObjectKey{Namespace: testNamespace, Name: "component-cluster"}, &appsv1.Deployment{})).Should(Succeed())
		})

		It("Test application with component using cluster context", func() {
			By("Create definition")
			bs, err := os.ReadFile("./testdata/def/cluster-config.yaml")
			Expect(err).Should(Succeed())
			def := &v1beta1.ComponentDefinition{}
			Expect(yaml.Unmarshal(bs, def)).Should(Succeed())
			def.SetNamespace(kubevelatypes.DefaultKubeVelaNS)
			Expect(k8sClient.Create(hubCtx, def)).Should(Succeed())
			defKey := client.ObjectKeyFromObject(def)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, defKey, def)).Should(Succeed())
			}, 5*time.Second).Should(Succeed())
			bs, err = os.ReadFile("./testdata/app/app-component-with-cluster-context.yaml")
			Expect(err).Should(Succeed())
			app := &v1beta1.Application{}
			Expect(yaml.Unmarshal(bs, app)).Should(Succeed())
			app.SetNamespace(testNamespace)
			Eventually(func(g Gomega) { // informer may have latency for the added definition
				g.Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
			key := client.ObjectKeyFromObject(app)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, key, app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			}, 20*time.Second).Should(Succeed())
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: testNamespace, Name: "test"}, cm)).Should(Succeed())
			Expect(cm.Data["cluster"]).Should(Equal("local"))
			Expect(k8sClient.Get(workerCtx, types.NamespacedName{Namespace: testNamespace, Name: "test"}, cm)).Should(Succeed())
			Expect(cm.Data["cluster"]).Should(Equal("cluster-worker"))
			Expect(k8sClient.Delete(hubCtx, def)).Should(Succeed())
		})

		It("Test application with read-only policy", func() {
			By("create deployment")
			bs, err := os.ReadFile("./testdata/app/standalone/deployment-busybox.yaml")
			Expect(err).Should(Succeed())
			deploy := &appsv1.Deployment{}
			Expect(yaml.Unmarshal(bs, deploy)).Should(Succeed())
			deploy.SetNamespace(namespace)
			Expect(k8sClient.Create(hubCtx, deploy)).Should(Succeed())
			By("create application")
			bs, err = os.ReadFile("./testdata/app/app-readonly.yaml")
			Expect(err).Should(Succeed())
			app := &v1beta1.Application{}
			Expect(yaml.Unmarshal(bs, app)).Should(Succeed())
			app.SetNamespace(namespace)
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
				g.Expect(app.Status.Workflow).ShouldNot(BeNil())
				g.Expect(len(app.Status.Workflow.Steps)).ShouldNot(Equal(0))
				g.Expect(app.Status.Workflow.Steps[0].Phase).Should(Equal(workflowv1alpha1.WorkflowStepPhaseFailed))
			}, 20*time.Second).Should(Succeed())
			By("update application")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
				app.Spec.Components[0].Name = "busybox-ref"
				g.Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			}, 20*time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			}, 20*time.Second).Should(Succeed())
			By("delete application")
			appKey := client.ObjectKeyFromObject(app)
			deployKey := client.ObjectKeyFromObject(deploy)
			Expect(k8sClient.Delete(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(kerrors.IsNotFound(k8sClient.Get(hubCtx, appKey, app))).Should(BeTrue())
			}, 20*time.Second).Should(Succeed())
			Expect(k8sClient.Get(hubCtx, deployKey, deploy)).Should(Succeed())
		})

		It("Test application with take-over policy", func() {
			By("create deployment")
			bs, err := os.ReadFile("./testdata/app/standalone/deployment-busybox.yaml")
			Expect(err).Should(Succeed())
			deploy := &appsv1.Deployment{}
			Expect(yaml.Unmarshal(bs, deploy)).Should(Succeed())
			deploy.SetNamespace(namespace)
			Expect(k8sClient.Create(hubCtx, deploy)).Should(Succeed())
			By("create application")
			bs, err = os.ReadFile("./testdata/app/app-takeover.yaml")
			Expect(err).Should(Succeed())
			app := &v1beta1.Application{}
			Expect(yaml.Unmarshal(bs, app)).Should(Succeed())
			app.SetNamespace(namespace)
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			}, 20*time.Second).Should(Succeed())
			By("delete application")
			appKey := client.ObjectKeyFromObject(app)
			deployKey := client.ObjectKeyFromObject(deploy)
			Expect(k8sClient.Delete(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(kerrors.IsNotFound(k8sClient.Get(hubCtx, appKey, app))).Should(BeTrue())
				g.Expect(kerrors.IsNotFound(k8sClient.Get(hubCtx, deployKey, deploy))).Should(BeTrue())
			}, 20*time.Second).Should(Succeed())
		})

		It("Test application with input/output in deploy step", func() {
			By("create application")
			bs, err := os.ReadFile("./testdata/app/app-deploy-io.yaml")
			Expect(err).Should(Succeed())
			app := &v1beta1.Application{}
			Expect(yaml.Unmarshal(bs, app)).Should(Succeed())
			app.SetNamespace(namespace)
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			}, 30*time.Second).Should(Succeed())

			By("Check input/output work properly")
			cm := &corev1.ConfigMap{}
			cmKey := client.ObjectKey{Namespace: namespace, Name: "deployment-msg"}
			var (
				ipLocal  string
				ipWorker string
			)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, cmKey, cm)).Should(Succeed())
				g.Expect(cm.Data["msg"]).Should(Equal("Deployment has minimum availability."))
				ipLocal = cm.Data["ip"]
				g.Expect(ipLocal).ShouldNot(BeEmpty())
			}, 20*time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(workerCtx, cmKey, cm)).Should(Succeed())
				g.Expect(cm.Data["msg"]).Should(Equal("Deployment has minimum availability."))
				ipWorker = cm.Data["ip"]
				g.Expect(ipWorker).ShouldNot(BeEmpty())
			}, 20*time.Second).Should(Succeed())
			Expect(ipLocal).ShouldNot(Equal(ipWorker))

			By("delete application")
			appKey := client.ObjectKeyFromObject(app)
			Expect(k8sClient.Delete(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(kerrors.IsNotFound(k8sClient.Get(hubCtx, appKey, app))).Should(BeTrue())
			}, 20*time.Second).Should(Succeed())
		})

		It("Test application with failed gc and restart workflow", func() {
			By("duplicate cluster")
			secret := &corev1.Secret{}
			const secretName = "disconnection-test"
			Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: kubevelatypes.DefaultKubeVelaNS, Name: WorkerClusterName}, secret)).Should(Succeed())
			secret.SetName(secretName)
			secret.SetResourceVersion("")
			Expect(k8sClient.Create(hubCtx, secret)).Should(Succeed())
			defer func() {
				_ = k8sClient.Delete(hubCtx, secret)
			}()

			By("create cluster normally")
			bs, err := os.ReadFile("./testdata/app/app-disconnection-test.yaml")
			Expect(err).Should(Succeed())
			app := &v1beta1.Application{}
			Expect(yaml.Unmarshal(bs, app)).Should(Succeed())
			app.SetNamespace(namespace)
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, key, app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("disconnect cluster")
			Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: kubevelatypes.DefaultKubeVelaNS, Name: secretName}, secret)).Should(Succeed())
			secret.Data["endpoint"] = []byte("https://1.2.3.4:9999")
			Expect(k8sClient.Update(hubCtx, secret)).Should(Succeed())

			By("update application")
			Expect(k8sClient.Get(hubCtx, key, app)).Should(Succeed())
			app.Spec.Policies = nil
			Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, key, app)).Should(Succeed())
				g.Expect(app.Status.ObservedGeneration).Should(Equal(app.Generation))
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
				rts := &v1beta1.ResourceTrackerList{}
				g.Expect(k8sClient.List(hubCtx, rts, client.MatchingLabels{oam.LabelAppName: key.Name, oam.LabelAppNamespace: key.Namespace})).Should(Succeed())
				cnt := 0
				for _, item := range rts.Items {
					if item.Spec.Type == v1beta1.ResourceTrackerTypeVersioned {
						cnt++
					}
				}
				g.Expect(cnt).Should(Equal(2))
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("try update application again")
			Expect(k8sClient.Get(hubCtx, key, app)).Should(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations[oam.AnnotationPublishVersion] = "test"
			Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, key, app)).Should(Succeed())
				g.Expect(app.Status.LatestRevision).ShouldNot(BeNil())
				g.Expect(app.Status.LatestRevision.Revision).Should(Equal(int64(3)))
				g.Expect(app.Status.ObservedGeneration).Should(Equal(app.Generation))
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			}).WithTimeout(1 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			By("clear disconnection cluster secret")
			Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: kubevelatypes.DefaultKubeVelaNS, Name: secretName}, secret)).Should(Succeed())
			Expect(k8sClient.Delete(hubCtx, secret)).Should(Succeed())

			By("wait gc application completed")
			Eventually(func(g Gomega) {
				rts := &v1beta1.ResourceTrackerList{}
				g.Expect(k8sClient.List(hubCtx, rts, client.MatchingLabels{oam.LabelAppName: key.Name, oam.LabelAppNamespace: key.Namespace})).Should(Succeed())
				cnt := 0
				for _, item := range rts.Items {
					if item.Spec.Type == v1beta1.ResourceTrackerTypeVersioned {
						cnt++
					}
				}
				g.Expect(cnt).Should(Equal(1))
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("Test application with gc policy and shared-resource policy", func() {
			app := &v1beta1.Application{}
			bs, err := os.ReadFile("./testdata/app/app-gc-shared.yaml")
			Expect(err).Should(Succeed())
			Expect(yaml.Unmarshal(bs, app)).Should(Succeed())
			app.SetNamespace(namespace)
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			appKey := client.ObjectKeyFromObject(app)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, appKey, app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
				g.Expect(k8sClient.Get(hubCtx, appKey, &corev1.ConfigMap{})).Should(Succeed())
			}).WithTimeout(10 * time.Second).Should(Succeed())
			Expect(k8sClient.Get(hubCtx, appKey, app)).Should(Succeed())
			Expect(k8sClient.Delete(hubCtx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(kerrors.IsNotFound(k8sClient.Get(hubCtx, appKey, app))).Should(BeTrue())
				g.Expect(k8sClient.Get(hubCtx, appKey, &corev1.ConfigMap{})).Should(Succeed())
			}).WithTimeout(10 * time.Second).Should(Succeed())
		})

		It("Test application skip webservice component health check", func() {
			td := &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "ignore-health-check", Namespace: namespace},
				Spec: v1beta1.TraitDefinitionSpec{
					Schematic: &common.Schematic{CUE: &common.CUE{
						Template: `
							patch: metadata: annotations: "app.oam.dev/disable-health-check": parameter.key
							parameter: key: string
						`,
					}},
					Status: &common.Status{HealthPolicy: `isHealth: context.parameter.key == "true"`},
				},
			}
			Expect(k8sClient.Create(hubCtx, td)).Should(Succeed())

			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: namespace},
				Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{
					Type:       "webservice",
					Name:       "test",
					Properties: &runtime.RawExtension{Raw: []byte(`{"image":"bad"}`)},
					Traits: []common.ApplicationTrait{{
						Type:       "ignore-health-check",
						Properties: &runtime.RawExtension{Raw: []byte(`{"key":"false"}`)},
					}},
				}}},
			}
			Eventually(func(g Gomega) { // in case the trait definition has not been watched by vela-core
				g.Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
			appKey := client.ObjectKeyFromObject(app)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, appKey, app)).Should(Succeed())
				g.Expect(len(app.Status.Services) > 0).Should(BeTrue())
				g.Expect(len(app.Status.Services[0].Traits) > 0).Should(BeTrue())
				g.Expect(app.Status.Services[0].Traits[0].Healthy).Should(BeFalse())
			}).WithTimeout(10 * time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, appKey, app)).Should(Succeed())
				app.Spec.Components[0].Traits[0].Properties.Raw = []byte(`{"key":"true"}`)
				g.Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			}).WithTimeout(10 * time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, appKey, app)).Should(Succeed())
				g.Expect(len(app.Status.Services) > 0).Should(BeTrue())
				g.Expect(len(app.Status.Services[0].Traits) > 0).Should(BeTrue())
				g.Expect(app.Status.Services[0].Traits[0].Healthy).Should(BeTrue())
			}).WithTimeout(20 * time.Second).Should(Succeed())
		})

		It("Test pause application", func() {
			app := &v1beta1.Application{}
			bs, err := os.ReadFile("./testdata/app/app-pause.yaml")
			Expect(err).Should(Succeed())
			Expect(yaml.Unmarshal(bs, app)).Should(Succeed())
			app.SetNamespace(namespace)
			Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
			time.Sleep(10 * time.Second)
			appKey := client.ObjectKeyFromObject(app)
			Expect(k8sClient.Get(hubCtx, appKey, app)).Should(Succeed())
			Expect(app.Status.Workflow).Should(BeNil())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, appKey, app)).Should(Succeed())
				reconciler.SetPause(app, false)
				g.Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
			}).WithTimeout(5 * time.Second).WithPolling(time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(hubCtx, appKey, app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			}).WithTimeout(15 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
			Expect(k8sClient.Delete(hubCtx, app)).Should(Succeed())
		})
	})
})
