/*
Copyright 2025 The KubeVela Authors.

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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

	"github.com/kubevela/pkg/util/rand"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

type helmTestContext struct {
	ctx          context.Context
	namespace    string
	appNamespace string
	app          *v1beta1.Application
	appKey       client.ObjectKey
}

func newHelmTestContext() *helmTestContext {
	return &helmTestContext{
		ctx:          context.Background(),
		namespace:    "helm-e2e-" + rand.RandomString(4),
		appNamespace: "default",
	}
}

func (h *helmTestContext) createNamespace() {
	By("Creating target namespace for Helm release: " + h.namespace)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: h.namespace}}
	Expect(k8sClient.Create(h.ctx, ns)).Should(SatisfyAny(Succeed(), Not(HaveOccurred())))
}

func (h *helmTestContext) cleanup() {
	By("Deleting Application if it exists")
	if h.app != nil {
		_ = k8sClient.Delete(h.ctx, h.app)
		Eventually(func() bool {
			err := k8sClient.Get(h.ctx, h.appKey, &v1beta1.Application{})
			return err != nil
		}, 60*time.Second, 2*time.Second).Should(BeTrue())
	}
	By("Deleting target namespace")
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: h.namespace}}
	_ = k8sClient.Delete(h.ctx, ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
}

func (h *helmTestContext) cleanupNamespaceOnly() {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: h.namespace}}
	_ = k8sClient.Delete(h.ctx, ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
}

func (h *helmTestContext) deployApp() {
	h.deployAppFrom("testdata/helm/app_helmchart_podinfo.yaml")
}

func (h *helmTestContext) deployAppFrom(yamlPath string) {
	By("Deploying helmchart Application from " + yamlPath)
	raw, err := os.ReadFile(yamlPath)
	Expect(err).Should(BeNil())
	raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.namespace))
	h.app = &v1beta1.Application{}
	Expect(yaml.Unmarshal(raw, h.app)).Should(BeNil())
	h.app.SetNamespace(h.appNamespace)
	h.app.SetName("podinfo-helm-test-" + rand.RandomString(4))
	Expect(k8sClient.Create(h.ctx, h.app)).Should(Succeed())
	h.appKey = client.ObjectKeyFromObject(h.app)
	By("Waiting for Application to reach running state")
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
		g.Expect(h.app.Status.Phase).Should(Equal(common2.ApplicationRunning))
	}, 120*time.Second, 3*time.Second).Should(Succeed())
}

func (h *helmTestContext) waitForDeploymentReady() {
	By("Waiting for Deployment to be ready with 2 replicas")
	Eventually(func(g Gomega) {
		deploy := &appsv1.Deployment{}
		g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{
			Namespace: h.namespace, Name: "podinfo",
		}, deploy)).Should(Succeed())
		g.Expect(deploy.Status.ReadyReplicas).Should(Equal(int32(2)))
	}, 120*time.Second, 3*time.Second).Should(Succeed())
}

func (h *helmTestContext) getHelmSecrets() *corev1.SecretList {
	secrets := &corev1.SecretList{}
	Expect(k8sClient.List(h.ctx, secrets,
		client.InNamespace(h.namespace),
		client.MatchingLabels{"owner": "helm", "name": "podinfo"},
	)).Should(Succeed())
	return secrets
}

func (h *helmTestContext) waitForAppRunning() {
	By("Waiting for Application to return to running state")
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
		g.Expect(h.app.Status.Phase).Should(Equal(common2.ApplicationRunning))
	}, 180*time.Second, 3*time.Second).Should(Succeed())
}

func (h *helmTestContext) latestHelmSecretName() string {
	secrets := h.getHelmSecrets()
	var latest string
	for _, s := range secrets.Items {
		if latest == "" || s.Name > latest {
			latest = s.Name
		}
	}
	return latest
}

func (h *helmTestContext) updateAppValues(values map[string]interface{}) {
	By("Updating Application values to trigger upgrade")
	Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
	raw, err := json.Marshal(h.app.Spec.Components[0].Properties)
	Expect(err).Should(BeNil())
	var props map[string]interface{}
	Expect(json.Unmarshal(raw, &props)).Should(BeNil())
	if existing, ok := props["values"].(map[string]interface{}); ok {
		for k, v := range values {
			existing[k] = v
		}
		props["values"] = existing
	} else {
		props["values"] = values
	}
	newRaw, err := json.Marshal(props)
	Expect(err).Should(BeNil())
	h.app.Spec.Components[0].Properties = &runtime.RawExtension{Raw: newRaw}
	Expect(k8sClient.Update(h.ctx, h.app)).Should(Succeed())
	h.waitForAppRunning()
}

func (h *helmTestContext) recordPodUIDs() map[types.UID]bool {
	podList := &corev1.PodList{}
	Expect(k8sClient.List(h.ctx, podList,
		client.InNamespace(h.namespace),
		client.MatchingLabels{"app.kubernetes.io/name": "podinfo"},
	)).Should(Succeed())
	uids := make(map[types.UID]bool)
	for _, pod := range podList.Items {
		uids[pod.UID] = true
	}
	return uids
}

func (h *helmTestContext) countSurvivingPods(originalUIDs map[types.UID]bool) int {
	podList := &corev1.PodList{}
	Expect(k8sClient.List(h.ctx, podList,
		client.InNamespace(h.namespace),
		client.MatchingLabels{"app.kubernetes.io/name": "podinfo"},
	)).Should(Succeed())
	count := 0
	for _, pod := range podList.Items {
		if originalUIDs[pod.UID] {
			count++
		}
	}
	return count
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	GinkgoWriter.Printf("$ %s %v\n%s\n", name, args, string(out))
	return string(out), err
}

func runCommandSucceed(name string, args ...string) string {
	out, err := runCommand(name, args...)
	Expect(err).Should(Succeed(), "command failed: %s %v\noutput: %s", name, args, out)
	return out
}

// ============================================================================
// Self-Healing Scenarios
// ============================================================================

var _ = Describe("Helmchart Self-Healing", func() {

	Context("Scenario 1: Delete a Single Managed Resource (Deployment)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should deploy podinfo successfully", func() {
			h.deployApp()
			h.waitForDeploymentReady()
			By("Verifying Helm release secret exists")
			Expect(len(h.getHelmSecrets().Items)).Should(BeNumerically(">=", 1))
		})

		It("should recover when the Deployment is deleted via kubectl", func() {
			initialCount := len(h.getHelmSecrets().Items)
			latestSecret := h.latestHelmSecretName()

			By("Deleting the Deployment via kubectl")
			runCommandSucceed("kubectl", "delete", "deployment", "podinfo", "-n", h.namespace)

			By("Verifying Deployment is gone")
			Eventually(func() bool {
				err := k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, &appsv1.Deployment{})
				return err != nil
			}, 10*time.Second, time.Second).Should(BeTrue())

			By("Triggering reconciliation")
			RequestReconcileNow(h.ctx, h.app)

			By("Verifying KubeVela recreates the Deployment")
			h.waitForDeploymentReady()

			By("Verifying Helm revision did NOT increment (recovery is via ResourceTracker)")
			Expect(len(h.getHelmSecrets().Items)).Should(Equal(initialCount))
			Expect(h.latestHelmSecretName()).Should(Equal(latestSecret))

			By("Verifying pods are back to desired replica count")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			Expect(*deploy.Spec.Replicas).Should(Equal(int32(2)))
		})
	})

	Context("Scenario 2: Helm Uninstall the Release", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should deploy podinfo successfully", func() {
			h.deployApp()
			h.waitForDeploymentReady()
			out := runCommandSucceed("helm", "list", "-n", h.namespace, "-q")
			Expect(out).Should(ContainSubstring("podinfo"))
		})

		It("should recover after external helm uninstall", func() {
			By("Running helm uninstall podinfo externally")
			runCommandSucceed("helm", "uninstall", "podinfo", "-n", h.namespace)

			By("Verifying helm list no longer shows the release")
			Eventually(func() string {
				out, _ := runCommand("helm", "list", "-n", h.namespace, "-q")
				return out
			}, 15*time.Second, time.Second).ShouldNot(ContainSubstring("podinfo"))

			By("Verifying Deployment is gone after uninstall")
			Eventually(func() bool {
				err := k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, &appsv1.Deployment{})
				return err != nil
			}, 30*time.Second, 2*time.Second).Should(BeTrue())

			By("Triggering reconciliation")
			RequestReconcileNow(h.ctx, h.app)

			By("Verifying KubeVela performs fresh helm install")
			h.waitForDeploymentReady()

			By("Verifying helm list shows the release again")
			Eventually(func() string {
				out, _ := runCommand("helm", "list", "-n", h.namespace, "-q")
				return out
			}, 60*time.Second, 3*time.Second).Should(ContainSubstring("podinfo"))

			h.waitForAppRunning()
		})
	})

	Context("Scenario 3: Delete ONLY the Helm Release Secret", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should deploy podinfo successfully", func() {
			h.deployApp()
			h.waitForDeploymentReady()
		})

		It("should recover release secrets without affecting running pods", func() {
			originalPodUIDs := h.recordPodUIDs()
			Expect(len(originalPodUIDs)).Should(BeNumerically(">=", 2))

			By("Deleting all Helm release secrets via kubectl")
			runCommandSucceed("kubectl", "delete", "secrets", "-l", "owner=helm,name=podinfo", "-n", h.namespace)

			By("Verifying Helm release secrets are gone")
			Eventually(func() int {
				return len(h.getHelmSecrets().Items)
			}, 15*time.Second, time.Second).Should(Equal(0))

			By("Verifying running pods are NOT affected")
			Consistently(func() int {
				pods := &corev1.PodList{}
				Expect(k8sClient.List(h.ctx, pods, client.InNamespace(h.namespace),
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
			RequestReconcileNow(h.ctx, h.app)

			By("Verifying KubeVela restores Helm release secrets")
			Eventually(func() int {
				return len(h.getHelmSecrets().Items)
			}, 120*time.Second, 3*time.Second).Should(BeNumerically(">=", 1))

			By("Verifying helm list shows the release again")
			Eventually(func() string {
				out, _ := runCommand("helm", "list", "-n", h.namespace, "-q")
				return out
			}, 60*time.Second, 3*time.Second).Should(ContainSubstring("podinfo"))

			By("Verifying original pods still exist (not restarted)")
			Expect(h.countSurvivingPods(originalPodUIDs)).Should(BeNumerically(">=", 2))

			h.waitForAppRunning()
		})
	})

	Context("Scenario 4: Mutate a Managed Resource (Scale Deployment)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should deploy podinfo with replicaCount=2", func() {
			h.deployApp()
			h.waitForDeploymentReady()
		})

		It("should revert manual scaling back to 2 replicas", func() {
			By("Scaling Deployment to 5 replicas via kubectl scale")
			runCommandSucceed("kubectl", "scale", "deployment", "podinfo", "--replicas=5", "-n", h.namespace)

			By("Verifying Deployment scaled to 5")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(*d.Spec.Replicas).Should(Equal(int32(5)))
			}, 10*time.Second, time.Second).Should(Succeed())

			By("Triggering force reconcile via annotation")
			RequestReconcileNow(h.ctx, h.app)

			By("Verifying KubeVela reverts replicas to 2")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(*d.Spec.Replicas).Should(Equal(int32(2)))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			h.waitForDeploymentReady()
		})
	})

	Context("Scenario 5: Add Extra Annotation/Label", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should deploy podinfo successfully", func() {
			h.deployApp()
			h.waitForDeploymentReady()
		})

		It("should preserve user-added annotations and labels after reconciliation", func() {
			By("Adding custom annotation via kubectl annotate")
			runCommandSucceed("kubectl", "annotate", "deployment", "podinfo", "custom.io/test=test-value", "-n", h.namespace)
			By("Adding custom label via kubectl label")
			runCommandSucceed("kubectl", "label", "deployment", "podinfo", "extra.io/label=extra-value", "-n", h.namespace)

			By("Waiting 2+ reconcile cycles")
			time.Sleep(10 * time.Second)

			By("Triggering reconciliation")
			RequestReconcileNow(h.ctx, h.app)
			h.waitForAppRunning()

			By("Verifying annotation and label are preserved (3-way merge)")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(d.GetAnnotations()).Should(HaveKeyWithValue("custom.io/test", "test-value"))
				g.Expect(d.GetLabels()).Should(HaveKeyWithValue("extra.io/label", "extra-value"))
			}, 30*time.Second, 3*time.Second).Should(Succeed())
		})
	})

	Context("Scenario 6: Delete the Application CR", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanupNamespaceOnly() })

		It("should deploy podinfo and perform 3 upgrades", func() {
			h.deployApp()
			h.waitForDeploymentReady()
			h.updateAppValues(map[string]interface{}{"ui": map[string]interface{}{"message": "upgrade-1"}})
			h.updateAppValues(map[string]interface{}{"ui": map[string]interface{}{"message": "upgrade-2"}})
			h.updateAppValues(map[string]interface{}{"ui": map[string]interface{}{"message": "upgrade-3"}})
			Expect(len(h.getHelmSecrets().Items)).Should(BeNumerically(">=", 1))
		})

		It("should clean up all resources when Application is deleted", func() {
			appName := h.app.Name
			By("Deleting Application via kubectl")
			runCommandSucceed("kubectl", "delete", "application", appName, "-n", h.appNamespace)

			By("Verifying Application is gone")
			Eventually(func() bool {
				return k8sClient.Get(h.ctx, h.appKey, &v1beta1.Application{}) != nil
			}, 60*time.Second, 2*time.Second).Should(BeTrue())
			h.app = nil

			By("Verifying ALL Helm release secrets are deleted")
			Eventually(func() int { return len(h.getHelmSecrets().Items) }, 60*time.Second, 3*time.Second).Should(Equal(0))

			By("Verifying helm list shows empty")
			Eventually(func() string {
				out, _ := runCommand("helm", "list", "-n", h.namespace, "-q")
				return strings.TrimSpace(out)
			}, 30*time.Second, 3*time.Second).Should(BeEmpty())

			By("Verifying Deployment and Service are deleted")
			Eventually(func() bool {
				return k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, &appsv1.Deployment{}) != nil
			}, 30*time.Second, 2*time.Second).Should(BeTrue())
			Eventually(func() bool {
				return k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, &corev1.Service{}) != nil
			}, 30*time.Second, 2*time.Second).Should(BeTrue())

			By("Verifying ResourceTracker is deleted")
			Eventually(func() bool {
				rtList := &v1beta1.ResourceTrackerList{}
				Expect(k8sClient.List(h.ctx, rtList, client.MatchingLabels{"app.oam.dev/name": appName})).Should(Succeed())
				return len(rtList.Items) == 0
			}, 30*time.Second, 2*time.Second).Should(BeTrue())
		})
	})

	Context("Scenario 7: Delete a Non-Deployment Resource (Service)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should deploy podinfo successfully", func() {
			h.deployApp()
			h.waitForDeploymentReady()
		})

		It("should recover when the Service is deleted via kubectl", func() {
			originalPodUIDs := h.recordPodUIDs()

			By("Deleting the Service via kubectl")
			runCommandSucceed("kubectl", "delete", "svc", "podinfo", "-n", h.namespace)

			By("Triggering reconciliation")
			RequestReconcileNow(h.ctx, h.app)

			By("Verifying KubeVela recreates the Service with a new ClusterIP")
			Eventually(func(g Gomega) {
				svc := &corev1.Service{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, svc)).Should(Succeed())
				g.Expect(svc.Spec.ClusterIP).ShouldNot(BeEmpty())
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying pods are NOT affected")
			Expect(h.countSurvivingPods(originalPodUIDs)).Should(BeNumerically(">=", 2))
		})
	})

	Context("Scenario 8: Delete the Namespace", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should deploy podinfo successfully", func() {
			h.deployApp()
			h.waitForDeploymentReady()
		})

		It("should recover after namespace deletion", func() {
			By("Deleting the target namespace via kubectl")
			runCommandSucceed("kubectl", "delete", "namespace", h.namespace, "--wait=false")

			By("Waiting for namespace to be fully deleted")
			Eventually(func() bool {
				return k8sClient.Get(h.ctx, types.NamespacedName{Name: h.namespace}, &corev1.Namespace{}) != nil
			}, 120*time.Second, 3*time.Second).Should(BeTrue())

			By("Verifying Application CR survives (it is in default namespace)")
			Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())

			By("Triggering reconciliation")
			RequestReconcileNow(h.ctx, h.app)

			By("Verifying namespace is recreated (via createNamespace: true)")
			Eventually(func(g Gomega) {
				ns := &corev1.Namespace{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Name: h.namespace}, ns)).Should(Succeed())
				g.Expect(ns.Status.Phase).Should(Equal(corev1.NamespaceActive))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying all resources and Helm release are restored")
			h.waitForDeploymentReady()
			Eventually(func() string {
				out, _ := runCommand("helm", "list", "-n", h.namespace, "-q")
				return out
			}, 60*time.Second, 3*time.Second).Should(ContainSubstring("podinfo"))
			h.waitForAppRunning()
		})
	})

	Context("Scenario 9: Corrupt the Helm Release Secret", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should deploy podinfo successfully", func() {
			h.deployApp()
			h.waitForDeploymentReady()
		})

		It("should recover from corrupted Helm release secret", func() {
			originalPodUIDs := h.recordPodUIDs()
			latestSecret := h.latestHelmSecretName()
			Expect(latestSecret).ShouldNot(BeEmpty())

			By("Corrupting the release secret via kubectl patch")
			runCommandSucceed("kubectl", "patch", "secret", latestSecret, "-n", h.namespace,
				"--type=json", `-p=[{"op":"replace","path":"/data/release","value":"Y29ycnVwdGVk"}]`)

			By("Triggering reconciliation")
			RequestReconcileNow(h.ctx, h.app)

			By("Verifying fresh helm install succeeds")
			Eventually(func() string {
				out, _ := runCommand("helm", "list", "-n", h.namespace, "-q")
				return out
			}, 120*time.Second, 3*time.Second).Should(ContainSubstring("podinfo"))

			h.waitForDeploymentReady()
			h.waitForAppRunning()

			By("Verifying pods are unaffected during recovery")
			Expect(h.countSurvivingPods(originalPodUIDs)).Should(BeNumerically(">=", 2))
		})
	})
})

// ============================================================================
// Adoption & Takeover Scenarios
// ============================================================================

var _ = Describe("Helmchart Adoption & Takeover", func() {

	Context("Scenario 10: Adopt an Existing Vanilla Helm Release", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should adopt a pre-existing Helm release", func() {
			By("Installing podinfo via helm install directly (no KubeVela)")
			runCommandSucceed("helm", "install", "podinfo",
				"--repo", "https://stefanprodan.github.io/podinfo", "podinfo",
				"--version", "6.11.1", "--set", "replicaCount=2", "-n", h.namespace)

			initialSecretCount := len(h.getHelmSecrets().Items)

			By("Recording running pod UIDs before adoption")
			var podList corev1.PodList
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.List(h.ctx, &podList, client.InNamespace(h.namespace),
					client.MatchingLabels{"app.kubernetes.io/name": "podinfo"})).Should(Succeed())
				g.Expect(len(podList.Items)).Should(BeNumerically(">=", 2))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
			originalPodUIDs := make(map[types.UID]bool)
			for _, pod := range podList.Items {
				originalPodUIDs[pod.UID] = true
			}

			By("Applying a KubeVela Application with same release name, chart, and values")
			h.deployApp()
			h.waitForAppRunning()

			By("Verifying Helm revision increments by 1 (forced upgrade to inject KubeVela labels)")
			Expect(len(h.getHelmSecrets().Items)).Should(Equal(initialSecretCount + 1))

			By("Verifying pods are NOT restarted (zero downtime adoption)")
			Expect(h.countSurvivingPods(originalPodUIDs)).Should(BeNumerically(">=", 2))

			By("Verifying app.oam.dev/* labels appear on Deployment")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			Expect(deploy.GetLabels()).Should(HaveKey("app.oam.dev/name"))

			By("Verifying meta.helm.sh/release-name annotation is preserved")
			Expect(deploy.GetAnnotations()).Should(HaveKeyWithValue("meta.helm.sh/release-name", "podinfo"))
		})
	})

	Context("Scenario 11: Adopt Release with Different Values", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should adopt and upgrade a release with different values", func() {
			By("Installing podinfo via helm install with replicaCount=1")
			runCommandSucceed("helm", "install", "podinfo",
				"--repo", "https://stefanprodan.github.io/podinfo", "podinfo",
				"--version", "6.11.1", "--set", "replicaCount=1", "-n", h.namespace)

			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(Equal(int32(1)))
			}, 60*time.Second, 3*time.Second).Should(Succeed())

			initialSecretCount := len(h.getHelmSecrets().Items)

			By("Applying KubeVela Application with replicaCount=2")
			h.deployApp()

			By("Verifying Helm upgrade occurs (fingerprint differs)")
			Expect(len(h.getHelmSecrets().Items)).Should(BeNumerically(">", initialSecretCount))

			By("Verifying replicas scale to 2")
			h.waitForDeploymentReady()

			By("Verifying KubeVela labels injected")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			Expect(deploy.GetLabels()).Should(HaveKey("app.oam.dev/name"))
		})
	})

	Context("Scenario 12: Re-adopt After Application Deletion", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanupNamespaceOnly() })

		It("should re-adopt seamlessly after deletion and reinstall", func() {
			By("Installing podinfo via helm install")
			runCommandSucceed("helm", "install", "podinfo",
				"--repo", "https://stefanprodan.github.io/podinfo", "podinfo",
				"--version", "6.11.1", "--set", "replicaCount=2", "-n", h.namespace)

			By("Applying KubeVela Application (adopts the release)")
			h.deployApp()
			h.waitForAppRunning()

			By("Deleting the Application (GC cleans up everything)")
			runCommandSucceed("kubectl", "delete", "application", h.app.Name, "-n", h.appNamespace)
			Eventually(func() bool {
				return k8sClient.Get(h.ctx, h.appKey, &v1beta1.Application{}) != nil
			}, 60*time.Second, 2*time.Second).Should(BeTrue())
			h.app = nil

			By("Waiting for GC to clean up resources")
			Eventually(func() int { return len(h.getHelmSecrets().Items) }, 60*time.Second, 3*time.Second).Should(Equal(0))

			By("Installing podinfo via helm install again")
			runCommandSucceed("helm", "install", "podinfo",
				"--repo", "https://stefanprodan.github.io/podinfo", "podinfo",
				"--version", "6.11.1", "--set", "replicaCount=2", "-n", h.namespace)

			By("Applying the same KubeVela Application again")
			h.deployApp()
			h.waitForAppRunning()
			h.waitForDeploymentReady()
		})
	})
})

// ============================================================================
// Helm State Integrity Scenarios
// ============================================================================

var _ = Describe("Helmchart State Integrity", func() {

	Context("Scenario 13: Upgrade History Preserved Across Multiple Changes", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should preserve upgrade history across 5 changes", func() {
			h.deployApp()
			h.waitForDeploymentReady()

			for i := 1; i <= 5; i++ {
				h.updateAppValues(map[string]interface{}{"ui": map[string]interface{}{"message": fmt.Sprintf("upgrade-%d", i)}})
			}

			By("Verifying helm history shows multiple revisions")
			out := runCommandSucceed("helm", "history", "podinfo", "-n", h.namespace, "--output", "json")
			var history []map[string]interface{}
			Expect(json.Unmarshal([]byte(out), &history)).Should(Succeed())
			Expect(len(history)).Should(BeNumerically(">=", 2))

			Expect(len(h.getHelmSecrets().Items)).Should(BeNumerically(">=", 2))
		})
	})
})

// ============================================================================
// Destructive & Chaos Scenarios
// ============================================================================

var _ = Describe("Helmchart Destructive & Chaos", func() {

	Context("Scenario 14: Two Applications Targeting Same Release Name", Ordered, func() {
		h := newHelmTestContext()
		var appB *v1beta1.Application
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() {
			if appB != nil {
				_ = k8sClient.Delete(h.ctx, appB)
			}
			h.cleanup()
		})

		It("should detect ownership conflict", func() {
			h.deployApp()
			h.waitForDeploymentReady()
			h.waitForAppRunning()

			By("Deploying second Application also targeting release podinfo")
			raw, err := os.ReadFile("testdata/helm/app_helmchart_podinfo.yaml")
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.namespace))
			appB = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, appB)).Should(BeNil())
			appB.SetNamespace(h.appNamespace)
			appB.SetName("podinfo-conflict-" + rand.RandomString(4))
			Expect(k8sClient.Create(h.ctx, appB)).Should(Succeed())

			time.Sleep(15 * time.Second)
			RequestReconcileNow(h.ctx, h.app)

			By("Verifying the first application remains healthy")
			h.waitForAppRunning()
			h.waitForDeploymentReady()
		})
	})
})

// ============================================================================
// Resource Ordering Scenarios
// ============================================================================

var _ = Describe("Helmchart Resource Ordering", func() {

	Context("Scenario 15: Chart with CRDs", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanupNamespaceOnly() })

		It("should deploy a chart with CRDs and clean up on deletion", func() {
			h.deployApp()
			h.waitForDeploymentReady()
			h.waitForAppRunning()

			By("Deleting the Application")
			runCommandSucceed("kubectl", "delete", "application", h.app.Name, "-n", h.appNamespace)
			Eventually(func() bool {
				return k8sClient.Get(h.ctx, h.appKey, &v1beta1.Application{}) != nil
			}, 60*time.Second, 2*time.Second).Should(BeTrue())
			h.app = nil

			By("Verifying all resources are cleaned up")
			Eventually(func() bool {
				return k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, &appsv1.Deployment{}) != nil
			}, 60*time.Second, 3*time.Second).Should(BeTrue())
		})
	})

	Context("Scenario 16: Chart with Namespaces (createNamespace)", Ordered, func() {
		h := newHelmTestContext()
		AfterAll(func() { h.cleanup() })

		It("should create namespace before deploying namespace-scoped resources", func() {
			By("Verifying target namespace does not exist yet")
			err := k8sClient.Get(h.ctx, types.NamespacedName{Name: h.namespace}, &corev1.Namespace{})
			Expect(err).ShouldNot(BeNil())

			h.deployApp()

			By("Verifying namespace was created")
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Name: h.namespace}, &corev1.Namespace{})).Should(Succeed())
			h.waitForDeploymentReady()
		})
	})
})

// ============================================================================
// Health Check Scenarios
// ============================================================================

var _ = Describe("Helmchart Health Checks", func() {

	Context("Scenario 17: Custom Health Check — Deployment Available", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should report healthy when deployment is available", func() {
			h.deployAppFrom("testdata/helm/app_helmchart_podinfo_health.yaml")
			h.waitForDeploymentReady()
			Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
			Expect(h.app.Status.Phase).Should(Equal(common2.ApplicationRunning))
		})

		It("should revert when deployment is scaled to 0", func() {
			runCommandSucceed("kubectl", "scale", "deployment", "podinfo", "--replicas=0", "-n", h.namespace)
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(Equal(int32(0)))
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			RequestReconcileNow(h.ctx, h.app)
			h.waitForDeploymentReady()
			h.waitForAppRunning()
		})
	})

	Context("Scenario 18: Custom Health Check — Multiple Criteria", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should deploy with multiple health criteria", func() {
			h.deployAppFrom("testdata/helm/app_helmchart_podinfo_multi_health.yaml")
			h.waitForDeploymentReady()
			h.waitForAppRunning()
		})

		It("should recover when Service is deleted", func() {
			runCommandSucceed("kubectl", "delete", "svc", "podinfo", "-n", h.namespace)
			RequestReconcileNow(h.ctx, h.app)

			Eventually(func(g Gomega) {
				svc := &corev1.Service{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, svc)).Should(Succeed())
			}, 120*time.Second, 3*time.Second).Should(Succeed())
			h.waitForAppRunning()
		})
	})

	Context("Scenario 19: No Health Check Defined", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should default to healthy when no healthStatus field", func() {
			h.deployAppFrom("testdata/helm/app_helmchart_podinfo_no_values.yaml")
			h.waitForAppRunning()
		})
	})
})

// ============================================================================
// Edge Cases & Boundary Conditions
// ============================================================================

var _ = Describe("Helmchart Edge Cases", func() {

	Context("Scenario 20: Empty Values", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should install chart with default values when no values field", func() {
			h.deployAppFrom("testdata/helm/app_helmchart_podinfo_no_values.yaml")
			h.waitForAppRunning()

			deploy := &appsv1.Deployment{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, deploy)).Should(Succeed())
				g.Expect(deploy.Status.ReadyReplicas).Should(BeNumerically(">=", 1))
			}, 120*time.Second, 3*time.Second).Should(Succeed())
		})
	})

	Context("Scenario 21: Namespace Does Not Exist and createNamespace=false", Ordered, func() {
		h := &helmTestContext{
			ctx:          context.Background(),
			namespace:    "nonexistent-ns-" + rand.RandomString(4),
			appNamespace: "default",
		}
		AfterAll(func() {
			if h.app != nil {
				_ = k8sClient.Delete(h.ctx, h.app)
			}
		})

		It("should fail when namespace does not exist", func() {
			raw, err := os.ReadFile("testdata/helm/app_helmchart_podinfo_no_create_ns.yaml")
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.namespace))
			h.app = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, h.app)).Should(BeNil())
			h.app.SetNamespace(h.appNamespace)
			h.app.SetName("no-ns-test-" + rand.RandomString(4))
			Expect(k8sClient.Create(h.ctx, h.app)).Should(Succeed())
			h.appKey = client.ObjectKeyFromObject(h.app)

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
				g.Expect(h.app.Status.Phase).Should(SatisfyAny(
					Equal(common2.ApplicationWorkflowFailed),
					Equal(common2.ApplicationUnhealthy),
				))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying namespace was not created")
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Name: h.namespace}, &corev1.Namespace{})).ShouldNot(Succeed())
		})
	})

	Context("Scenario 22: Chart Not Found in Repository", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() {
			if h.app != nil {
				_ = k8sClient.Delete(h.ctx, h.app)
			}
			h.cleanupNamespaceOnly()
		})

		It("should fail with clear error for non-existent chart", func() {
			raw, err := os.ReadFile("testdata/helm/app_helmchart_bad_chart.yaml")
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.namespace))
			h.app = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, h.app)).Should(BeNil())
			h.app.SetNamespace(h.appNamespace)
			h.app.SetName("bad-chart-test-" + rand.RandomString(4))
			Expect(k8sClient.Create(h.ctx, h.app)).Should(Succeed())
			h.appKey = client.ObjectKeyFromObject(h.app)

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
				g.Expect(h.app.Status.Phase).Should(SatisfyAny(
					Equal(common2.ApplicationWorkflowFailed),
					Equal(common2.ApplicationUnhealthy),
				))
			}, 120*time.Second, 3*time.Second).Should(Succeed())
		})
	})

	Context("Scenario 23: Invalid Chart Version", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should fail with bad version then succeed with correct version", func() {
			raw, err := os.ReadFile("testdata/helm/app_helmchart_bad_version.yaml")
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.namespace))
			h.app = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, h.app)).Should(BeNil())
			h.app.SetNamespace(h.appNamespace)
			h.app.SetName("bad-version-test-" + rand.RandomString(4))
			Expect(k8sClient.Create(h.ctx, h.app)).Should(Succeed())
			h.appKey = client.ObjectKeyFromObject(h.app)

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
				g.Expect(h.app.Status.Phase).Should(SatisfyAny(
					Equal(common2.ApplicationWorkflowFailed),
					Equal(common2.ApplicationUnhealthy),
				))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("Updating to correct version")
			Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
			rawProps, err := json.Marshal(h.app.Spec.Components[0].Properties)
			Expect(err).Should(BeNil())
			var props map[string]interface{}
			Expect(json.Unmarshal(rawProps, &props)).Should(BeNil())
			chart := props["chart"].(map[string]interface{})
			chart["version"] = "6.11.1"
			props["chart"] = chart
			newRaw, err := json.Marshal(props)
			Expect(err).Should(BeNil())
			h.app.Spec.Components[0].Properties = &runtime.RawExtension{Raw: newRaw}
			Expect(k8sClient.Update(h.ctx, h.app)).Should(Succeed())

			h.waitForAppRunning()
		})
	})

	Context("Scenario 24: Two helmchart Components in Same Application", Ordered, func() {
		h := newHelmTestContext()
		nsA := "helm-multi-a-" + rand.RandomString(4)
		nsB := "helm-multi-b-" + rand.RandomString(4)
		BeforeAll(func() {
			for _, ns := range []string{nsA, nsB} {
				n := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
				Expect(k8sClient.Create(h.ctx, n)).Should(SatisfyAny(Succeed(), Not(HaveOccurred())))
			}
		})
		AfterAll(func() {
			if h.app != nil {
				_ = k8sClient.Delete(h.ctx, h.app)
				Eventually(func() bool {
					return k8sClient.Get(h.ctx, h.appKey, &v1beta1.Application{}) != nil
				}, 60*time.Second, 2*time.Second).Should(BeTrue())
			}
			for _, ns := range []string{nsA, nsB} {
				n := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
				_ = k8sClient.Delete(h.ctx, n, client.PropagationPolicy(metav1.DeletePropagationForeground))
			}
		})

		It("should manage podinfo and crossplane as independent Helm releases", func() {
			raw, err := os.ReadFile("testdata/helm/app_helmchart_two_components.yaml")
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns_a"), []byte(nsA))
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns_b"), []byte(nsB))
			h.app = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, h.app)).Should(BeNil())
			h.app.SetNamespace(h.appNamespace)
			h.app.SetName("two-comp-test-" + rand.RandomString(4))
			Expect(k8sClient.Create(h.ctx, h.app)).Should(Succeed())
			h.appKey = client.ObjectKeyFromObject(h.app)

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
				g.Expect(h.app.Status.Phase).Should(Equal(common2.ApplicationRunning))
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
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: nsA, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(Equal(int32(1)))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying crossplane Deployment is ready in namespace B")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: nsB, Name: "crossplane"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(BeNumerically(">=", 1))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying crossplane release secrets exist")
			cpSecrets := &corev1.SecretList{}
			Expect(k8sClient.List(h.ctx, cpSecrets,
				client.InNamespace(nsB),
				client.MatchingLabels{"owner": "helm", "name": "crossplane"},
			)).Should(Succeed())
			Expect(len(cpSecrets.Items)).Should(BeNumerically(">=", 1))
		})

		It("should clean up both releases when Application is deleted", func() {
			appName := h.app.Name
			runCommandSucceed("kubectl", "delete", "application", appName, "-n", h.appNamespace)
			Eventually(func() bool {
				return k8sClient.Get(h.ctx, h.appKey, &v1beta1.Application{}) != nil
			}, 60*time.Second, 2*time.Second).Should(BeTrue())
			h.app = nil

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
				Expect(k8sClient.List(h.ctx, cpSecrets,
					client.InNamespace(nsB),
					client.MatchingLabels{"owner": "helm", "name": "crossplane"},
				)).Should(Succeed())
				return len(cpSecrets.Items)
			}, 60*time.Second, 3*time.Second).Should(Equal(0))
		})
	})

	Context("Scenario 25: Helm Release Exists with Different Chart", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should detect chart mismatch and upgrade to new chart", func() {
			By("Installing podinfo v6.11.0 as release 'myrelease' via helm install")
			runCommandSucceed("helm", "install", "myrelease",
				"--repo", "https://stefanprodan.github.io/podinfo", "podinfo",
				"--version", "6.11.0", "--set", "replicaCount=1", "-n", h.namespace)

			Eventually(func() string {
				out, _ := runCommand("helm", "list", "-n", h.namespace, "-q")
				return out
			}, 30*time.Second, 3*time.Second).Should(ContainSubstring("myrelease"))

			By("Applying KubeVela Application with podinfo v6.11.1 targeting 'myrelease'")
			raw, err := os.ReadFile("testdata/helm/app_helmchart_podinfo.yaml")
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.namespace))
			raw = bytes.ReplaceAll(raw, []byte("name: podinfo\n          namespace"), []byte("name: myrelease\n          namespace"))
			h.app = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, h.app)).Should(BeNil())
			h.app.SetNamespace(h.appNamespace)
			h.app.SetName("chart-mismatch-" + rand.RandomString(4))
			Expect(k8sClient.Create(h.ctx, h.app)).Should(Succeed())
			h.appKey = client.ObjectKeyFromObject(h.app)

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
				g.Expect(h.app.Status.Phase).Should(Equal(common2.ApplicationRunning))
			}, 180*time.Second, 3*time.Second).Should(Succeed())

			deployList := &appsv1.DeploymentList{}
			Expect(k8sClient.List(h.ctx, deployList, client.InNamespace(h.namespace))).Should(Succeed())
			Expect(len(deployList.Items)).Should(BeNumerically(">=", 1))
			Expect(deployList.Items[0].GetLabels()).Should(HaveKey("app.oam.dev/name"))
		})
	})

	Context("Scenario 26: Apply Same Application Twice Without Changes", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should not trigger Helm upgrade when no changes", func() {
			h.deployApp()
			h.waitForDeploymentReady()
			h.waitForAppRunning()

			initialSecretCount := len(h.getHelmSecrets().Items)
			latestSecret := h.latestHelmSecretName()

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			initialResourceVersion := deploy.ResourceVersion

			By("Applying the exact same manifest via kubectl apply")
			raw, err := os.ReadFile("testdata/helm/app_helmchart_podinfo.yaml")
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.namespace))
			raw = bytes.ReplaceAll(raw, []byte("name: podinfo-helm-test"), []byte("name: "+h.app.Name))
			tmpFile := fmt.Sprintf("/tmp/helm-reapply-%s.yaml", rand.RandomString(4))
			Expect(os.WriteFile(tmpFile, raw, 0644)).Should(Succeed())
			defer os.Remove(tmpFile)
			runCommandSucceed("kubectl", "apply", "-f", tmpFile, "-n", h.appNamespace)

			time.Sleep(10 * time.Second)

			By("Verifying no Helm upgrade occurred")
			Expect(len(h.getHelmSecrets().Items)).Should(Equal(initialSecretCount))
			Expect(h.latestHelmSecretName()).Should(Equal(latestSecret))

			By("Verifying Deployment resourceVersion is stable")
			deploy = &appsv1.Deployment{}
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			Expect(deploy.ResourceVersion).Should(Equal(initialResourceVersion))
		})
	})
})

func init() {
	_ = fmt.Sprintf("helm chart tests registered")
}
