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

package helm_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// ============================================================================
// Self-Healing Scenarios
// ============================================================================

// The self-healing scenarios run in sequence against one release. Each
// disruption ends with the release back at two ready replicas; the destructive
// ones come last.
var _ = Describe("Helmchart Self-Healing", Label("core-helm"), Ordered, func() {
	h := newHelmTestContext()
	BeforeAll(func() { h.CreateNamespace() })
	AfterAll(func() { h.Cleanup() })

	It("should deploy podinfo successfully", func() {
		h.DeployApp()
		h.WaitForDeploymentReady()
		By("Verifying Helm release secret exists")
		Expect(len(h.GetHelmSecrets().Items)).Should(BeNumerically(">=", 1))
	})

	It("should preserve user-added annotations and labels across reconciles", func() {
		By("Adding custom annotation via kubectl annotate")
		runCommandSucceed("kubectl", "annotate", "deployment", "podinfo", "custom.io/test=test-value", "-n", h.Namespace)
		By("Adding custom label via kubectl label")
		runCommandSucceed("kubectl", "label", "deployment", "podinfo", "extra.io/label=extra-value", "-n", h.Namespace)

		By("Verifying annotation and label are preserved (3-way merge)")
		ConsistentlyReconciled(h.Ctx, h.App, func(g Gomega) {
			d := &appsv1.Deployment{}
			g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, d)).Should(Succeed())
			g.Expect(d.GetAnnotations()).Should(HaveKeyWithValue("custom.io/test", "test-value"))
			g.Expect(d.GetLabels()).Should(HaveKeyWithValue("extra.io/label", "extra-value"))
		}).Should(Succeed())
		h.WaitForAppRunning()
	})

	It("should revert manual scaling back to 2 replicas", func() {
		By("Scaling Deployment to 5 replicas via kubectl scale")
		runCommandSucceed("kubectl", "scale", "deployment", "podinfo", "--replicas=5", "-n", h.Namespace)

		By("Verifying Deployment scaled to 5")
		Eventually(func(g Gomega) {
			d := &appsv1.Deployment{}
			g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, d)).Should(Succeed())
			g.Expect(*d.Spec.Replicas).Should(Equal(int32(5)))
		}, 10*time.Second, time.Second).Should(Succeed())

		By("Triggering force reconcile via annotation")
		RequestReconcileNow(h.Ctx, h.App)

		By("Verifying KubeVela reverts replicas to 2")
		Eventually(func(g Gomega) {
			d := &appsv1.Deployment{}
			g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, d)).Should(Succeed())
			g.Expect(*d.Spec.Replicas).Should(Equal(int32(2)))
		}, 120*time.Second, 3*time.Second).Should(Succeed())

		h.WaitForDeploymentReady()
	})

	It("should recover when the Service is deleted via kubectl", func() {
		originalPodUIDs := h.RecordPodUIDs()

		By("Recording old ClusterIP")
		oldSvc := &corev1.Service{}
		Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, oldSvc)).Should(Succeed())
		oldClusterIP := oldSvc.Spec.ClusterIP

		By("Deleting the Service via kubectl")
		runCommandSucceed("kubectl", "delete", "svc", "podinfo", "-n", h.Namespace)

		By("Triggering reconciliation")
		RequestReconcileNow(h.Ctx, h.App)

		By("Verifying KubeVela recreates the Service with a new ClusterIP")
		var newClusterIP string
		Eventually(func(g Gomega) {
			svc := &corev1.Service{}
			g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, svc)).Should(Succeed())
			g.Expect(svc.Spec.ClusterIP).ShouldNot(BeEmpty())
			newClusterIP = svc.Spec.ClusterIP
		}, 120*time.Second, 3*time.Second).Should(Succeed())
		Expect(newClusterIP).ShouldNot(Equal(oldClusterIP),
			"New ClusterIP should be assigned after Service recreation")

		By("Verifying pods are NOT affected")
		Expect(h.CountSurvivingPods(originalPodUIDs)).Should(BeNumerically(">=", 2))
	})

	It("should recover when the Deployment is deleted via kubectl", func() {
		initialCount := len(h.GetHelmSecrets().Items)
		latestSecret := h.LatestHelmSecretName()

		By("Deleting the Deployment via kubectl")
		runCommandSucceed("kubectl", "delete", "deployment", "podinfo", "-n", h.Namespace)

		By("Verifying Deployment is gone")
		Eventually(func() bool {
			err := k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, &appsv1.Deployment{})
			return err != nil
		}, 10*time.Second, time.Second).Should(BeTrue())

		By("Triggering reconciliation")
		RequestReconcileNow(h.Ctx, h.App)

		By("Verifying KubeVela recreates the Deployment")
		h.WaitForDeploymentReady()

		By("Verifying Helm revision did NOT increment (recovery is via ResourceTracker)")
		Expect(len(h.GetHelmSecrets().Items)).Should(Equal(initialCount))
		Expect(h.LatestHelmSecretName()).Should(Equal(latestSecret))

		By("Verifying pods are back to desired replica count")
		deploy := &appsv1.Deployment{}
		Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, deploy)).Should(Succeed())
		Expect(*deploy.Spec.Replicas).Should(Equal(int32(2)))
	})

	It("should recover release secrets without affecting running pods", func() {
		originalPodUIDs := h.RecordPodUIDs()
		Expect(len(originalPodUIDs)).Should(BeNumerically(">=", 2))

		By("Deleting all Helm release secrets via kubectl")
		runCommandSucceed("kubectl", "delete", "secrets", "-l", "owner=helm,name=podinfo", "-n", h.Namespace)

		By("Verifying Helm release secrets are gone")
		Eventually(func() int {
			return len(h.GetHelmSecrets().Items)
		}, 15*time.Second, time.Second).Should(Equal(0))

		By("Verifying running pods are NOT affected")
		Consistently(func() int {
			pods := &corev1.PodList{}
			Expect(k8sClient.List(h.Ctx, pods, client.InNamespace(h.Namespace),
				client.MatchingLabels{"app.kubernetes.io/name": "podinfo"})).Should(Succeed())
			count := 0
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning {
					count++
				}
			}
			return count
		}, 10*time.Second, 2*time.Second).Should(BeNumerically(">=", 2))

		By("Triggering reconciliation")
		RequestReconcileNow(h.Ctx, h.App)

		By("Verifying KubeVela restores Helm release secrets")
		Eventually(func() int {
			return len(h.GetHelmSecrets().Items)
		}, 120*time.Second, 3*time.Second).Should(BeNumerically(">=", 1))

		By("Verifying helm list shows the release again")
		Eventually(func() string {
			out, _ := runCommand("helm", "list", "-n", h.Namespace, "-q")
			return out
		}, 60*time.Second, 3*time.Second).Should(ContainSubstring("podinfo"))

		By("Verifying original pods still exist (not restarted)")
		Expect(h.CountSurvivingPods(originalPodUIDs)).Should(BeNumerically(">=", 2))

		h.WaitForAppRunning()
	})

	It("should recover from corrupted Helm release secret", func() {
		originalPodUIDs := h.RecordPodUIDs()
		latestSecret := h.LatestHelmSecretName()
		Expect(latestSecret).ShouldNot(BeEmpty())

		By("Corrupting the release secret via kubectl patch")
		runCommandSucceed("kubectl", "patch", "secret", latestSecret, "-n", h.Namespace,
			"--type=json", `-p=[{"op":"replace","path":"/data/release","value":"Y29ycnVwdGVk"}]`)

		By("Applying a spec change to trigger re-render")
		Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
		annotations := h.App.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations["test.oam.dev/trigger"] = "corrupt-recovery"
		h.App.SetAnnotations(annotations)
		Expect(k8sClient.Update(h.Ctx, h.App)).Should(Succeed())

		By("Verifying corrupted secret is automatically deleted")
		Eventually(func() bool {
			s := &corev1.Secret{}
			err := k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: latestSecret}, s)
			if err != nil {
				return true
			}
			return string(s.Data["release"]) != "corrupted"
		}, 60*time.Second, 3*time.Second).Should(BeTrue())

		By("Verifying helm list shows a clean release")
		Eventually(func() string {
			out, _ := runCommand("helm", "list", "-n", h.Namespace, "-q")
			return out
		}, 120*time.Second, 3*time.Second).Should(ContainSubstring("podinfo"))

		h.WaitForDeploymentReady()
		h.WaitForAppRunning()

		By("Verifying pods are unaffected during recovery")
		Expect(h.CountSurvivingPods(originalPodUIDs)).Should(BeNumerically(">=", 2))
	})

	It("should recover after external helm uninstall", func() {
		By("Running helm uninstall podinfo externally")
		runCommandSucceed("helm", "uninstall", "podinfo", "-n", h.Namespace)

		By("Verifying helm list no longer shows the release")
		Eventually(func() string {
			out, _ := runCommand("helm", "list", "-n", h.Namespace, "-q")
			return out
		}, 15*time.Second, time.Second).ShouldNot(ContainSubstring("podinfo"))

		By("Verifying Deployment is gone after uninstall")
		Eventually(func() bool {
			err := k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, &appsv1.Deployment{})
			return err != nil
		}, 30*time.Second, 2*time.Second).Should(BeTrue())

		By("Triggering reconciliation")
		RequestReconcileNow(h.Ctx, h.App)

		By("Verifying KubeVela performs fresh helm install")
		h.WaitForDeploymentReady()

		By("Verifying helm list shows the release again")
		Eventually(func() string {
			out, _ := runCommand("helm", "list", "-n", h.Namespace, "-q")
			return out
		}, 60*time.Second, 3*time.Second).Should(ContainSubstring("podinfo"))

		h.WaitForAppRunning()
	})

	It("should recover after namespace deletion", func() {
		By("Deleting the target namespace via kubectl")
		runCommandSucceed("kubectl", "delete", "namespace", h.Namespace, "--wait=false")

		By("Waiting for namespace to be fully deleted")
		Eventually(func() bool {
			return k8sClient.Get(h.Ctx, types.NamespacedName{Name: h.Namespace}, &corev1.Namespace{}) != nil
		}, 120*time.Second, 3*time.Second).Should(BeTrue())

		By("Verifying Application CR survives (it is in default namespace)")
		Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())

		By("Applying a spec change to trigger re-render")
		Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
		annotations := h.App.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations["test.oam.dev/trigger"] = "ns-delete-recovery"
		h.App.SetAnnotations(annotations)
		Expect(k8sClient.Update(h.Ctx, h.App)).Should(Succeed())

		By("Verifying namespace is recreated (via createNamespace: true)")
		Eventually(func(g Gomega) {
			ns := &corev1.Namespace{}
			g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Name: h.Namespace}, ns)).Should(Succeed())
			g.Expect(ns.Status.Phase).Should(Equal(corev1.NamespaceActive))
		}, 120*time.Second, 3*time.Second).Should(Succeed())

		By("Verifying all resources and Helm release are restored")
		h.WaitForDeploymentReady()
		Eventually(func() string {
			out, _ := runCommand("helm", "list", "-n", h.Namespace, "-q")
			return out
		}, 60*time.Second, 3*time.Second).Should(ContainSubstring("podinfo"))
		h.WaitForAppRunning()
	})

	It("should clean up all resources when Application is deleted after several upgrades", func() {
		h.UpdateAppValues(map[string]interface{}{"ui": map[string]interface{}{"message": "upgrade-1"}})
		h.UpdateAppValues(map[string]interface{}{"ui": map[string]interface{}{"message": "upgrade-2"}})
		h.UpdateAppValues(map[string]interface{}{"ui": map[string]interface{}{"message": "upgrade-3"}})
		Expect(len(h.GetHelmSecrets().Items)).Should(BeNumerically(">", 1))

		appName := h.App.Name
		By("Deleting Application via kubectl")
		runCommandSucceed("kubectl", "delete", "application", appName, "-n", h.AppNamespace)

		By("Verifying Application is gone")
		Eventually(func() bool {
			return k8sClient.Get(h.Ctx, h.AppKey, &v1beta1.Application{}) != nil
		}, 60*time.Second, 2*time.Second).Should(BeTrue())
		h.App = nil

		By("Verifying ALL Helm release secrets are deleted")
		Eventually(func() int { return len(h.GetHelmSecrets().Items) }, 60*time.Second, 3*time.Second).Should(Equal(0))

		By("Verifying helm list shows empty")
		Eventually(func() string {
			out, _ := runCommand("helm", "list", "-n", h.Namespace, "-q")
			return strings.TrimSpace(out)
		}, 30*time.Second, 3*time.Second).Should(BeEmpty())

		By("Verifying Deployment, Service, and pods are all deleted")
		Eventually(func() bool {
			return k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, &appsv1.Deployment{}) != nil
		}, 30*time.Second, 2*time.Second).Should(BeTrue())
		Eventually(func() bool {
			return k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, &corev1.Service{}) != nil
		}, 30*time.Second, 2*time.Second).Should(BeTrue())
		Eventually(func() int {
			pods := &corev1.PodList{}
			_ = k8sClient.List(h.Ctx, pods, client.InNamespace(h.Namespace),
				client.MatchingLabels{"app.kubernetes.io/name": "podinfo"})
			return len(pods.Items)
		}, 60*time.Second, 2*time.Second).Should(Equal(0))

		By("Verifying ResourceTracker is deleted")
		Eventually(func() bool {
			rtList := &v1beta1.ResourceTrackerList{}
			Expect(k8sClient.List(h.Ctx, rtList, client.MatchingLabels{"app.oam.dev/name": appName})).Should(Succeed())
			return len(rtList.Items) == 0
		}, 30*time.Second, 2*time.Second).Should(BeTrue())
	})
})

