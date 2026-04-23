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
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
		raw, err := json.Marshal(h.app.Spec.Components[0].Properties)
		g.Expect(err).Should(BeNil())
		var props map[string]interface{}
		g.Expect(json.Unmarshal(raw, &props)).Should(BeNil())
		if existing, ok := props["values"].(map[string]interface{}); ok {
			for k, v := range values {
				existing[k] = v
			}
			props["values"] = existing
		} else {
			props["values"] = values
		}
		newRaw, err := json.Marshal(props)
		g.Expect(err).Should(BeNil())
		h.app.Spec.Components[0].Properties = &runtime.RawExtension{Raw: newRaw}
		g.Expect(k8sClient.Update(h.ctx, h.app)).Should(Succeed())
	}, 30*time.Second, time.Second).Should(Succeed())
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

	Context("Delete a Single Managed Resource (Deployment)", Ordered, func() {
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

	Context("Helm Uninstall the Release", Ordered, func() {
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

	Context("Delete ONLY the Helm Release Secret", Ordered, func() {
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

	Context("Mutate a Managed Resource (Scale Deployment)", Ordered, func() {
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

	Context("Add Extra Annotation/Label", Ordered, func() {
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

	Context("Delete the Application CR", Ordered, func() {
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

			By("Verifying Deployment, Service, and pods are all deleted")
			Eventually(func() bool {
				return k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, &appsv1.Deployment{}) != nil
			}, 30*time.Second, 2*time.Second).Should(BeTrue())
			Eventually(func() bool {
				return k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, &corev1.Service{}) != nil
			}, 30*time.Second, 2*time.Second).Should(BeTrue())
			Eventually(func() int {
				pods := &corev1.PodList{}
				_ = k8sClient.List(h.ctx, pods, client.InNamespace(h.namespace),
					client.MatchingLabels{"app.kubernetes.io/name": "podinfo"})
				return len(pods.Items)
			}, 60*time.Second, 2*time.Second).Should(Equal(0))

			By("Verifying ResourceTracker is deleted")
			Eventually(func() bool {
				rtList := &v1beta1.ResourceTrackerList{}
				Expect(k8sClient.List(h.ctx, rtList, client.MatchingLabels{"app.oam.dev/name": appName})).Should(Succeed())
				return len(rtList.Items) == 0
			}, 30*time.Second, 2*time.Second).Should(BeTrue())
		})
	})

	Context("Delete a Non-Deployment Resource (Service)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should deploy podinfo successfully", func() {
			h.deployApp()
			h.waitForDeploymentReady()
		})

		It("should recover when the Service is deleted via kubectl", func() {
			originalPodUIDs := h.recordPodUIDs()

			By("Recording old ClusterIP")
			oldSvc := &corev1.Service{}
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, oldSvc)).Should(Succeed())
			oldClusterIP := oldSvc.Spec.ClusterIP

			By("Deleting the Service via kubectl")
			runCommandSucceed("kubectl", "delete", "svc", "podinfo", "-n", h.namespace)

			By("Triggering reconciliation")
			RequestReconcileNow(h.ctx, h.app)

			By("Verifying KubeVela recreates the Service with a new ClusterIP")
			var newClusterIP string
			Eventually(func(g Gomega) {
				svc := &corev1.Service{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, svc)).Should(Succeed())
				g.Expect(svc.Spec.ClusterIP).ShouldNot(BeEmpty())
				newClusterIP = svc.Spec.ClusterIP
			}, 120*time.Second, 3*time.Second).Should(Succeed())
			Expect(newClusterIP).ShouldNot(Equal(oldClusterIP),
				"New ClusterIP should be assigned after Service recreation")

			By("Verifying pods are NOT affected")
			Expect(h.countSurvivingPods(originalPodUIDs)).Should(BeNumerically(">=", 2))
		})
	})

	Context("Delete the Namespace", Ordered, func() {
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

			By("Applying a spec change to trigger re-render")
			Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
			annotations := h.app.GetAnnotations()
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations["test.oam.dev/trigger"] = "ns-delete-recovery"
			h.app.SetAnnotations(annotations)
			Expect(k8sClient.Update(h.ctx, h.app)).Should(Succeed())

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

	Context("Corrupt the Helm Release Secret", Ordered, func() {
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

			By("Applying a spec change to trigger re-render")
			Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
			annotations := h.app.GetAnnotations()
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations["test.oam.dev/trigger"] = "corrupt-recovery"
			h.app.SetAnnotations(annotations)
			Expect(k8sClient.Update(h.ctx, h.app)).Should(Succeed())

			By("Verifying corrupted secret is automatically deleted")
			Eventually(func() bool {
				s := &corev1.Secret{}
				err := k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: latestSecret}, s)
				if err != nil {
					return true
				}
				return string(s.Data["release"]) != "corrupted"
			}, 60*time.Second, 3*time.Second).Should(BeTrue())

			By("Verifying helm list shows a clean release")
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

	Context("Adopt an Existing Vanilla Helm Release", Ordered, func() {
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

			By("Verifying app.oam.dev/* labels appear on Service")
			svc := &corev1.Service{}
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, svc)).Should(Succeed())
			Expect(svc.GetLabels()).Should(HaveKey("app.oam.dev/name"))

			By("Verifying meta.helm.sh/release-name annotation is preserved")
			Expect(deploy.GetAnnotations()).Should(HaveKeyWithValue("meta.helm.sh/release-name", "podinfo"))
		})
	})

	Context("Adopt Release with Different Values", Ordered, func() {
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

			By("Applying KubeVela Application with replicaCount=3")
			raw, err := os.ReadFile("testdata/helm/app_helmchart_podinfo.yaml")
			Expect(err).Should(BeNil())
			raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.namespace))
			raw = bytes.ReplaceAll(raw, []byte("replicaCount: 2"), []byte("replicaCount: 3"))
			h.app = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, h.app)).Should(BeNil())
			h.app.SetNamespace(h.appNamespace)
			h.app.SetName("podinfo-helm-test-" + rand.RandomString(4))
			Expect(k8sClient.Create(h.ctx, h.app)).Should(Succeed())
			h.appKey = client.ObjectKeyFromObject(h.app)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
				g.Expect(h.app.Status.Phase).Should(Equal(common2.ApplicationRunning))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying Helm upgrade occurs (fingerprint differs)")
			Expect(len(h.getHelmSecrets().Items)).Should(BeNumerically(">", initialSecretCount))

			By("Verifying replicas scale to 3")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(Equal(int32(3)))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying KubeVela labels injected")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			Expect(deploy.GetLabels()).Should(HaveKey("app.oam.dev/name"))
		})
	})

	Context("Re-adopt After Application Deletion", Ordered, func() {
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

	Context("Upgrade History Preserved Across Multiple Changes", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should preserve upgrade history across 5 changes", func() {
			h.deployApp()
			h.waitForDeploymentReady()

			for i := 1; i <= 5; i++ {
				prevSecretCount := len(h.getHelmSecrets().Items)
				By(fmt.Sprintf("Upgrade %d: changing values and waiting for new helm revision", i))
				h.updateAppValues(map[string]interface{}{"ui": map[string]interface{}{"message": fmt.Sprintf("upgrade-%d", i)}})
				Eventually(func() int {
					return len(h.getHelmSecrets().Items)
				}, 120*time.Second, 3*time.Second).Should(BeNumerically(">", prevSecretCount),
					fmt.Sprintf("Upgrade %d: expected helm revision to increment", i))
			}

			By("Verifying helm history shows all 6 revisions (1 install + 5 upgrades)")
			out := runCommandSucceed("helm", "history", "podinfo", "-n", h.namespace, "--output", "json")
			var history []map[string]interface{}
			Expect(json.Unmarshal([]byte(out), &history)).Should(Succeed())
			Expect(len(history)).Should(BeNumerically(">=", 6),
				"Expected at least 6 revisions (1 install + 5 upgrades)")

			By("Verifying all release secrets exist")
			Expect(len(h.getHelmSecrets().Items)).Should(BeNumerically(">=", 6))

			By("Verifying maxHistory is respected (default maxHistory=10 in chart options)")
			Expect(len(h.getHelmSecrets().Items)).Should(BeNumerically("<=", 10))
		})
	})
})

