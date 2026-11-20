/*
Copyright 2026 The KubeVela Authors.

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
	"strings"
	"time"

	"github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/pkg/util/rand"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/release"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// podinfoWithRelease is podinfo with only the release fields given set.
func podinfoWithRelease(name string, release map[string]interface{}) common.ApplicationComponent {
	props := map[string]interface{}{
		"chart": map[string]interface{}{
			"source":  "podinfo",
			"repoURL": "https://stefanprodan.github.io/podinfo",
			"version": "6.11.1",
		},
	}
	if release != nil {
		props["release"] = release
	}
	raw, err := json.Marshal(props)
	Expect(err).Should(Succeed())
	return common.ApplicationComponent{Name: name, Type: "helmchart", Properties: &runtime.RawExtension{Raw: raw}}
}

func topologyInNamespace(namespace string, clusters ...string) v1beta1.AppPolicy {
	raw, err := json.Marshal(v1alpha1.TopologyPolicySpec{Placement: v1alpha1.Placement{Clusters: clusters}, Namespace: namespace})
	Expect(err).Should(Succeed())
	return v1beta1.AppPolicy{Name: "topology", Type: v1alpha1.TopologyPolicyType, Properties: &runtime.RawExtension{Raw: raw}}
}

var _ = Describe("Test helmchart release ownership", func() {

	const helmTimeout = 3 * time.Minute

	var namespace string
	var hubCtx context.Context
	var workerCtx context.Context

	BeforeEach(func() {
		hubCtx, workerCtx, namespace = initializeContextAndNamespace()
	})

	AfterEach(func() {
		cleanUpNamespace(hubCtx, workerCtx, namespace)
	})

	app := func(name string, comp common.ApplicationComponent, policies ...v1beta1.AppPolicy) *v1beta1.Application {
		return &v1beta1.Application{
			TypeMeta:   metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.ApplicationKind},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{comp}, Policies: policies},
		}
	}

	waitPhase := func(key client.ObjectKey, phases ...common.ApplicationPhase) *v1beta1.Application {
		app := &v1beta1.Application{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(hubCtx, key, app)).Should(Succeed())
			g.Expect(phases).Should(ContainElement(app.Status.Phase), "workflow: %s", workflowMessages(app))
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())
		return app
	}

	deleteApp := func(a *v1beta1.Application) {
		Expect(k8sClient.Delete(hubCtx, a)).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(kerrors.IsNotFound(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(a), &v1beta1.Application{}))).Should(BeTrue())
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())
	}

	It("refuses a release another Application owns, and leaves that release alone", func() {
		shop := app("shop", podinfoWithRelease("redis", nil), topologyPolicy(multicluster.Local))
		Expect(k8sClient.Create(hubCtx, shop)).Should(Succeed())
		waitPhase(client.ObjectKeyFromObject(shop), common.ApplicationRunning)

		blog := app("blog", podinfoWithRelease("redis", nil), topologyPolicy(multicluster.Local))
		Expect(k8sClient.Create(hubCtx, blog)).Should(Succeed())
		Eventually(func(g Gomega) {
			got := &v1beta1.Application{}
			g.Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(blog), got)).Should(Succeed())
			g.Expect(workflowMessages(got)).Should(ContainSubstring(fmt.Sprintf("belongs to Application %s/shop", namespace)))
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())

		rel, ok := helmReleases(Default, multicluster.Local, namespace)["redis"]
		Expect(ok).Should(BeTrue())
		Expect(rel.Version).Should(Equal(1))
		Expect(rel.Labels).Should(HaveKeyWithValue("app.oam.dev/name", "shop"))

		deleteApp(blog)
		deleteApp(shop)
	})

	It("installs into a topology namespace and keeps the Application as the release's owner", func() {
		target := "test-mc-target-" + rand.RandomString(4)
		Expect(k8sClient.Create(hubCtx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: target}})).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(hubCtx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: target}})).Should(Succeed())
		}()

		a := app("placed", podinfoWithRelease("podinfo", nil), topologyInNamespace(target, multicluster.Local))
		Expect(k8sClient.Create(hubCtx, a)).Should(Succeed())
		waitPhase(client.ObjectKeyFromObject(a), common.ApplicationRunning)

		rel, ok := helmReleases(Default, multicluster.Local, target)["podinfo"]
		Expect(ok).Should(BeTrue())
		Expect(rel.Info.Status).Should(Equal(release.StatusDeployed))
		Expect(rel.Labels).Should(HaveKeyWithValue("app.oam.dev/namespace", namespace))

		deleteApp(a)
		Eventually(func(g Gomega) {
			g.Expect(helmReleases(g, multicluster.Local, target)).Should(BeEmpty())
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())
	})

	It("names a release with only a namespace after its component", func() {
		a := app("ns-only", podinfoWithRelease("podinfo", map[string]interface{}{"namespace": namespace}), topologyPolicy(multicluster.Local))
		Expect(k8sClient.Create(hubCtx, a)).Should(Succeed())
		waitPhase(client.ObjectKeyFromObject(a), common.ApplicationRunning)

		releases := helmReleases(Default, multicluster.Local, namespace)
		Expect(releases).Should(HaveKey("podinfo"))
		Expect(releases).ShouldNot(HaveKey("release"))
		deleteApp(a)
	})

	It("takes over what 1.11 left on both clusters when the component is placed on the worker", func() {
		By("a hub release labelled for the component, which no ResourceTracker holds")
		first := app("leftover", podinfoWithRelease("podinfo", nil), topologyPolicy(multicluster.Local))
		Expect(k8sClient.Create(hubCtx, first)).Should(Succeed())
		waitPhase(client.ObjectKeyFromObject(first), common.ApplicationRunning)
		// Captured before the orphaning delete below, which strips the ownership labels.
		hubDeploy := &appsv1.Deployment{}
		Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "podinfo"}, hubDeploy)).Should(Succeed())
		hubService := &corev1.Service{}
		Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "podinfo"}, hubService)).Should(Succeed())
		secrets := &corev1.SecretList{}
		Expect(k8sClient.List(hubCtx, secrets, client.InNamespace(namespace), client.MatchingLabels{"owner": "helm", "name": "podinfo"})).Should(Succeed())
		Expect(secrets.Items).ShouldNot(BeEmpty())

		Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(first), first)).Should(Succeed())
		controllerutil.AddFinalizer(first, oam.FinalizerOrphanResource)
		Expect(k8sClient.Update(hubCtx, first)).Should(Succeed())
		deleteApp(first)
		for _, s := range secrets.Items {
			labelled := &corev1.Secret{}
			Expect(k8sClient.Get(hubCtx, client.ObjectKeyFromObject(&s), labelled)).Should(Succeed())
			for _, k := range []string{oam.LabelAppName, oam.LabelAppNamespace, oam.LabelAppComponent} {
				labelled.Labels[k] = s.Labels[k]
			}
			Expect(k8sClient.Update(hubCtx, labelled)).Should(Succeed())
		}
		Expect(helmReleases(Default, multicluster.Local, namespace)).Should(HaveKey("podinfo"))

		By("the chart's objects on the worker without Helm's ownership metadata, and the release Secret copied there")
		strip := func(obj client.Object) {
			labels, annotations := obj.GetLabels(), obj.GetAnnotations()
			delete(labels, "app.kubernetes.io/managed-by")
			for k := range annotations {
				if strings.HasPrefix(k, "meta.helm.sh/") {
					delete(annotations, k)
				}
			}
			obj.SetLabels(labels)
			obj.SetAnnotations(annotations)
			obj.SetResourceVersion("")
			obj.SetUID("")
			obj.SetOwnerReferences(nil)
			obj.SetManagedFields(nil)
		}
		strip(hubDeploy)
		hubDeploy.Status = appsv1.DeploymentStatus{}
		Expect(k8sClient.Create(workerCtx, hubDeploy)).Should(Succeed())
		strip(hubService)
		hubService.Spec.ClusterIP, hubService.Spec.ClusterIPs = "", nil
		hubService.Status = corev1.ServiceStatus{}
		Expect(k8sClient.Create(workerCtx, hubService)).Should(Succeed())
		for _, s := range secrets.Items {
			placeholder := s.DeepCopy()
			delete(placeholder.Labels, "owner")
			strip(placeholder)
			Expect(k8sClient.Create(workerCtx, placeholder)).Should(Succeed())
		}

		By("the Application, now placed on the worker, installs there and removes the hub copy")
		moved := app("leftover", podinfoWithRelease("podinfo", nil), topologyPolicy(WorkerClusterName))
		Expect(k8sClient.Create(hubCtx, moved)).Should(Succeed())
		waitPhase(client.ObjectKeyFromObject(moved), common.ApplicationRunning)

		Eventually(func(g Gomega) {
			rel, ok := helmReleases(g, WorkerClusterName, namespace)["podinfo"]
			g.Expect(ok).Should(BeTrue())
			g.Expect(rel.Info.Status).Should(Equal(release.StatusDeployed))
			g.Expect(helmReleases(g, multicluster.Local, namespace)).Should(BeEmpty())
			g.Expect(kerrors.IsNotFound(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "podinfo"}, &appsv1.Deployment{}))).Should(BeTrue())
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())

		deleteApp(moved)
		Eventually(func(g Gomega) {
			g.Expect(helmReleases(g, WorkerClusterName, namespace)).Should(BeEmpty())
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())
	})

	It("keeps the hub release of a component placed on the hub and the worker", func() {
		a := app("both", podinfoWithRelease("podinfo", nil), topologyPolicy(multicluster.Local, WorkerClusterName))
		Expect(k8sClient.Create(hubCtx, a)).Should(Succeed())
		waitPhase(client.ObjectKeyFromObject(a), common.ApplicationRunning)

		Consistently(func(g Gomega) {
			g.Expect(helmReleases(g, multicluster.Local, namespace)).Should(HaveKey("podinfo"))
			g.Expect(helmReleases(g, WorkerClusterName, namespace)).Should(HaveKey("podinfo"))
		}).WithPolling(2 * time.Second).WithTimeout(15 * time.Second).Should(Succeed())
		deleteApp(a)
	})
})