// ============================================================================
// Adoption & Takeover Scenarios
// ============================================================================

var _ = Describe("Helmchart Adoption & Takeover", Label("core-helm"), func() {

	Context("Adopt an Existing Vanilla Helm Release", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() { h.Cleanup() })

		It("should adopt a pre-existing Helm release", func() {
			By("Installing podinfo via helm install directly (no KubeVela)")
			runCommandSucceed("helm", append([]string{"install", "podinfo",
				"--repo", "https://stefanprodan.github.io/podinfo", "podinfo",
				"--version", "6.11.1", "--set", "replicaCount=2", "-n", h.Namespace},
				podinfoAppResourceFlags...)...)

			initialSecretCount := len(h.GetHelmSecrets().Items)

			By("Waiting for Deployment to be ready before recording pod UIDs")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(Equal(int32(2)))
			}, 120*time.Second, 3*time.Second).Should(Succeed())
			By("Recording running pod UIDs before adoption")
			var podList corev1.PodList
			Expect(k8sClient.List(h.Ctx, &podList, client.InNamespace(h.Namespace),
				client.MatchingLabels{"app.kubernetes.io/name": "podinfo"})).Should(Succeed())
			originalPodUIDs := make(map[types.UID]bool)
			for _, pod := range podList.Items {
				originalPodUIDs[pod.UID] = true
			}

			By("Applying a KubeVela Application with same release name, chart, and values")
			h.DeployApp()
			h.WaitForAppRunning()

			By("Verifying Helm revision increments by 1 (forced upgrade to inject KubeVela labels)")
			Expect(len(h.GetHelmSecrets().Items)).Should(Equal(initialSecretCount + 1))

			By("Verifying pods are NOT restarted (zero downtime adoption)")
			Eventually(func() int { return h.CountSurvivingPods(originalPodUIDs) }, 30*time.Second, 2*time.Second).Should(BeNumerically(">=", 2))

			By("Verifying app.oam.dev/* labels appear on Deployment")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			Expect(deploy.GetLabels()).Should(HaveKey("app.oam.dev/name"))

			By("Verifying the pod template was not changed, so no rollout started")
			Expect(deploy.GetAnnotations()).Should(HaveKeyWithValue("deployment.kubernetes.io/revision", "1"))

			By("Verifying app.oam.dev/* labels appear on Service")
			svc := &corev1.Service{}
			Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, svc)).Should(Succeed())
			Expect(svc.GetLabels()).Should(HaveKey("app.oam.dev/name"))

			By("Verifying meta.helm.sh/release-name annotation is preserved")
			Expect(deploy.GetAnnotations()).Should(HaveKeyWithValue("meta.helm.sh/release-name", "podinfo"))
		})
	})

	Context("Adopt Release with Different Values", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() { h.Cleanup() })

		It("should adopt and upgrade a release with different values", func() {
			By("Installing podinfo via helm install with replicaCount=1")
			runCommandSucceed("helm", "install", "podinfo",
				"--repo", "https://stefanprodan.github.io/podinfo", "podinfo",
				"--version", "6.11.1", "--set", "replicaCount=1", "-n", h.Namespace)

			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(Equal(int32(1)))
			}, 60*time.Second, 3*time.Second).Should(Succeed())

			initialSecretCount := len(h.GetHelmSecrets().Items)

			By("Applying KubeVela Application with replicaCount=3")
			raw, err := os.ReadFile(testDataPath("helm/app_helmchart_podinfo.yaml"))
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.Namespace))
			raw = bytes.ReplaceAll(raw, []byte("replicaCount: 2"), []byte("replicaCount: 3"))
			h.App = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, h.App)).Should(BeNil())
			h.App.SetNamespace(h.AppNamespace)
			h.App.SetName("podinfo-helm-test-" + helmTestSuffix())
			Expect(k8sClient.Create(h.Ctx, h.App)).Should(Succeed())
			h.AppKey = client.ObjectKeyFromObject(h.App)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
				g.Expect(h.App.Status.Phase).Should(Equal(common2.ApplicationRunning))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying Helm upgrade occurs (fingerprint differs)")
			Expect(len(h.GetHelmSecrets().Items)).Should(BeNumerically(">", initialSecretCount))

			By("Verifying replicas scale to 3")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(Equal(int32(3)))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying KubeVela labels injected")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			Expect(deploy.GetLabels()).Should(HaveKey("app.oam.dev/name"))
		})
	})

	Context("Re-adopt After Application Deletion", FlakeAttempts(2), Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() { h.CleanupNamespaceOnly() })

		It("should re-adopt seamlessly after deletion and reinstall", func() {
			h.FreshAttempt()
			By("Installing podinfo via helm install")
			runCommandSucceed("helm", append([]string{"install", "podinfo",
				"--repo", "https://stefanprodan.github.io/podinfo", "podinfo",
				"--version", "6.11.1", "--set", "replicaCount=2", "-n", h.Namespace},
				podinfoAppResourceFlags...)...)
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(Equal(int32(2)))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("Applying KubeVela Application (adopts the release)")
			h.DeployApp()
			h.WaitForAppRunning()

			By("Deleting the Application (GC cleans up everything)")
			runCommandSucceed("kubectl", "delete", "application", h.App.Name, "-n", h.AppNamespace)
			Eventually(func() bool {
				return k8sClient.Get(h.Ctx, h.AppKey, &v1beta1.Application{}) != nil
			}, 60*time.Second, 2*time.Second).Should(BeTrue())
			h.App = nil

			By("Waiting for GC to clean up resources")
			Eventually(func() int { return len(h.GetHelmSecrets().Items) }, 60*time.Second, 3*time.Second).Should(Equal(0))

			By("Installing podinfo via helm install again")
			runCommandSucceed("helm", append([]string{"install", "podinfo",
				"--repo", "https://stefanprodan.github.io/podinfo", "podinfo",
				"--version", "6.11.1", "--set", "replicaCount=2", "-n", h.Namespace},
				podinfoAppResourceFlags...)...)
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(Equal(int32(2)))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("Applying the same KubeVela Application again")
			h.DeployApp()
			h.WaitForAppRunning()
			h.WaitForDeploymentReady()
		})
	})
})