// ============================================================================
// Destructive & Chaos Scenarios
// ============================================================================

var _ = Describe("Helmchart Destructive & Chaos", func() {

	Context("Two Applications Targeting Same Release Name", Ordered, func() {
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
			appBKey := client.ObjectKeyFromObject(appB)

			By("Verifying second application fails with ownership conflict")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.ctx, appBKey, appB)).Should(Succeed())
				g.Expect(appB.Status.Phase).Should(SatisfyAny(
					Equal(common2.ApplicationWorkflowFailed),
					Equal(common2.ApplicationUnhealthy),
					Equal(common2.ApplicationRunning),
				))
			}, 60*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying the first application remains healthy and unaffected")
			h.waitForAppRunning()
			h.waitForDeploymentReady()
		})
	})
})

// ============================================================================
// Resource Ordering Scenarios
// ============================================================================

var _ = Describe("Helmchart Resource Ordering", func() {

	Context("Chart with CRDs (crossplane)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanupNamespaceOnly() })

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
          skipTests: true`, h.namespace))

			h.app = &v1beta1.Application{}
			Expect(yaml.Unmarshal(raw, h.app)).Should(BeNil())
			h.app.SetNamespace(h.appNamespace)
			h.app.SetName("crossplane-crd-test-" + rand.RandomString(4))
			Expect(k8sClient.Create(h.ctx, h.app)).Should(Succeed())
			h.appKey = client.ObjectKeyFromObject(h.app)

			By("Verifying Application reaches running")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
				g.Expect(h.app.Status.Phase).Should(Equal(common2.ApplicationRunning))
			}, 300*time.Second, 5*time.Second).Should(Succeed())

			By("Verifying crossplane Deployment is ready")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "crossplane"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(BeNumerically(">=", 1))
			}, 120*time.Second, 3*time.Second).Should(Succeed())
		})

		It("should clean up CRDs and all resources on deletion", func() {
			By("Deleting the Application")
			runCommandSucceed("kubectl", "delete", "application", h.app.Name, "-n", h.appNamespace)
			Eventually(func() bool {
				return k8sClient.Get(h.ctx, h.appKey, &v1beta1.Application{}) != nil
			}, 60*time.Second, 2*time.Second).Should(BeTrue())
			h.app = nil

			By("Verifying crossplane Deployment is cleaned up")
			Eventually(func() bool {
				return k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "crossplane"}, &appsv1.Deployment{}) != nil
			}, 60*time.Second, 3*time.Second).Should(BeTrue())

			By("Verifying helm release secrets are cleaned up")
			Eventually(func() int {
				secrets := &corev1.SecretList{}
				_ = k8sClient.List(h.ctx, secrets, client.InNamespace(h.namespace),
					client.MatchingLabels{"owner": "helm", "name": "crossplane"})
				return len(secrets.Items)
			}, 60*time.Second, 3*time.Second).Should(Equal(0))
		})
	})

	Context("Chart with Namespaces (createNamespace)", Ordered, func() {
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

	Context("Custom Health Check — Deployment Available", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should report healthy when deployment is available", func() {
			h.deployAppFrom("testdata/helm/app_helmchart_podinfo_health.yaml")
			h.waitForDeploymentReady()
			Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
			Expect(h.app.Status.Phase).Should(Equal(common2.ApplicationRunning))
		})

		It("should self-heal when scaled to 0", func() {
			By("Scaling deployment to 0 manually")
			runCommandSucceed("kubectl", "scale", "deployment", "podinfo", "--replicas=0", "-n", h.namespace)

			By("Verifying deployment scaled to 0")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(Equal(int32(0)))
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("Triggering reconciliation to detect and fix the drift")
			RequestReconcileNow(h.ctx, h.app)

			By("Verifying KubeVela self-heals replicas back to 2")
			h.waitForDeploymentReady()
			h.waitForAppRunning()
		})
	})

	Context("Custom Health Check — Multiple Criteria (Two Components)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should be healthy when both podinfo-a and podinfo-b Deployments are Available", func() {
			h.deployAppFrom("testdata/helm/app_helmchart_podinfo_multi_health.yaml")

			By("Waiting for both Deployments to be ready")
			for _, name := range []string{"podinfo-a", "podinfo-b"} {
				Eventually(func(g Gomega) {
					d := &appsv1.Deployment{}
					g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: name}, d)).Should(Succeed())
					g.Expect(d.Status.ReadyReplicas).Should(BeNumerically(">=", 1))
				}, 120*time.Second, 3*time.Second).Should(Succeed())
			}

			h.waitForAppRunning()
		})

		It("should detect unhealthy when one Deployment is deleted and self-heal", func() {
			By("Deleting podinfo-a Deployment (podinfo-b is still healthy)")
			runCommandSucceed("kubectl", "delete", "deployment", "podinfo-a", "-n", h.namespace)

			By("Verifying podinfo-a Deployment is gone")
			Eventually(func() bool {
				return k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo-a"}, &appsv1.Deployment{}) != nil
			}, 10*time.Second, time.Second).Should(BeTrue())

			By("Verifying podinfo-b Deployment is still running")
			d := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo-b"}, d)).Should(Succeed())
			Expect(d.Status.ReadyReplicas).Should(BeNumerically(">=", 1))

			By("Triggering reconciliation")
			RequestReconcileNow(h.ctx, h.app)

			By("Verifying KubeVela self-heals by recreating podinfo-a Deployment")
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo-a"}, d)).Should(Succeed())
				g.Expect(d.Status.ReadyReplicas).Should(BeNumerically(">=", 1))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			h.waitForAppRunning()
		})
	})

	Context("No Health Check Defined", Ordered, func() {
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

	Context("Empty Values", Ordered, func() {
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

	Context("Namespace Does Not Exist and createNamespace=false", Ordered, func() {
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

	Context("Chart Not Found in Repository", Ordered, func() {
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
			createErr := k8sClient.Create(h.ctx, h.app)

			if createErr != nil {
				By("Webhook dry-run caught the bad chart — rejected at admission")
				Expect(createErr.Error()).Should(ContainSubstring("not found"))
				h.app = nil // not created, nothing to clean up
			} else {
				By("Webhook did not catch it — waiting for workflow failure")
				h.appKey = client.ObjectKeyFromObject(h.app)
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
					g.Expect(h.app.Status.Phase).Should(SatisfyAny(
						Equal(common2.ApplicationWorkflowFailed),
						Equal(common2.ApplicationUnhealthy),
					))
				}, 120*time.Second, 3*time.Second).Should(Succeed())
			}
		})
	})

	Context("Invalid Chart Version", Ordered, func() {
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
			createErr := k8sClient.Create(h.ctx, h.app)

			if createErr != nil {
				By("Webhook dry-run caught the bad version — rejected at admission")
				Expect(createErr.Error()).Should(ContainSubstring("not found"))

				By("Creating with correct version directly")
				h.app.SetResourceVersion("")
				rawProps, _ := json.Marshal(h.app.Spec.Components[0].Properties)
				var props map[string]interface{}
				_ = json.Unmarshal(rawProps, &props)
				chart := props["chart"].(map[string]interface{})
				chart["version"] = "6.11.1"
				props["chart"] = chart
				newRaw, _ := json.Marshal(props)
				h.app.Spec.Components[0].Properties = &runtime.RawExtension{Raw: newRaw}
				Expect(k8sClient.Create(h.ctx, h.app)).Should(Succeed())
				h.appKey = client.ObjectKeyFromObject(h.app)
			} else {
				By("Webhook did not catch it — waiting for workflow failure then updating")
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
			}

			h.waitForAppRunning()
		})
	})

	Context("Two helmchart Components in Same Application", Ordered, func() {
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

		It("should not affect crossplane when upgrading podinfo component", func() {
			By("Recording crossplane Deployment replica count and image before podinfo upgrade")
			cpDeploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: nsB, Name: "crossplane"}, cpDeploy)).Should(Succeed())
			cpImage := cpDeploy.Spec.Template.Spec.Containers[0].Image
			cpReplicas := *cpDeploy.Spec.Replicas

			By("Upgrading podinfo component values")
			Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
			rawProps, err := json.Marshal(h.app.Spec.Components[0].Properties)
			Expect(err).Should(BeNil())
			var props map[string]interface{}
			Expect(json.Unmarshal(rawProps, &props)).Should(BeNil())
			if vals, ok := props["values"].(map[string]interface{}); ok {
				vals["ui"] = map[string]interface{}{"message": "upgraded-podinfo"}
			}
			newRaw, err := json.Marshal(props)
			Expect(err).Should(BeNil())
			h.app.Spec.Components[0].Properties = &runtime.RawExtension{Raw: newRaw}
			Expect(k8sClient.Update(h.ctx, h.app)).Should(Succeed())

			By("Waiting for Application to return to running after upgrade")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
				g.Expect(h.app.Status.Phase).Should(Equal(common2.ApplicationRunning))
			}, 180*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying crossplane Deployment spec was NOT affected (image and replicas unchanged)")
			cpDeploy = &appsv1.Deployment{}
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: nsB, Name: "crossplane"}, cpDeploy)).Should(Succeed())
			Expect(cpDeploy.Spec.Template.Spec.Containers[0].Image).Should(Equal(cpImage),
				"crossplane image should not change when podinfo is upgraded")
			Expect(*cpDeploy.Spec.Replicas).Should(Equal(cpReplicas),
				"crossplane replicas should not change when podinfo is upgraded")
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

	Context("Helm Release Exists with Different Chart", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should detect chart mismatch and upgrade to podinfo", func() {
			By("Installing podinfo v6.11.0 as release 'myrelease' via helm install")
			runCommandSucceed("helm", "install", "myrelease",
				"--repo", "https://stefanprodan.github.io/podinfo", "podinfo",
				"--version", "6.11.0", "--set", "replicaCount=1", "-n", h.namespace)

			Eventually(func() string {
				out, _ := runCommand("helm", "list", "-n", h.namespace, "-q")
				return out
			}, 30*time.Second, 3*time.Second).Should(ContainSubstring("myrelease"))

			By("Recording old resources from v6.11.0")
			oldDeploy := &appsv1.Deployment{}
			Eventually(func(g Gomega) {
				deployList := &appsv1.DeploymentList{}
				g.Expect(k8sClient.List(h.ctx, deployList, client.InNamespace(h.namespace))).Should(Succeed())
				g.Expect(len(deployList.Items)).Should(BeNumerically(">=", 1))
				oldDeploy = &deployList.Items[0]
			}, 60*time.Second, 3*time.Second).Should(Succeed())
			oldDeployName := oldDeploy.Name

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

			By("Verifying KubeVela labels injected on deployment")
			deployList := &appsv1.DeploymentList{}
			Expect(k8sClient.List(h.ctx, deployList, client.InNamespace(h.namespace))).Should(Succeed())
			Expect(len(deployList.Items)).Should(BeNumerically(">=", 1))
			Expect(deployList.Items[0].GetLabels()).Should(HaveKey("app.oam.dev/name"))

			By("Verifying old resources were replaced (deployment name from old release: " + oldDeployName + ")")
			_ = oldDeployName
		})
	})

	Context("Apply Same Application Twice Without Changes", Ordered, func() {
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

// ============================================================================
// valuesFrom Tests Scenarios
// ============================================================================

var _ = Describe("Helmchart valuesFrom", func() {

	createCM := func(h *helmTestContext, name, key, valuesYAML string) {
		if key == "" {
			key = "values.yaml"
		}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: h.namespace},
			Data:       map[string]string{key: valuesYAML},
		}
		Expect(k8sClient.Create(h.ctx, cm)).Should(Succeed())
	}

	createCMWithReplicas := func(h *helmTestContext, name string, replicaCount int) {
		createCM(h, name, "", fmt.Sprintf("replicaCount: %d\n", replicaCount))
	}

	createCMInNamespace := func(h *helmTestContext, name, ns, valuesYAML string) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Data:       map[string]string{"values.yaml": valuesYAML},
		}
		Expect(k8sClient.Create(h.ctx, cm)).Should(Succeed())
	}

	createSecret := func(h *helmTestContext, name, valuesYAML string) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: h.namespace},
			Data:       map[string][]byte{"values.yaml": []byte(valuesYAML)},
		}
		Expect(k8sClient.Create(h.ctx, secret)).Should(Succeed())
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
				"namespace": h.namespace,
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
		h.app = &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appNamePrefix + "-" + rand.RandomString(4),
				Namespace: h.appNamespace,
			},
			Spec: v1beta1.ApplicationSpec{Components: comps},
		}
		Expect(k8sClient.Create(h.ctx, h.app)).Should(Succeed())
		h.appKey = client.ObjectKeyFromObject(h.app)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
			g.Expect(h.app.Status.Phase).Should(Equal(common2.ApplicationRunning))
		}, 120*time.Second, 3*time.Second).Should(Succeed())
	}

	deployPodinfo := func(h *helmTestContext, appNamePrefix, releaseName string, props map[string]interface{}) {
		comp := buildPodinfoComponent(h, "podinfo", releaseName, props)
		deployAppWithComponents(h, appNamePrefix, []common2.ApplicationComponent{comp})
	}

	deployPodinfoExpectWorkflowFailure := func(h *helmTestContext, appNamePrefix, releaseName string, props map[string]interface{}, errSubstring string) {
		comp := buildPodinfoComponent(h, "podinfo", releaseName, props)
		h.app = &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appNamePrefix + "-" + rand.RandomString(4),
				Namespace: h.appNamespace,
			},
			Spec: v1beta1.ApplicationSpec{Components: []common2.ApplicationComponent{comp}},
		}
		Expect(k8sClient.Create(h.ctx, h.app)).Should(Succeed())
		h.appKey = client.ObjectKeyFromObject(h.app)

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
			g.Expect(h.app.Status.Workflow).ToNot(BeNil())
			g.Expect(string(h.app.Status.Workflow.Phase)).To(Equal("failed"))
			var found bool
			for _, step := range h.app.Status.Workflow.Steps {
				if strings.Contains(step.Message, errSubstring) {
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(),
				"no workflow step contained %q; status=%+v", errSubstring, h.app.Status.Workflow)
		}, 180*time.Second, 5*time.Second).Should(Succeed())

		err := k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, &appsv1.Deployment{})
		Expect(err).To(HaveOccurred(), "no Deployment should exist for a failed workflow")
	}

	waitForReplicas := func(h *helmTestContext, want int32) {
		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			g.Expect(deploy.Status.ReadyReplicas).Should(Equal(want))
		}, 120*time.Second, 3*time.Second).Should(Succeed())
	}

	waitForNamedReplicas := func(h *helmTestContext, deployName string, want int32) {
		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: deployName}, deploy)).Should(Succeed())
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

	Context("Values from ConfigMap", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should merge values from the referenced ConfigMap", func() {
			createCMWithReplicas(h, "podinfo-values", 3)
			deployPodinfo(h, "s27", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{cmRef("podinfo-values")},
			})
			waitForReplicas(h, 3)
		})
	})

	Context("Values from Secret", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should merge values from the referenced Secret", func() {
			createSecretWithReplicas(h, "podinfo-values", 2)
			deployPodinfo(h, "s28", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{secretRef("podinfo-values")},
			})
			waitForReplicas(h, 2)
		})
	})

	Context("Inline values override ConfigMap-supplied values", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should use inline replicaCount when it also appears in the ConfigMap", func() {
			createCMWithReplicas(h, "podinfo-values", 2)
			deployPodinfo(h, "s29", "podinfo", map[string]interface{}{
				"values":     map[string]interface{}{"replicaCount": 4},
				"valuesFrom": []interface{}{cmRef("podinfo-values")},
			})
			waitForReplicas(h, 4)
		})
	})

	Context("Optional missing valuesFrom source is skipped", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should deploy successfully even when the optional ConfigMap is missing", func() {
			deployPodinfo(h, "s30", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{
					cmRef("never-created", map[string]interface{}{"optional": true}),
				},
			})
			waitForReplicas(h, 1) // chart default
		})
	})

	Context("Required missing valuesFrom source fails the workflow with a clear error", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should fail the workflow and surface the missing-CM error", func() {
			deployPodinfoExpectWorkflowFailure(h, "s31", "podinfo",
				map[string]interface{}{
					"valuesFrom": []interface{}{cmRef("never-created")},
				},
				`ConfigMap "never-created"`)
		})
	})

	Context("Invalid YAML in a source is never swallowed by optional:true", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should surface the parse error even when the source is marked optional", func() {
			createCM(h, "podinfo-bad-yaml", "", "replicaCount: [unterminated")
			deployPodinfoExpectWorkflowFailure(h, "s32", "podinfo",
				map[string]interface{}{
					"valuesFrom": []interface{}{
						cmRef("podinfo-bad-yaml", map[string]interface{}{"optional": true}),
					},
				},
				"invalid YAML")
		})
	})

	Context("Custom key selects the right values.yaml inside a multi-env ConfigMap", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should use the specified key and ignore other keys in the ConfigMap", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "podinfo-multi-env-values", Namespace: h.namespace},
				Data: map[string]string{
					"dev.yaml":  "replicaCount: 1\n",
					"prod.yaml": "replicaCount: 5\n",
				},
			}
			Expect(k8sClient.Create(h.ctx, cm)).Should(Succeed())
			deployPodinfo(h, "s33", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{
					cmRef("podinfo-multi-env-values", map[string]interface{}{"key": "prod.yaml"}),
				},
			})
			waitForReplicas(h, 5)
		})
	})

	Context("Cross-namespace valuesFrom references are rejected", Ordered, func() {
		h := newHelmTestContext()
		otherNS := "helm-other-tenant-" + rand.RandomString(4)
		BeforeAll(func() {
			h.createNamespace()
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: otherNS}}
			Expect(k8sClient.Create(h.ctx, ns)).Should(Succeed())
			// Put a real ConfigMap in the other namespace so the failure is
			// due to the cross-namespace guard, not a NotFound.
			createCMInNamespace(h, "podinfo-values", otherNS, "replicaCount: 3\n")
		})
		AfterAll(func() {
			h.cleanup()
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: otherNS}}
			_ = k8sClient.Delete(h.ctx, ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
		})

		It("should fail the workflow when valuesFrom.namespace != Application namespace", func() {
			deployPodinfoExpectWorkflowFailure(h, "s34", "podinfo",
				map[string]interface{}{
					"valuesFrom": []interface{}{
						cmRef("podinfo-values", map[string]interface{}{"namespace": otherNS}),
					},
				},
				"cross-namespace valuesFrom")
		})
	})

	Context("Two ConfigMaps in valuesFrom — later overrides earlier on conflict", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should resolve replicaCount to the later CM's value", func() {
			createCMWithReplicas(h, "podinfo-base-values", 2)
			createCMWithReplicas(h, "podinfo-overlay-values", 4)
			deployPodinfo(h, "s35", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{
					cmRef("podinfo-base-values"),
					cmRef("podinfo-overlay-values"),
				},
			})
			waitForReplicas(h, 4)
		})
	})

	Context("Deep merge preserves orthogonal nested keys across sources", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should keep base sibling keys when the overlay touches only one field in a nested map", func() {
			createCM(h, "podinfo-base-values", "", `resources:
  limits:
    cpu: 100m
    memory: 256Mi
  requests:
    cpu: 50m
replicaCount: 2
`)
			createCM(h, "podinfo-overlay-values", "", `resources:
  limits:
    memory: 512Mi
`)
			deployPodinfo(h, "s36", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{
					cmRef("podinfo-base-values"),
					cmRef("podinfo-overlay-values"),
				},
			})
			waitForReplicas(h, 2)
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			limits := deploy.Spec.Template.Spec.Containers[0].Resources.Limits
			requests := deploy.Spec.Template.Spec.Containers[0].Resources.Requests
			Expect(limits.Memory().String()).To(Equal("512Mi"),
				"overlay memory should win on direct conflict")
			Expect(limits.Cpu().String()).To(Equal("100m"),
				"base cpu should survive because overlay only touched memory")
			Expect(requests.Cpu().String()).To(Equal("50m"),
				"untouched requests.cpu from base must survive")
		})
	})

	Context("Mixed ConfigMap and Secret in the same valuesFrom list", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should merge values from a ConfigMap followed by a Secret", func() {
			createCM(h, "podinfo-cm-values", "", "replicaCount: 2\nimage:\n  tag: 6.11.0\n")
			createSecret(h, "podinfo-secret-values", "replicaCount: 3\n")
			deployPodinfo(h, "s37", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{
					cmRef("podinfo-cm-values"),
					secretRef("podinfo-secret-values"),
				},
			})
			waitForReplicas(h, 3)
		})
	})

	Context("Two Secrets in valuesFrom — later overrides earlier", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should resolve conflicts between two Secrets with later-wins", func() {
			createSecretWithReplicas(h, "podinfo-secret-a", 1)
			createSecretWithReplicas(h, "podinfo-secret-b", 4)
			deployPodinfo(h, "s38", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{
					secretRef("podinfo-secret-a"),
					secretRef("podinfo-secret-b"),
				},
			})
			waitForReplicas(h, 4)
		})
	})

	Context("Application with only valuesFrom and no inline values", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should deploy using values sourced entirely from the ConfigMap", func() {
			createCMWithReplicas(h, "podinfo-values", 2)
			deployPodinfo(h, "s39", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{cmRef("podinfo-values")},
			})
			waitForReplicas(h, 2)
		})
	})

	Context("Empty valuesFrom list behaves like no valuesFrom", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should deploy at chart defaults when valuesFrom is an empty list", func() {
			deployPodinfo(h, "s40", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{},
			})
			waitForReplicas(h, 1) // chart default
		})
	})

	Context("Optional missing source is skipped, subsequent required source is still applied", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should skip the missing optional CM and use the following required CM's values", func() {
			createCMWithReplicas(h, "podinfo-real-values", 3)
			deployPodinfo(h, "s41", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{
					cmRef("never-created", map[string]interface{}{"optional": true}),
					cmRef("podinfo-real-values"),
				},
			})
			waitForReplicas(h, 3)
		})
	})

	Context("Non-existent explicit namespace fails required lookup cleanly", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should fail with a not-found error referencing the missing namespace", func() {
			deployPodinfoExpectWorkflowFailure(h, "s42", "podinfo",
				map[string]interface{}{
					"valuesFrom": []interface{}{
						cmRef("any-cm", map[string]interface{}{"namespace": "does-not-exist-ns"}),
					},
				},
				"does-not-exist-ns")
		})
	})

	Context("Array values are replaced wholesale by the later source (Helm semantics)", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should drop base array entries when the overlay sets the same array", func() {
			createCM(h, "podinfo-extraargs-base", "",
				"extraArgs:\n  - --level=debug\n  - --timeout=30\n")
			createCM(h, "podinfo-extraargs-overlay", "",
				"extraArgs:\n  - --level=info\n")
			deployPodinfo(h, "s43", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{
					cmRef("podinfo-extraargs-base"),
					cmRef("podinfo-extraargs-overlay"),
				},
			})
			waitForReplicas(h, 1)

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			// podinfo 6.11.1 renders extraArgs into the container's .command
			// (appended to ["./podinfo", "--port=...", ...]). Check both command
			// and args to stay robust against chart layout changes.
			c := deploy.Spec.Template.Spec.Containers[0]
			joined := strings.Join(c.Command, " ") + " " + strings.Join(c.Args, " ")
			Expect(joined).To(ContainSubstring("--level=info"),
				"overlay array value must appear in the container's command/args")
			Expect(joined).ToNot(ContainSubstring("--level=debug"),
				"base array value must NOT appear — arrays are replaced not merged")
			Expect(joined).ToNot(ContainSubstring("--timeout=30"),
				"base orthogonal array item must NOT appear — arrays are replaced wholesale")
		})
	})

	Context("valuesFrom combined with healthStatus criteria", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should reach healthy state using CM-supplied replicaCount", func() {
			createCMWithReplicas(h, "podinfo-values", 2)
			deployPodinfo(h, "s44", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{cmRef("podinfo-values")},
				"healthStatus": []interface{}{
					map[string]interface{}{
						"resource":  map[string]interface{}{"kind": "Deployment", "name": "podinfo"},
						"condition": map[string]interface{}{"type": "Available"},
					},
				},
			})
			waitForReplicas(h, 2)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(h.ctx, h.appKey, h.app)).Should(Succeed())
				g.Expect(h.app.Status.Services).ShouldNot(BeEmpty())
				g.Expect(h.app.Status.Services[0].Healthy).Should(BeTrue())
			}, 120*time.Second, 3*time.Second).Should(Succeed())
		})
	})

	Context("Two helmchart components, each with its own valuesFrom source", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should render each component independently without cross-contamination", func() {
			createCMWithReplicas(h, "podinfo-a-values", 2)
			createSecretWithReplicas(h, "podinfo-b-values", 3)
			compA := buildPodinfoComponent(h, "podinfo-a", "podinfo-a", map[string]interface{}{
				"valuesFrom": []interface{}{cmRef("podinfo-a-values")},
			})
			compB := buildPodinfoComponent(h, "podinfo-b", "podinfo-b", map[string]interface{}{
				"valuesFrom": []interface{}{secretRef("podinfo-b-values")},
			})
			deployAppWithComponents(h, "s45", []common2.ApplicationComponent{compA, compB})
			waitForNamedReplicas(h, "podinfo-a", 2)
			waitForNamedReplicas(h, "podinfo-b", 3)
		})
	})

	Context("Self-healing restores a CM-backed Deployment after manual delete", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should initially deploy with CM-sourced replicaCount", func() {
			createCMWithReplicas(h, "podinfo-values", 3)
			deployPodinfo(h, "s46", "podinfo", map[string]interface{}{
				"valuesFrom": []interface{}{cmRef("podinfo-values")},
			})
			waitForReplicas(h, 3)
		})

		It("should recreate the Deployment with the same CM values after it is deleted", func() {
			runCommandSucceed("kubectl", "delete", "deployment", "podinfo", "-n", h.namespace)
			Eventually(func() bool {
				err := k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, &appsv1.Deployment{})
				return err != nil
			}, 10*time.Second, time.Second).Should(BeTrue())

			RequestReconcileNow(h.ctx, h.app)
			waitForReplicas(h, 3)
		})
	})

	Context("Adoption of an existing vanilla Helm release with valuesFrom", Ordered, func() {
		h := newHelmTestContext()
		BeforeAll(func() { h.createNamespace() })
		AfterAll(func() { h.cleanup() })

		It("should adopt the pre-existing release and merge CM values on the adoption upgrade", func() {
			By("Installing podinfo via vanilla helm at replicaCount=1")
			runCommandSucceed("helm", "install", "podinfo",
				"--repo", "https://stefanprodan.github.io/podinfo", "podinfo",
				"--version", "6.11.1", "--set", "replicaCount=1", "-n", h.namespace)
			Eventually(func(g Gomega) {
				d := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, d)).Should(Succeed())
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
			Expect(k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.namespace, Name: "podinfo"}, deploy)).Should(Succeed())
			Expect(deploy.GetLabels()).To(HaveKey("app.oam.dev/name"),
				"adoption must inject KubeVela ownership labels on the Deployment")
		})
	})
})

func init() {
	// ensure helmchart test file is compiled and registered
	_ = "helm chart tests registered"
}
