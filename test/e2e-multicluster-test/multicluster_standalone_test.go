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
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/workflow/operation"
)

var _ = Describe("Test multicluster standalone scenario", func() {
	waitObject := func(ctx context.Context, un unstructured.Unstructured) {
		Eventually(func(g Gomega) error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(&un), &un)
		}, 10*time.Second).Should(Succeed())
	}
	var namespace string
	var hubCtx context.Context
	var workerCtx context.Context

	readFile := func(filename string) *unstructured.Unstructured {
		bs, err := os.ReadFile("./testdata/app/standalone/" + filename)
		Expect(err).Should(Succeed())
		un := &unstructured.Unstructured{}
		Expect(yaml.Unmarshal(bs, un)).Should(Succeed())
		un.SetNamespace(namespace)
		return un
	}

	applyFile := func(filename string) {
		un := readFile(filename)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Create(context.Background(), un)).Should(Succeed())
		}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
	}

	BeforeEach(func() {
		hubCtx, workerCtx, namespace = initializeContextAndNamespace()
	})

	AfterEach(func() {
		cleanUpNamespace(hubCtx, workerCtx, namespace)
	})

	It("Test standalone app", func() {
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
			g.Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: "podinfo"}, app)).Should(Succeed())
			g.Expect(app.Status.Workflow).ShouldNot(BeNil())
			g.Expect(app.Status.Workflow.Mode).Should(Equal("DAG-DAG"))
			g.Expect(k8sClient.Delete(context.Background(), app)).Should(Succeed())
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

	It("Test standalone app with publish version", func() {
		By("Apply resources")

		nsLocal := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace + "-local"}}
		Expect(k8sClient.Create(hubCtx, nsLocal)).Should(Succeed())
		defer func() {
			_ = k8sClient.Delete(hubCtx, nsLocal)
		}()

		deploy := readFile("deployment.yaml")
		Expect(k8sClient.Create(hubCtx, deploy)).Should(Succeed())
		waitObject(hubCtx, *deploy)
		workflow := readFile("workflow-suspend.yaml")
		Expect(k8sClient.Create(hubCtx, workflow)).Should(Succeed())
		waitObject(hubCtx, *workflow)
		policy := readFile("policy-zero-replica.yaml")
		Expect(k8sClient.Create(hubCtx, policy)).Should(Succeed())
		waitObject(hubCtx, *policy)
		app := readFile("app-with-publish-version.yaml")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
		}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		appKey := client.ObjectKeyFromObject(app)

		Eventually(func(g Gomega) {
			_app := &v1beta1.Application{}
			g.Expect(k8sClient.Get(hubCtx, appKey, _app)).Should(Succeed())
			g.Expect(_app.Status.Phase).Should(Equal(oamcomm.ApplicationWorkflowSuspending))
		}, 15*time.Second).Should(Succeed())

		Expect(k8sClient.Delete(hubCtx, workflow)).Should(Succeed())
		Expect(k8sClient.Delete(hubCtx, policy)).Should(Succeed())

		Eventually(func(g Gomega) {
			_app := &v1beta1.Application{}
			g.Expect(k8sClient.Get(hubCtx, appKey, _app)).Should(Succeed())
			g.Expect(operation.ResumeWorkflow(hubCtx, k8sClient, _app, "")).Should(Succeed())
		}, 15*time.Second).Should(Succeed())

		// test application can run without external policies and workflow since they are recorded in the application revision
		_app := &v1beta1.Application{}
		Eventually(func(g Gomega) {
			deploys := &v1.DeploymentList{}
			g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
			g.Expect(len(deploys.Items)).Should(Equal(1))
			g.Expect(deploys.Items[0].Spec.Replicas).Should(Equal(pointer.Int32(0)))
			g.Expect(k8sClient.Get(hubCtx, appKey, _app)).Should(Succeed())
			g.Expect(_app.Status.Phase).Should(Equal(oamcomm.ApplicationRunning))
		}, 30*time.Second).Should(Succeed())

		// update application without updating publishVersion
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(hubCtx, appKey, _app)).Should(Succeed())
			_app.Spec.Policies[0].Properties = &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"clusters":["local"],"namespace":"%s"}`, nsLocal.Name))}
			g.Expect(k8sClient.Update(hubCtx, _app)).Should(Succeed())
		}, 10*time.Second).Should(Succeed())

		// application should no re-run workflow
		time.Sleep(10 * time.Second)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(hubCtx, appKey, _app)).Should(Succeed())
			g.Expect(_app.Status.Phase).Should(Equal(oamcomm.ApplicationRunning))
			apprevs := &v1beta1.ApplicationRevisionList{}
			g.Expect(k8sClient.List(hubCtx, apprevs, client.InNamespace(namespace))).Should(Succeed())
			g.Expect(len(apprevs.Items)).Should(Equal(1))
		}, 10*time.Second).Should(Succeed())

		// update application with publishVersion
		applyFile("policy.yaml")
		applyFile("workflow.yaml")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(hubCtx, appKey, _app)).Should(Succeed())
			_app.Annotations[oam.AnnotationPublishVersion] = "beta"
			g.Expect(k8sClient.Update(hubCtx, _app)).Should(Succeed())
		}, 10*time.Second).Should(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(hubCtx, appKey, _app)).Should(Succeed())
			g.Expect(_app.Status.Phase).Should(Equal(oamcomm.ApplicationRunning))
			deploys := &v1.DeploymentList{}
			g.Expect(k8sClient.List(workerCtx, deploys, client.InNamespace(namespace))).Should(Succeed())
			g.Expect(len(deploys.Items)).Should(Equal(0))
			g.Expect(k8sClient.List(hubCtx, deploys, client.InNamespace(nsLocal.Name))).Should(Succeed())
			g.Expect(len(deploys.Items)).Should(Equal(1))
			g.Expect(deploys.Items[0].Spec.Replicas).Should(Equal(pointer.Int32(3)))
		}, 30*time.Second).Should(Succeed())
	})

	It("Test rollback application with publish version", func() {
		By("Apply application successfully")
		applyFile("topology-policy.yaml")
		applyFile("workflow-deploy-worker.yaml")
		applyFile("deployment-busybox.yaml")
		applyFile("app-with-publish-version-busybox.yaml")
		app := &v1beta1.Application{}
		appKey := types.NamespacedName{Namespace: namespace, Name: "busybox"}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(hubCtx, appKey, app)).Should(Succeed())
			g.Expect(app.Status.Phase).Should(Equal(oamcomm.ApplicationRunning))
		}, 30*time.Second).Should(Succeed())

		By("Update Application to first failed version")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(hubCtx, appKey, app)).Should(Succeed())
			app.Annotations[oam.AnnotationPublishVersion] = "alpha2"
			app.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(`{"image":"busybox:bad"}`)}
			g.Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
		}, 30*time.Second).Should(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(hubCtx, appKey, app)).Should(Succeed())
			g.Expect(app.Status.Phase).Should(Equal(oamcomm.ApplicationRunningWorkflow))
		}, 30*time.Second).Should(Succeed())

		By("Update Application to second failed version")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(hubCtx, appKey, app)).Should(Succeed())
			app.Annotations[oam.AnnotationPublishVersion] = "alpha3"
			app.Spec.Components[0].Name = "busybox-bad"
			g.Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
		}, 30*time.Second).Should(Succeed())
		Eventually(func(g Gomega) {
			deploy := &v1.Deployment{}
			g.Expect(k8sClient.Get(workerCtx, types.NamespacedName{Namespace: namespace, Name: "busybox"}, deploy)).Should(Succeed())
			g.Expect(k8sClient.Delete(workerCtx, deploy)).Should(Succeed())
		}, 30*time.Second).Should(Succeed())

		By("Change external policy")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "busybox-v3"}, &v1beta1.ApplicationRevision{})).Should(Succeed())
			policy := &v1alpha1.Policy{}
			g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "topology-worker"}, policy)).Should(Succeed())
			policy.Properties = &runtime.RawExtension{Raw: []byte(`{"clusters":["changed"]}`)}
			g.Expect(k8sClient.Update(hubCtx, policy)).Should(Succeed())
		}, 30*time.Second).Should(Succeed())

		By("Change referred objects")
		Eventually(func(g Gomega) {
			deploy := &v1.Deployment{}
			g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "busybox-ref"}, deploy)).Should(Succeed())
			deploy.Spec.Replicas = pointer.Int32(1)
			g.Expect(k8sClient.Update(hubCtx, deploy)).Should(Succeed())
		}, 30*time.Second).Should(Succeed())

		By("Live-diff application")
		outputs, err := execCommand("live-diff", "-r", "busybox-v3,busybox-v1", "-n", namespace)
		Expect(err).Should(Succeed())
		Expect(outputs).Should(SatisfyAll(
			ContainSubstring("Application (busybox) has been modified(*)"),
			ContainSubstring("External Policy (topology-worker) has no change"),
			ContainSubstring("External Workflow (deploy-worker) has no change"),
			ContainSubstring(fmt.Sprintf("Referred Object (apps/v1 Deployment %s/busybox-ref) has no change", namespace)),
		))
		outputs, err = execCommand("live-diff", "busybox", "-n", namespace)
		Expect(err).Should(Succeed())
		Expect(outputs).Should(SatisfyAll(
			ContainSubstring("Application (busybox) has no change"),
			ContainSubstring("External Policy (topology-worker) has been modified(*)"),
			ContainSubstring("External Workflow (deploy-worker) has no change"),
			ContainSubstring(fmt.Sprintf("Referred Object (apps/v1 Deployment %s/busybox-ref) has been modified", namespace)),
		))

		By("Rollback application")
		Eventually(func(g Gomega) {
			_, err = execCommand("workflow", "suspend", "busybox", "-n", namespace)
			g.Expect(err).Should(Succeed())
		}).WithTimeout(10 * time.Second).Should(Succeed())
		Eventually(func(g Gomega) {
			_, err = execCommand("workflow", "rollback", "busybox", "-n", namespace)
			g.Expect(err).Should(Succeed())
		}).WithTimeout(10 * time.Second).Should(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(hubCtx, appKey, app)).Should(Succeed())
			g.Expect(app.Status.Phase).Should(Equal(oamcomm.ApplicationRunning))
			deploy := &v1.Deployment{}
			g.Expect(k8sClient.Get(workerCtx, types.NamespacedName{Namespace: namespace, Name: "busybox"}, deploy)).Should(Succeed())
			g.Expect(deploy.Spec.Template.Spec.Containers[0].Image).Should(Equal("busybox"))
			g.Expect(k8sClient.Get(workerCtx, types.NamespacedName{Namespace: namespace, Name: "busybox-ref"}, deploy)).Should(Succeed())
			g.Expect(deploy.Spec.Replicas).Should(Equal(pointer.Int32(0)))
			revs, err := application.GetSortedAppRevisions(hubCtx, k8sClient, app.Name, namespace)
			g.Expect(err).Should(Succeed())
			g.Expect(len(revs)).Should(Equal(1))
		}).WithTimeout(time.Minute).WithPolling(2 * time.Second).Should(Succeed())
	})

	It("Test large application parallel apply and delete", func() {
		newApp := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "large-app"}}
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
			Steps: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "deploy",
					Type:       "deploy",
					Properties: &runtime.RawExtension{Raw: []byte(`{"policies":["topology-deploy"],"parallelism":10}`)},
				},
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

	It("Test ref-objects with url", func() {
		newApp := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "app"},
			Spec: v1beta1.ApplicationSpec{
				Components: []oamcomm.ApplicationComponent{{
					Name:       "example",
					Type:       "ref-objects",
					Properties: &runtime.RawExtension{Raw: []byte(`{"urls":["https://gist.githubusercontent.com/Somefive/b189219a9222eaa70b8908cf4379402b/raw/920e83b1a2d56b584f9d8c7a97810a505a0bbaad/example-busybox-resources.yaml"]}`)},
				}},
			},
		}

		By("Create application")
		Expect(k8sClient.Create(hubCtx, newApp)).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(newApp), newApp)).Should(Succeed())
			g.Expect(newApp.Status.Phase).Should(Equal(oamcomm.ApplicationRunning))
		}, 15*time.Second).Should(Succeed())
		key := types.NamespacedName{Namespace: namespace, Name: "busybox"}
		Expect(k8sClient.Get(hubCtx, key, &v1.Deployment{})).Should(Succeed())
		Expect(k8sClient.Get(hubCtx, key, &corev1.ConfigMap{})).Should(Succeed())

		By("Delete application")
		Expect(k8sClient.Delete(hubCtx, newApp)).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(newApp), newApp)).Should(Satisfy(errors.IsNotFound))
		}, 15*time.Second).Should(Succeed())
		Expect(k8sClient.Get(hubCtx, key, &v1.Deployment{})).Should(Satisfy(errors.IsNotFound))
		Expect(k8sClient.Get(hubCtx, key, &corev1.ConfigMap{})).Should(Satisfy(errors.IsNotFound))
	})
})