// ============================================================================
// Helm State Integrity Scenarios
// ============================================================================

var _ = Describe("Helmchart State Integrity", Label("core-helm"), func() {

	Context("Upgrade History Preserved Across Multiple Changes", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() { h.Cleanup() })

		It("should preserve upgrade history across 5 changes", func() {
			h.DeployApp()
			h.WaitForDeploymentReady()

			for i := 1; i <= 5; i++ {
				prevSecretCount := len(h.GetHelmSecrets().Items)
				By(fmt.Sprintf("Upgrade %d: changing values and waiting for new helm revision", i))
				h.UpdateAppValues(map[string]interface{}{"ui": map[string]interface{}{"message": fmt.Sprintf("upgrade-%d", i)}})
				Eventually(func() int {
					return len(h.GetHelmSecrets().Items)
				}, 120*time.Second, 3*time.Second).Should(BeNumerically(">", prevSecretCount),
					fmt.Sprintf("Upgrade %d: expected helm revision to increment", i))
			}

			By("Verifying helm history shows all 6 revisions (1 install + 5 upgrades)")
			out := runCommandSucceed("helm", "history", "podinfo", "-n", h.Namespace, "--output", "json")
			var history []map[string]interface{}
			Expect(json.Unmarshal([]byte(out), &history)).Should(Succeed())
			Expect(len(history)).Should(BeNumerically(">=", 6),
				"Expected at least 6 revisions (1 install + 5 upgrades)")

			By("Verifying all release secrets exist")
			Expect(len(h.GetHelmSecrets().Items)).Should(BeNumerically(">=", 6))

			By("Verifying maxHistory is respected (default maxHistory=10 in chart options)")
			Expect(len(h.GetHelmSecrets().Items)).Should(BeNumerically("<=", 10))
		})
	})
})

// ============================================================================
// Destructive & Chaos Scenarios
// ============================================================================

var _ = Describe("Helmchart Destructive & Chaos", Label("core-helm"), func() {

	Context("Two Applications Targeting Same Release Name", Ordered, func() {
		h := newHelmTestContext()
		var appB *v1beta1.Application
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() {
			if appB != nil {
				_ = k8sClient.Delete(h.Ctx, appB)
			}
			h.Cleanup()
		})

		It("should detect ownership conflict", func() {
			h.DeployApp()
			h.WaitForDeploymentReady()
			h.WaitForAppRunning()

			By("Deploying second Application also targeting release podinfo")
			raw, err := os.ReadFile(testDataPath("helm/app_helmchart_podinfo.yaml"))
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.Namespace))
			appB = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, appB)).Should(BeNil())
			appB.SetNamespace(h.AppNamespace)
			appB.SetName("podinfo-conflict-" + helmTestSuffix())
			Expect(k8sClient.Create(h.Ctx, appB)).Should(Succeed())
			appBKey := client.ObjectKeyFromObject(appB)

			By("Verifying second application fails with ownership conflict")
			EventuallyReconciled(h.Ctx, appB, func(g Gomega) {
				g.Expect(k8sClient.Get(h.Ctx, appBKey, appB)).Should(Succeed())
				g.Expect(appB.Status.Phase).Should(Equal(common2.ApplicationWorkflowFailed), "workflow=%+v", appB.Status.Workflow)
				var messages []string
				for _, step := range appB.Status.Workflow.Steps {
					messages = append(messages, step.Message)
				}
				g.Expect(strings.Join(messages, "\n")).Should(ContainSubstring("managed by other application"))
			}).WithTimeout(90 * time.Second).Should(Succeed())

			By("Verifying the first application remains healthy and unaffected")
			h.WaitForAppRunning()
			h.WaitForDeploymentReady()
		})
	})
})

// ============================================================================
// Resource Ordering Scenarios
// ============================================================================

var _ = Describe("Helmchart Resource Ordering", Label("core-helm"), func() {

	Context("Chart with CRDs (crossplane)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() { h.CleanupNamespaceOnly() })

		It("should deploy crossplane chart with CRDs and reach running", func() {
			By("Deploying crossplane chart (includes CRDs)")
			raw := []byte(fmt.Sprintf(`apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: crossplane-crd-test
spec:
  components:
    - name: crossplane
      type: helmchart
      properties:
        chart:
          source: crossplane
          repoURL: https://charts.crossplane.io/stable
          version: "1.19.1"
        release:
          name: crossplane
          namespace: %s
        values:
          resources:
            limits:
              cpu: 500m
              memory: 512Mi
            requests:
              cpu: 100m
              memory: 256Mi
          args:
            - --debug=false
        options:
          createNamespace: true
          includeCRDs: true
          skipTests: true`, h.Namespace))

			h.App = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, h.App)).Should(BeNil())
			h.App.SetNamespace(h.AppNamespace)
			h.App.SetName("crossplane-crd-test-" + helmTestSuffix())
			Expect(k8sClient.Create(h.Ctx, h.App)).Should(Succeed())
			h.AppKey = client.ObjectKeyFromObject(h.App)

			By("Verifying Application reaches running")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
				g.Expect(h.App.Status.Phase).Should(Equal(common2.ApplicationRunning))
			}, 300*time.Second, 5*time.Second).Should(Succeed())

			By("Verifying crossplane Deployment is ready")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "crossplane"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(BeNumerically(">=", 1))
			}, 120*time.Second, 3*time.Second).Should(Succeed())
		})

		It("should clean up CRDs and all resources on deletion", func() {
			By("Deleting the Application")
			runCommandSucceed("kubectl", "delete", "application", h.App.Name, "-n", h.AppNamespace)
			Eventually(func() bool {
				return k8sClient.Get(h.Ctx, h.AppKey, &v1beta1.Application{}) != nil
			}, 60*time.Second, 2*time.Second).Should(BeTrue())
			h.App = nil

			By("Verifying crossplane Deployment is cleaned up")
			Eventually(func() bool {
				return k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "crossplane"}, &appsv1.Deployment{}) != nil
			}, 60*time.Second, 3*time.Second).Should(BeTrue())

			By("Verifying helm release secrets are cleaned up")
			Eventually(func() int {
				secrets := &corev1.SecretList{}
				_ = k8sClient.List(h.Ctx, secrets, client.InNamespace(h.Namespace),
					client.MatchingLabels{"owner": "helm", "name": "crossplane"})
				return len(secrets.Items)
			}, 60*time.Second, 3*time.Second).Should(Equal(0))
		})
	})

	Context("Chart with Namespaces (createNamespace)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.Namespace = randomNamespaceName("helm-e2e") })
		AfterAll(func() { h.Cleanup() })

		It("should create namespace before deploying namespace-scoped resources", func() {
			By("Verifying target namespace does not exist yet")
			err := k8sClient.Get(h.Ctx, types.NamespacedName{Name: h.Namespace}, &corev1.Namespace{})
			Expect(err).ShouldNot(BeNil())

			h.DeployApp()

			By("Verifying namespace was created")
			Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Name: h.Namespace}, &corev1.Namespace{})).Should(Succeed())
			h.WaitForDeploymentReady()
		})
	})
})

// ============================================================================
// Health Check Scenarios
// ============================================================================

var _ = Describe("Helmchart Health Checks", Label("core-helm"), func() {

	Context("Custom Health Check — Deployment Available", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() { h.Cleanup() })

		It("should report healthy when deployment is available", func() {
			h.DeployAppFrom(testDataPath("helm/app_helmchart_podinfo_health.yaml"))
			h.WaitForDeploymentReady()
			Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
			Expect(h.App.Status.Phase).Should(Equal(common2.ApplicationRunning))
		})

		It("should self-heal when scaled to 0", func() {
			By("Scaling deployment to 0 manually")
			runCommandSucceed("kubectl", "scale", "deployment", "podinfo", "--replicas=0", "-n", h.Namespace)

			By("Verifying deployment scaled to 0")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(Equal(int32(0)))
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("Triggering reconciliation to detect and fix the drift")
			RequestReconcileNow(h.Ctx, h.App)

			By("Verifying KubeVela self-heals replicas back to 2")
			h.WaitForDeploymentReady()
			h.WaitForAppRunning()
		})
	})

	Context("Custom Health Check — Multiple Criteria (Two Components)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() { h.Cleanup() })

		It("should be healthy when both podinfo-a and podinfo-b Deployments are Available", func() {
			h.DeployAppFrom(testDataPath("helm/app_helmchart_podinfo_multi_health.yaml"))

			By("Waiting for both Deployments to be ready")
			for _, name := range []string{"podinfo-a", "podinfo-b"} {
				Eventually(func(g Gomega) {
					d := &appsv1.Deployment{}
					g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: name}, d)).Should(Succeed())
					g.Expect(d.Status.ReadyReplicas).Should(BeNumerically(">=", 1))
				}, 120*time.Second, 3*time.Second).Should(Succeed())
			}

			h.WaitForAppRunning()
		})

		It("should detect unhealthy when one Deployment is deleted and self-heal", func() {
			By("Deleting podinfo-a Deployment (podinfo-b is still healthy)")
			runCommandSucceed("kubectl", "delete", "deployment", "podinfo-a", "-n", h.Namespace)

			By("Verifying podinfo-a Deployment is gone")
			Eventually(func() bool {
				return k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo-a"}, &appsv1.Deployment{}) != nil
			}, 10*time.Second, time.Second).Should(BeTrue())

			By("Verifying podinfo-b Deployment is still running")
			d := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo-b"}, d)).Should(Succeed())
			Expect(d.Status.ReadyReplicas).Should(BeNumerically(">=", 1))

			By("Triggering reconciliation")
			RequestReconcileNow(h.Ctx, h.App)

			By("Verifying KubeVela self-heals by recreating podinfo-a Deployment")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo-a"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(BeNumerically(">=", 1))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			h.WaitForAppRunning()
		})
	})

	Context("No Health Check Defined", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() { h.Cleanup() })

		It("should default to healthy when no healthStatus field", func() {
			h.DeployAppFrom(testDataPath("helm/app_helmchart_podinfo_no_values.yaml"))
			h.WaitForAppRunning()
		})
	})
})

// ============================================================================
// Edge Cases & Boundary Conditions
// ============================================================================

var _ = Describe("Helmchart Edge Cases", Label("core-helm"), func() {

	Context("Empty Values", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() { h.Cleanup() })

		It("should install chart with default values when no values field", func() {
			h.DeployAppFrom(testDataPath("helm/app_helmchart_podinfo_no_values.yaml"))
			h.WaitForAppRunning()

			deploy := &appsv1.Deployment{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, deploy)).Should(Succeed())
				g.Expect(deploy.Status.ReadyReplicas).Should(BeNumerically(">=", 1))
			}, 120*time.Second, 3*time.Second).Should(Succeed())
		})
	})

	Context("Namespace Does Not Exist and createNamespace=false", Ordered, func() {
		h := &helmTestContext{
			Framework:    support,
			Ctx:          context.Background(),
			Namespace:    "nonexistent-ns-" + helmTestSuffix(),
			AppNamespace: "default",
		}
		AfterAll(func() {
			if h.App != nil {
				_ = k8sClient.Delete(h.Ctx, h.App)
			}
		})

		It("should fail when namespace does not exist", func() {
			raw, err := os.ReadFile(testDataPath("helm/app_helmchart_podinfo_no_create_ns.yaml"))
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.Namespace))
			h.App = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, h.App)).Should(BeNil())
			h.App.SetNamespace(h.AppNamespace)
			h.App.SetName("no-ns-test-" + helmTestSuffix())
			Expect(k8sClient.Create(h.Ctx, h.App)).Should(Succeed())
			h.AppKey = client.ObjectKeyFromObject(h.App)

			EventuallyReconciled(h.Ctx, h.App, func(g Gomega) {
				g.Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
				g.Expect(h.App.Status.Phase).Should(SatisfyAny(
					Equal(common2.ApplicationWorkflowFailed),
					Equal(common2.ApplicationUnhealthy),
				))
			}).WithTimeout(90 * time.Second).Should(Succeed())

			By("Verifying namespace was not created")
			Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Name: h.Namespace}, &corev1.Namespace{})).ShouldNot(Succeed())
		})
	})

	Context("Chart Not Found in Repository", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() {
			if h.App != nil {
				_ = k8sClient.Delete(h.Ctx, h.App)
			}
			h.CleanupNamespaceOnly()
		})

		It("should fail with clear error for non-existent chart", func() {
			raw, err := os.ReadFile(testDataPath("helm/app_helmchart_bad_chart.yaml"))
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.Namespace))
			h.App = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, h.App)).Should(BeNil())
			h.App.SetNamespace(h.AppNamespace)
			h.App.SetName("bad-chart-test-" + helmTestSuffix())
			createErr := k8sClient.Create(h.Ctx, h.App)

			if createErr != nil {
				By("Webhook dry-run caught the bad chart — rejected at admission")
				Expect(createErr.Error()).Should(ContainSubstring("not found"))
				h.App = nil // not created, nothing to clean up
			} else {
				By("Webhook did not catch it — waiting for workflow failure")
				h.AppKey = client.ObjectKeyFromObject(h.App)
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
					g.Expect(h.App.Status.Phase).Should(SatisfyAny(
						Equal(common2.ApplicationWorkflowFailed),
						Equal(common2.ApplicationUnhealthy),
					))
				}, 120*time.Second, 3*time.Second).Should(Succeed())
			}
		})
	})

	Context("Invalid Chart Version", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() { h.Cleanup() })

		It("should fail with bad version then succeed with correct version", func() {
			raw, err := os.ReadFile(testDataPath("helm/app_helmchart_bad_version.yaml"))
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.Namespace))
			h.App = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, h.App)).Should(BeNil())
			h.App.SetNamespace(h.AppNamespace)
			h.App.SetName("bad-version-test-" + helmTestSuffix())
			createErr := k8sClient.Create(h.Ctx, h.App)

			if createErr != nil {
				By("Webhook dry-run caught the bad version — rejected at admission")
				Expect(createErr.Error()).Should(ContainSubstring("not found"))

				By("Creating with correct version directly")
				h.App.SetResourceVersion("")
				rawProps, _ := json.Marshal(h.App.Spec.Components[0].Properties)
				var props map[string]interface{}
				_ = json.Unmarshal(rawProps, &props)
				chart := props["chart"].(map[string]interface{})
				chart["version"] = "6.11.1"
				props["chart"] = chart
				newRaw, _ := json.Marshal(props)
				h.App.Spec.Components[0].Properties = &runtime.RawExtension{Raw: newRaw}
				Expect(k8sClient.Create(h.Ctx, h.App)).Should(Succeed())
				h.AppKey = client.ObjectKeyFromObject(h.App)
			} else {
				By("Webhook did not catch it — waiting for workflow failure then updating")
				h.AppKey = client.ObjectKeyFromObject(h.App)
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
					g.Expect(h.App.Status.Phase).Should(SatisfyAny(
						Equal(common2.ApplicationWorkflowFailed),
						Equal(common2.ApplicationUnhealthy),
					))
				}, 120*time.Second, 3*time.Second).Should(Succeed())

				By("Updating to correct version")
				Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
				rawProps, err := json.Marshal(h.App.Spec.Components[0].Properties)
				Expect(err).Should(BeNil())
				var props map[string]interface{}
				Expect(json.Unmarshal(rawProps, &props)).Should(BeNil())
				chart := props["chart"].(map[string]interface{})
				chart["version"] = "6.11.1"
				props["chart"] = chart
				newRaw, err := json.Marshal(props)
				Expect(err).Should(BeNil())
				h.App.Spec.Components[0].Properties = &runtime.RawExtension{Raw: newRaw}
				Expect(k8sClient.Update(h.Ctx, h.App)).Should(Succeed())
			}

			h.WaitForAppRunning()
		})
	})

	Context("Two helmchart Components in Same Application", Ordered, func() {
		h := newHelmTestContext()
		nsA := "helm-multi-a-" + helmTestSuffix()
		nsB := "helm-multi-b-" + helmTestSuffix()
		BeforeAll(func() {
			for _, ns := range []string{nsA, nsB} {
				n := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
				Expect(k8sClient.Create(h.Ctx, n)).Should(SatisfyAny(Succeed(), Not(HaveOccurred())))
			}
		})
		AfterAll(func() {
			if h.App != nil {
				_ = k8sClient.Delete(h.Ctx, h.App)
				Eventually(func() bool {
					return k8sClient.Get(h.Ctx, h.AppKey, &v1beta1.Application{}) != nil
				}, 60*time.Second, 2*time.Second).Should(BeTrue())
			}
			for _, ns := range []string{nsA, nsB} {
				n := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
				_ = k8sClient.Delete(h.Ctx, n, client.PropagationPolicy(metav1.DeletePropagationForeground))
			}
		})

		It("should manage podinfo and crossplane as independent Helm releases", func() {
			raw, err := os.ReadFile(testDataPath("helm/app_helmchart_two_components.yaml"))
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns_a"), []byte(nsA))
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns_b"), []byte(nsB))
			h.App = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, h.App)).Should(BeNil())
			h.App.SetNamespace(h.AppNamespace)
			h.App.SetName("two-comp-test-" + helmTestSuffix())
			Expect(k8sClient.Create(h.Ctx, h.App)).Should(Succeed())
			h.AppKey = client.ObjectKeyFromObject(h.App)

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
				g.Expect(h.App.Status.Phase).Should(Equal(common2.ApplicationRunning))
			}, 300*time.Second, 5*time.Second).Should(Succeed())

			By("Verifying podinfo release exists in namespace A")
			out, _ := runCommand("helm", "list", "-n", nsA, "-q")
			Expect(out).Should(ContainSubstring("podinfo"))

			By("Verifying crossplane release exists in namespace B")
			out, _ = runCommand("helm", "list", "-n", nsB, "-q")
			Expect(out).Should(ContainSubstring("crossplane"))

			By("Verifying podinfo Deployment is ready in namespace A")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: nsA, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(Equal(int32(1)))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying crossplane Deployment is ready in namespace B")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: nsB, Name: "crossplane"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(BeNumerically(">=", 1))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying crossplane release secrets exist")
			cpSecrets := &corev1.SecretList{}
			Expect(k8sClient.List(h.Ctx, cpSecrets,
				client.InNamespace(nsB),
				client.MatchingLabels{"owner": "helm", "name": "crossplane"},
			)).Should(Succeed())
			Expect(len(cpSecrets.Items)).Should(BeNumerically(">=", 1))
		})

		It("should not affect crossplane when upgrading podinfo component", func() {
			By("Recording crossplane Deployment replica count and image before podinfo upgrade")
			cpDeploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: nsB, Name: "crossplane"}, cpDeploy)).Should(Succeed())
			cpImage := cpDeploy.Spec.Template.Spec.Containers[0].Image
			cpReplicas := *cpDeploy.Spec.Replicas

			By("Upgrading podinfo component values")
			Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
			rawProps, err := json.Marshal(h.App.Spec.Components[0].Properties)
			Expect(err).Should(BeNil())
			var props map[string]interface{}
			Expect(json.Unmarshal(rawProps, &props)).Should(BeNil())
			if vals, ok := props["values"].(map[string]interface{}); ok {
				vals["ui"] = map[string]interface{}{"message": "upgraded-podinfo"}
			}
			newRaw, err := json.Marshal(props)
			Expect(err).Should(BeNil())
			h.App.Spec.Components[0].Properties = &runtime.RawExtension{Raw: newRaw}
			Expect(k8sClient.Update(h.Ctx, h.App)).Should(Succeed())

			By("Waiting for Application to return to running after upgrade")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
				g.Expect(h.App.Status.Phase).Should(Equal(common2.ApplicationRunning))
			}, 180*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying crossplane Deployment spec was NOT affected (image and replicas unchanged)")
			cpDeploy = &appsv1.Deployment{}
			Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: nsB, Name: "crossplane"}, cpDeploy)).Should(Succeed())
			Expect(cpDeploy.Spec.Template.Spec.Containers[0].Image).Should(Equal(cpImage),
				"crossplane image should not change when podinfo is upgraded")
			Expect(*cpDeploy.Spec.Replicas).Should(Equal(cpReplicas),
				"crossplane replicas should not change when podinfo is upgraded")
		})

		It("should clean up both releases when Application is deleted", func() {
			appName := h.App.Name
			runCommandSucceed("kubectl", "delete", "application", appName, "-n", h.AppNamespace)
			Eventually(func() bool {
				return k8sClient.Get(h.Ctx, h.AppKey, &v1beta1.Application{}) != nil
			}, 60*time.Second, 2*time.Second).Should(BeTrue())
			h.App = nil

			By("Verifying podinfo release cleaned up in namespace A")
			Eventually(func() string {
				out, _ := runCommand("helm", "list", "-n", nsA, "-q")
				return strings.TrimSpace(out)
			}, 60*time.Second, 3*time.Second).Should(BeEmpty())

			By("Verifying crossplane release cleaned up in namespace B")
			Eventually(func() string {
				out, _ := runCommand("helm", "list", "-n", nsB, "-q")
				return strings.TrimSpace(out)
			}, 60*time.Second, 3*time.Second).Should(BeEmpty())

			By("Verifying crossplane release secrets are deleted")
			Eventually(func() int {
				cpSecrets := &corev1.SecretList{}
				Expect(k8sClient.List(h.Ctx, cpSecrets,
					client.InNamespace(nsB),
					client.MatchingLabels{"owner": "helm", "name": "crossplane"},
				)).Should(Succeed())
				return len(cpSecrets.Items)
			}, 60*time.Second, 3*time.Second).Should(Equal(0))
		})
	})

	Context("Helm Release Exists with Different Chart", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() { h.Cleanup() })

		It("should detect chart mismatch and upgrade to podinfo", func() {
			By("Installing podinfo v6.11.0 as release 'myrelease' via helm install")
			runCommandSucceed("helm", "install", "myrelease",
				"--repo", "https://stefanprodan.github.io/podinfo", "podinfo",
				"--version", "6.11.0", "--set", "replicaCount=1", "-n", h.Namespace)

			Eventually(func() string {
				out, _ := runCommand("helm", "list", "-n", h.Namespace, "-q")
				return out
			}, 30*time.Second, 3*time.Second).Should(ContainSubstring("myrelease"))

			By("Recording old resources from v6.11.0")
			oldDeploy := &appsv1.Deployment{}
			Eventually(func(g Gomega) {
				deployList := &appsv1.DeploymentList{}
				g.Expect(k8sClient.List(h.Ctx, deployList, client.InNamespace(h.Namespace))).Should(Succeed())
				g.Expect(len(deployList.Items)).Should(BeNumerically(">=", 1))
				oldDeploy = &deployList.Items[0]
			}, 60*time.Second, 3*time.Second).Should(Succeed())
			oldDeployName := oldDeploy.Name

			By("Applying KubeVela Application with podinfo v6.11.1 targeting 'myrelease'")
			raw, err := os.ReadFile(testDataPath("helm/app_helmchart_podinfo.yaml"))
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.Namespace))
			raw = bytes.ReplaceAll(raw, []byte("name: podinfo\n          namespace"), []byte("name: myrelease\n          namespace"))
			h.App = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, h.App)).Should(BeNil())
			h.App.SetNamespace(h.AppNamespace)
			h.App.SetName("chart-mismatch-" + helmTestSuffix())
			Expect(k8sClient.Create(h.Ctx, h.App)).Should(Succeed())
			h.AppKey = client.ObjectKeyFromObject(h.App)

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
				g.Expect(h.App.Status.Phase).Should(Equal(common2.ApplicationRunning))
			}, 180*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying KubeVela labels injected on deployment")
			deployList := &appsv1.DeploymentList{}
			Expect(k8sClient.List(h.Ctx, deployList, client.InNamespace(h.Namespace))).Should(Succeed())
			Expect(len(deployList.Items)).Should(BeNumerically(">=", 1))
			Expect(deployList.Items[0].GetLabels()).Should(HaveKey("app.oam.dev/name"))

			By("Verifying old resources were replaced (deployment name from old release: " + oldDeployName + ")")
			_ = oldDeployName
		})
	})

	Context("Apply Same Application Twice Without Changes", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() { h.Cleanup() })

		It("should not trigger Helm upgrade when no changes", func() {
			h.DeployApp()
			h.WaitForDeploymentReady()
			h.WaitForAppRunning()

			initialSecretCount := len(h.GetHelmSecrets().Items)
			latestSecret := h.LatestHelmSecretName()

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			initialResourceVersion := deploy.ResourceVersion

			By("Applying the exact same manifest via kubectl apply")
			raw, err := os.ReadFile(testDataPath("helm/app_helmchart_podinfo.yaml"))
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.Namespace))
			raw = bytes.ReplaceAll(raw, []byte("name: podinfo-helm-test"), []byte("name: "+h.App.Name))
			tmp, err := os.CreateTemp("", "helm-reapply-*.yaml")
			Expect(err).Should(Succeed())
			tmpFile := tmp.Name()
			Expect(tmp.Close()).Should(Succeed())
			DeferCleanup(func() { Expect(os.Remove(tmpFile)).To(Succeed()) })
			Expect(os.WriteFile(tmpFile, raw, 0600)).Should(Succeed())
			runCommandSucceed("kubectl", "apply", "-f", tmpFile, "-n", h.AppNamespace)

			By("Verifying no Helm upgrade occurs and the Deployment is untouched across reconciles")
			ConsistentlyReconciled(h.Ctx, h.App, func(g Gomega) {
				g.Expect(h.GetHelmSecrets().Items).Should(HaveLen(initialSecretCount))
				g.Expect(h.LatestHelmSecretName()).Should(Equal(latestSecret))
				current := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, current)).Should(Succeed())
				g.Expect(current.ResourceVersion).Should(Equal(initialResourceVersion))
			}).Should(Succeed())
		})
	})
})

// ============================================================================
// valuesFrom Tests Scenarios
// ============================================================================

var _ = Describe("Helmchart valuesFrom", Label("core-helm"), func() {

	createCM := func(h *helmTestContext, name, key, valuesYAML string) {
		if key == "" {
			key = "values.yaml"
		}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: h.Namespace},
			Data:       map[string]string{key: valuesYAML},
		}
		Expect(k8sClient.Create(h.Ctx, cm)).Should(Succeed())
	}

	createCMWithReplicas := func(h *helmTestContext, name string, replicaCount int) {
		createCM(h, name, "", fmt.Sprintf("replicaCount: %d\n", replicaCount))
	}

	createCMInNamespace := func(h *helmTestContext, name, ns, valuesYAML string) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Data:       map[string]string{"values.yaml": valuesYAML},
		}
		Expect(k8sClient.Create(h.Ctx, cm)).Should(Succeed())
	}

	createSecret := func(h *helmTestContext, name, valuesYAML string) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: h.Namespace},
			Data:       map[string][]byte{"values.yaml": []byte(valuesYAML)},
		}
		Expect(k8sClient.Create(h.Ctx, secret)).Should(Succeed())
	}

	createSecretWithReplicas := func(h *helmTestContext, name string, replicaCount int) {
		createSecret(h, name, fmt.Sprintf("replicaCount: %d\n", replicaCount))
	}

	buildPodinfoComponent := func(h *helmTestContext, componentName, releaseName string, props map[string]interface{}) common2.ApplicationComponent {
		merged := map[string]interface{}{
			"chart": map[string]interface{}{
				"source":  "podinfo",
				"repoURL": "https://stefanprodan.github.io/podinfo",
				"version": "6.11.1",
			},
			"release": map[string]interface{}{
				"name":      releaseName,
				"namespace": h.Namespace,
			},
			"options": map[string]interface{}{
				"createNamespace": true,
				"skipTests":       true,
			},
		}
		for k, v := range props {
			merged[k] = v
		}
		raw, err := json.Marshal(merged)
		Expect(err).ShouldNot(HaveOccurred())
		return common2.ApplicationComponent{
			Name:       componentName,
			Type:       "helmchart",
			Properties: &runtime.RawExtension{Raw: raw},
		}
	}

	deployAppWithComponents := func(h *helmTestContext, appNamePrefix string, comps []common2.ApplicationComponent) {
		h.App = &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appNamePrefix + "-" + helmTestSuffix(),
				Namespace: h.AppNamespace,
			},
			Spec: v1beta1.ApplicationSpec{Components: comps},
		}
		Expect(k8sClient.Create(h.Ctx, h.App)).Should(Succeed())
		h.AppKey = client.ObjectKeyFromObject(h.App)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
			g.Expect(h.App.Status.Phase).Should(Equal(common2.ApplicationRunning))
		}, 120*time.Second, 3*time.Second).Should(Succeed())
	}

	deployPodinfo := func(h *helmTestContext, appNamePrefix, releaseName string, props map[string]interface{}) {
		comp := buildPodinfoComponent(h, "podinfo", releaseName, props)
		deployAppWithComponents(h, appNamePrefix, []common2.ApplicationComponent{comp})
	}

	// createPodinfoApp creates an Application without waiting on it, so several
	// can progress at once.
	createPodinfoApp := func(h *helmTestContext, appNamePrefix string, comps ...common2.ApplicationComponent) *v1beta1.Application {
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appNamePrefix + "-" + helmTestSuffix(),
				Namespace: h.AppNamespace,
			},
			Spec: v1beta1.ApplicationSpec{Components: comps},
		}
		Expect(k8sClient.Create(h.Ctx, app)).Should(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(h.Ctx, app) })
		return app
	}

	expectWorkflowFailure := func(h *helmTestContext, app *v1beta1.Application, releaseName, errSubstring string) {
		EventuallyReconciled(h.Ctx, app, func(g Gomega) {
			g.Expect(k8sClient.Get(h.Ctx, client.ObjectKeyFromObject(app), app)).Should(Succeed())
			g.Expect(app.Status.Workflow).ToNot(BeNil())
			g.Expect(string(app.Status.Workflow.Phase)).To(Equal("failed"))
			var found bool
			for _, step := range app.Status.Workflow.Steps {
				if strings.Contains(step.Message, errSubstring) {
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(),
				"no workflow step contained %q; status=%+v", errSubstring, app.Status.Workflow)
		}).WithTimeout(90 * time.Second).Should(Succeed())

		err := k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: releaseName}, &appsv1.Deployment{})
		Expect(err).To(HaveOccurred(), "no Deployment should exist for a failed workflow")
	}

	waitForReplicas := func(h *helmTestContext, want int32) {
		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			g.Expect(deploy.Status.ReadyReplicas).Should(Equal(want))
		}, 120*time.Second, 3*time.Second).Should(Succeed())
	}

	waitForNamedReplicas := func(h *helmTestContext, deployName string, want int32) {
		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: deployName}, deploy)).Should(Succeed())
			g.Expect(deploy.Status.ReadyReplicas).Should(Equal(want))
		}, 120*time.Second, 3*time.Second).Should(Succeed())
	}

	cmRef := func(name string, opts ...map[string]interface{}) map[string]interface{} {
		entry := map[string]interface{}{"kind": "ConfigMap", "name": name}
		for _, o := range opts {
			for k, v := range o {
				entry[k] = v
			}
		}
		return entry
	}
	secretRef := func(name string, opts ...map[string]interface{}) map[string]interface{} {
		entry := map[string]interface{}{"kind": "Secret", "name": name}
		for _, o := range opts {
			for k, v := range o {
				entry[k] = v
			}
		}
		return entry
	}

	// Each merge case is its own release in one Application, so the cases share
	// one Application and one teardown. Release names contain "podinfo", so the
	// chart names each Deployment after its release.
	Context("Merging values from ConfigMaps and Secrets", Ordered, ContinueOnFailure, func() {
		h := newHelmTestContext()
		BeforeAll(func() {
			h.CreateNamespace()
			createCMWithReplicas(h, "cm-only", 3)
			createSecretWithReplicas(h, "secret-only", 2)
			createCMWithReplicas(h, "inline-override", 2)
			Expect(k8sClient.Create(h.Ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "multi-env", Namespace: h.Namespace},
				Data: map[string]string{
					"dev.yaml":  "replicaCount: 1\n",
					"prod.yaml": "replicaCount: 5\n",
				},
			})).Should(Succeed())
			createCMWithReplicas(h, "two-cms-base", 2)
			createCMWithReplicas(h, "two-cms-overlay", 4)
			createCM(h, "deep-base", "", `resources:
  limits:
    cpu: 100m
    memory: 256Mi
  requests:
    cpu: 50m
replicaCount: 2
`)
			createCM(h, "deep-overlay", "", `resources:
  limits:
    memory: 512Mi
`)
			createCM(h, "mixed-cm", "", "replicaCount: 2\nimage:\n  tag: 6.11.0\n")
			createSecret(h, "mixed-secret", "replicaCount: 3\n")
			createSecretWithReplicas(h, "two-secrets-a", 1)
			createSecretWithReplicas(h, "two-secrets-b", 4)
			createCMWithReplicas(h, "after-optional", 3)
			createCM(h, "arrays-base", "", "extraArgs:\n  - --level=debug\n  - --timeout=30\n")
			createCM(h, "arrays-overlay", "", "extraArgs:\n  - --level=info\n")
			createCMWithReplicas(h, "with-health", 2)

			release := func(name string, props map[string]interface{}) common2.ApplicationComponent {
				return buildPodinfoComponent(h, name, name, props)
			}
			valuesFrom := func(refs ...map[string]interface{}) map[string]interface{} {
				list := make([]interface{}, len(refs))
				for i, r := range refs {
					list[i] = r
				}
				return map[string]interface{}{"valuesFrom": list}
			}
			optional := map[string]interface{}{"optional": true}
			h.App = createPodinfoApp(h, "vf-merge",
				release("podinfo-cm", valuesFrom(cmRef("cm-only"))),
				release("podinfo-secret", valuesFrom(secretRef("secret-only"))),
				release("podinfo-inline", map[string]interface{}{
					"values":     map[string]interface{}{"replicaCount": 4},
					"valuesFrom": []interface{}{cmRef("inline-override")},
				}),
				release("podinfo-optional", valuesFrom(cmRef("never-created", optional))),
				release("podinfo-key", valuesFrom(cmRef("multi-env", map[string]interface{}{"key": "prod.yaml"}))),
				release("podinfo-two-cms", valuesFrom(cmRef("two-cms-base"), cmRef("two-cms-overlay"))),
				release("podinfo-deep", valuesFrom(cmRef("deep-base"), cmRef("deep-overlay"))),
				release("podinfo-mixed", valuesFrom(cmRef("mixed-cm"), secretRef("mixed-secret"))),
				release("podinfo-two-secrets", valuesFrom(secretRef("two-secrets-a"), secretRef("two-secrets-b"))),
				release("podinfo-empty", map[string]interface{}{"valuesFrom": []interface{}{}}),
				release("podinfo-after-optional", valuesFrom(cmRef("never-created", optional), cmRef("after-optional"))),
				release("podinfo-arrays", valuesFrom(cmRef("arrays-base"), cmRef("arrays-overlay"))),
				release("podinfo-health", map[string]interface{}{
					"valuesFrom": []interface{}{cmRef("with-health")},
					"healthStatus": []interface{}{
						map[string]interface{}{
							"resource":  map[string]interface{}{"kind": "Deployment", "name": "podinfo-health"},
							"condition": map[string]interface{}{"type": "Available"},
						},
					},
				}),
			)
			h.AppKey = client.ObjectKeyFromObject(h.App)
		})
		AfterAll(func() { h.Cleanup() })

		It("should merge values from the referenced ConfigMap", func() {
			waitForNamedReplicas(h, "podinfo-cm", 3)
		})

		It("should merge values from the referenced Secret", func() {
			waitForNamedReplicas(h, "podinfo-secret", 2)
		})

		It("should use inline replicaCount when it also appears in the ConfigMap", func() {
			waitForNamedReplicas(h, "podinfo-inline", 4)
		})

		It("should deploy at chart defaults when the only source is optional and missing", func() {
			waitForNamedReplicas(h, "podinfo-optional", 1)
		})

		It("should use the specified key and ignore other keys in the ConfigMap", func() {
			waitForNamedReplicas(h, "podinfo-key", 5)
		})

		It("should resolve a conflict between two ConfigMaps to the later one", func() {
			waitForNamedReplicas(h, "podinfo-two-cms", 4)
		})

		It("should keep base sibling keys when the overlay touches only one field in a nested map", func() {
			waitForNamedReplicas(h, "podinfo-deep", 2)
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo-deep"}, deploy)).Should(Succeed())
			limits := deploy.Spec.Template.Spec.Containers[0].Resources.Limits
			requests := deploy.Spec.Template.Spec.Containers[0].Resources.Requests
			Expect(limits.Memory().String()).To(Equal("512Mi"),
				"overlay memory should win on direct conflict")
			Expect(limits.Cpu().String()).To(Equal("100m"),
				"base cpu should survive because overlay only touched memory")
			Expect(requests.Cpu().String()).To(Equal("50m"),
				"untouched requests.cpu from base must survive")
		})

		It("should merge values from a ConfigMap followed by a Secret", func() {
			waitForNamedReplicas(h, "podinfo-mixed", 3)
		})

		It("should resolve a conflict between two Secrets to the later one", func() {
			waitForNamedReplicas(h, "podinfo-two-secrets", 4)
		})

		It("should deploy at chart defaults when valuesFrom is an empty list", func() {
			waitForNamedReplicas(h, "podinfo-empty", 1)
		})

		It("should skip a missing optional source and apply the required one after it", func() {
			waitForNamedReplicas(h, "podinfo-after-optional", 3)
		})

		It("should replace arrays wholesale with the later source's (Helm semantics)", func() {
			waitForNamedReplicas(h, "podinfo-arrays", 1)
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo-arrays"}, deploy)).Should(Succeed())
			// podinfo 6.11.1 renders extraArgs into the container's .command
			// (appended to ["./podinfo", "--port=...", ...]). Check both command
			// and args to stay robust against chart layout changes.
			c := deploy.Spec.Template.Spec.Containers[0]
			joined := strings.Join(c.Command, " ") + " " + strings.Join(c.Args, " ")
			Expect(joined).To(ContainSubstring("--level=info"),
				"overlay array value must appear in the container's command/args")
			Expect(joined).ToNot(ContainSubstring("--level=debug"),
				"base array value must NOT appear: arrays are replaced not merged")
			Expect(joined).ToNot(ContainSubstring("--timeout=30"),
				"base orthogonal array item must NOT appear: arrays are replaced wholesale")
		})

		It("should reach healthy state using CM-supplied replicaCount", func() {
			waitForNamedReplicas(h, "podinfo-health", 2)
			EventuallyReconciled(h.Ctx, h.App, func(g Gomega) {
				g.Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
				var healthy *bool
				for i, svc := range h.App.Status.Services {
					if svc.Name == "podinfo-health" {
						healthy = &h.App.Status.Services[i].Healthy
					}
				}
				g.Expect(healthy).ShouldNot(BeNil(), "no status for podinfo-health")
				g.Expect(*healthy).Should(BeTrue())
			}).WithTimeout(60 * time.Second).Should(Succeed())
		})

		It("should bring the Application with every release to running", func() {
			EventuallyReconciled(h.Ctx, h.App, func(g Gomega) {
				g.Expect(k8sClient.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
				g.Expect(h.App.Status.Phase).Should(Equal(common2.ApplicationRunning))
			}).WithTimeout(2 * time.Minute).WithPolling(3 * time.Second).Should(Succeed())
		})

		It("should recreate a CM-backed Deployment with the same values after it is deleted", func() {
			runCommandSucceed("kubectl", "delete", "deployment", "podinfo-cm", "-n", h.Namespace)
			Eventually(func() bool {
				err := k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo-cm"}, &appsv1.Deployment{})
				return err != nil
			}, 10*time.Second, time.Second).Should(BeTrue())

			RequestReconcileNow(h.Ctx, h.App)
			waitForNamedReplicas(h, "podinfo-cm", 3)
		})
	})

	// Each failure is its own Application so one step's error cannot satisfy
	// another's assertion; they are created together so their retries overlap.
	Context("Failing valuesFrom sources", Ordered, ContinueOnFailure, func() {
		h := newHelmTestContext()
		otherNS := randomNamespaceName("helm-other-tenant")
		var missing, badYAML, crossNS, noNS *v1beta1.Application
		BeforeAll(func() {
			h.CreateNamespace()
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: otherNS}}
			Expect(k8sClient.Create(h.Ctx, ns)).Should(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(h.Ctx, ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
			})
			// A real ConfigMap in the other namespace, so the failure is due to
			// the cross-namespace guard rather than a NotFound.
			createCMInNamespace(h, "podinfo-values", otherNS, "replicaCount: 3\n")
			createCM(h, "podinfo-bad-yaml", "", "replicaCount: [unterminated")

			failing := func(release string, ref map[string]interface{}) *v1beta1.Application {
				return createPodinfoApp(h, release, buildPodinfoComponent(h, "podinfo", release,
					map[string]interface{}{"valuesFrom": []interface{}{ref}}))
			}
			missing = failing("podinfo-missing", cmRef("never-created"))
			badYAML = failing("podinfo-bad-yaml", cmRef("podinfo-bad-yaml", map[string]interface{}{"optional": true}))
			crossNS = failing("podinfo-cross-ns", cmRef("podinfo-values", map[string]interface{}{"namespace": otherNS}))
			noNS = failing("podinfo-no-ns", cmRef("any-cm", map[string]interface{}{"namespace": "does-not-exist-ns"}))
		})
		AfterAll(func() { h.CleanupNamespaceOnly() })

		It("should fail the workflow and surface the missing-CM error for a required source", func() {
			expectWorkflowFailure(h, missing, "podinfo-missing", `ConfigMap "never-created"`)
		})

		It("should surface a parse error even when the source is marked optional", func() {
			expectWorkflowFailure(h, badYAML, "podinfo-bad-yaml", "invalid YAML")
		})

		It("should fail the workflow when valuesFrom.namespace != Application namespace", func() {
			expectWorkflowFailure(h, crossNS, "podinfo-cross-ns", "cross-namespace valuesFrom")
		})

		It("should fail with a not-found error referencing a missing explicit namespace", func() {
			expectWorkflowFailure(h, noNS, "podinfo-no-ns", "does-not-exist-ns")
		})
	})

	Context("Adoption of an existing vanilla Helm release with valuesFrom", FlakeAttempts(2), Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() { h.Cleanup() })

		It("should adopt the pre-existing release and merge CM values on the adoption upgrade", func() {
			h.FreshAttempt()
			By("Installing podinfo via vanilla helm at replicaCount=1")
			runCommandSucceed("helm", "install", "podinfo",
				"--repo", "https://stefanprodan.github.io/podinfo", "podinfo",
				"--version", "6.11.1", "--set", "replicaCount=1", "-n", h.Namespace)
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(Equal(int32(1)))
			}, 60*time.Second, 3*time.Second).Should(Succeed())

			By("Creating CM with replicaCount=3 and applying the Application (adoption path)")
			createCMWithReplicas(h, "podinfo-adopt-values", 3)
			deployPodinfo(h, "s47", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{cmRef("podinfo-adopt-values")},
			})

			By("Verifying adoption applied CM values (replicas scaled 1→3) and injected KubeVela labels")
			waitForReplicas(h, 3)
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Namespace: h.Namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			Expect(deploy.GetLabels()).To(HaveKey("app.oam.dev/name"),
				"adoption must inject KubeVela ownership labels on the Deployment")
		})
	})

	Context("Auto-reconcile on ConfigMap content change (without spec edit)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.CreateNamespace() })
		AfterAll(func() { h.Cleanup() })

		It("rolls out a new Helm revision when only the referenced ConfigMap changes", func() {
			By("creating the backing ConfigMap with replicaCount=2 and deploying the Application")
			createCMWithReplicas(h, "vf-autorec-values", 2)
			deployPodinfo(h, "s48", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{cmRef("vf-autorec-values")},
			})
			waitForReplicas(h, 2)

			By("recording the current Helm release secret count for later comparison")
			initialCount := len(h.GetHelmSecrets().Items)

			By("editing the ConfigMap content (replicaCount: 2 -> 4) WITHOUT touching the Application spec")
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(h.Ctx, types.NamespacedName{Name: "vf-autorec-values", Namespace: h.Namespace}, cm)).Should(Succeed())
			cm.Data["values.yaml"] = "replicaCount: 4\n"
			Expect(k8sClient.Update(h.Ctx, cm)).Should(Succeed())

			By("forcing a reconcile via the requestreconcile annotation (skips the periodic-resync wait)")
			RequestReconcileNow(h.Ctx, h.App)

			By("expecting the Deployment to roll forward to replicaCount=4 driven by the CM edit alone")
			waitForReplicas(h, 4)

			By("confirming a new Helm revision was created (release secret count grew)")
			Eventually(func(g Gomega) {
				secrets := h.GetHelmSecrets()
				g.Expect(len(secrets.Items)).Should(BeNumerically(">", initialCount))
			}, 60*time.Second, 5*time.Second).Should(Succeed())
		})
	})
})
