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

package framework

import (
	"bytes"
	"context"
	"encoding/json"
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

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

type HelmTestContext struct {
	Framework    *Framework
	Ctx          context.Context
	Namespace    string
	AppNamespace string
	App          *v1beta1.Application
	AppKey       client.ObjectKey
	Attempted    bool
}

func (f *Framework) NewHelmTestContext() *HelmTestContext {
	return &HelmTestContext{
		Framework:    f,
		Ctx:          context.Background(),
		AppNamespace: "default",
	}
}

func HelmTestSuffix() string {
	return strings.TrimPrefix(RandomNamespaceName(""), "-")
}

func (h *HelmTestContext) CreateNamespace() {
	if h.Namespace == "" {
		h.Namespace = RandomNamespaceName("helm-e2e")
	}
	By("Creating target namespace for Helm release: " + h.Namespace)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: h.Namespace}}
	Expect(h.Framework.Client.Create(h.Ctx, ns)).Should(SatisfyAny(Succeed(), Not(HaveOccurred())))
}

func (h *HelmTestContext) Cleanup() {
	By("Deleting Application if it exists")
	if h.App != nil {
		_ = h.Framework.Client.Delete(h.Ctx, h.App)
		Eventually(func() bool {
			err := h.Framework.Client.Get(h.Ctx, h.AppKey, &v1beta1.Application{})
			return err != nil
		}, 60*time.Second, 2*time.Second).Should(BeTrue())
	}
	By("Deleting target namespace")
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: h.Namespace}}
	_ = h.Framework.Client.Delete(h.Ctx, ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
}

// freshAttempt moves a retried spec to a new namespace, removing the previous
// attempt's Application and release. FlakeAttempts reruns an It but not its
// BeforeAll, and a helm install into the first attempt's namespace fails.
func (h *HelmTestContext) FreshAttempt() {
	if h.Attempted {
		h.Cleanup()
		h.App = nil
		h.Namespace = RandomNamespaceName("helm-e2e")
		h.CreateNamespace()
	}
	h.Attempted = true
}

func (h *HelmTestContext) CleanupNamespaceOnly() {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: h.Namespace}}
	_ = h.Framework.Client.Delete(h.Ctx, ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
}

func (h *HelmTestContext) DeployApp() {
	h.DeployAppFrom(TestDataPath("helm/app_helmchart_podinfo.yaml"))
}

func (h *HelmTestContext) DeployAppFrom(yamlPath string) {
	By("Deploying helmchart Application from " + yamlPath)
	raw, err := os.ReadFile(yamlPath) //nolint:gosec // The caller resolves a committed E2E fixture path.
	Expect(err).Should(BeNil())
	raw = bytes.ReplaceAll(raw, []byte("placeholder_ns"), []byte(h.Namespace))
	h.App = &v1beta1.Application{}
	Expect(yaml.Unmarshal(raw, h.App)).Should(BeNil())
	h.App.SetNamespace(h.AppNamespace)
	h.App.SetName(RandomNamespaceName("podinfo-helm-test"))
	Expect(h.Framework.Client.Create(h.Ctx, h.App)).Should(Succeed())
	h.AppKey = client.ObjectKeyFromObject(h.App)
	By("Waiting for Application to reach running state")
	Eventually(func(g Gomega) {
		g.Expect(h.Framework.Client.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
		g.Expect(h.App.Status.Phase).Should(Equal(common2.ApplicationRunning))
	}, 120*time.Second, 3*time.Second).Should(Succeed())
}

func (h *HelmTestContext) WaitForDeploymentReady() {
	By("Waiting for Deployment to be ready with 2 replicas")
	Eventually(func(g Gomega) {
		deploy := &appsv1.Deployment{}
		g.Expect(h.Framework.Client.Get(h.Ctx, types.NamespacedName{
			Namespace: h.Namespace, Name: "podinfo",
		}, deploy)).Should(Succeed())
		g.Expect(deploy.Status.ReadyReplicas).Should(Equal(int32(2)))
	}, 120*time.Second, 3*time.Second).Should(Succeed())
}

func (h *HelmTestContext) GetHelmSecrets() *corev1.SecretList {
	secrets := &corev1.SecretList{}
	Expect(h.Framework.Client.List(h.Ctx, secrets,
		client.InNamespace(h.Namespace),
		client.MatchingLabels{"owner": "helm", "name": "podinfo"},
	)).Should(Succeed())
	return secrets
}

func (h *HelmTestContext) WaitForAppRunning() {
	By("Waiting for Application to return to running state")
	Eventually(func(g Gomega) {
		g.Expect(h.Framework.Client.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
		g.Expect(h.App.Status.Phase).Should(Equal(common2.ApplicationRunning))
	}, 180*time.Second, 3*time.Second).Should(Succeed())
}

func (h *HelmTestContext) LatestHelmSecretName() string {
	secrets := h.GetHelmSecrets()
	var latest string
	for _, s := range secrets.Items {
		if latest == "" || s.Name > latest {
			latest = s.Name
		}
	}
	return latest
}

func (h *HelmTestContext) UpdateAppValues(values map[string]interface{}) {
	By("Updating Application values to trigger upgrade")
	Eventually(func(g Gomega) {
		g.Expect(h.Framework.Client.Get(h.Ctx, h.AppKey, h.App)).Should(Succeed())
		raw, err := json.Marshal(h.App.Spec.Components[0].Properties)
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
		h.App.Spec.Components[0].Properties = &runtime.RawExtension{Raw: newRaw}
		g.Expect(h.Framework.Client.Update(h.Ctx, h.App)).Should(Succeed())
	}, 30*time.Second, time.Second).Should(Succeed())
	h.WaitForAppRunning()
}

func (h *HelmTestContext) RecordPodUIDs() map[types.UID]bool {
	podList := &corev1.PodList{}
	Expect(h.Framework.Client.List(h.Ctx, podList,
		client.InNamespace(h.Namespace),
		client.MatchingLabels{"app.kubernetes.io/name": "podinfo"},
	)).Should(Succeed())
	uids := make(map[types.UID]bool)
	for _, pod := range podList.Items {
		if pod.DeletionTimestamp == nil {
			uids[pod.UID] = true
		}
	}
	return uids
}

func (h *HelmTestContext) CountSurvivingPods(originalUIDs map[types.UID]bool) int {
	podList := &corev1.PodList{}
	Expect(h.Framework.Client.List(h.Ctx, podList,
		client.InNamespace(h.Namespace),
		client.MatchingLabels{"app.kubernetes.io/name": "podinfo"},
	)).Should(Succeed())
	count := 0
	for _, pod := range podList.Items {
		if originalUIDs[pod.UID] && pod.DeletionTimestamp == nil {
			count++
		}
	}
	return count
}

// PodinfoAppResourceFlags are the values testdata/helm/app_helmchart_podinfo.yaml
// sets besides replicaCount. A vanilla release installed with them has the pod
// template the Application renders, so adopting it rolls no pods.
var PodinfoAppResourceFlags = []string{
	"--set", "resources.limits.memory=256Mi",
	"--set", "resources.limits.cpu=100m",
}

func RunCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	GinkgoWriter.Printf("$ %s %v\n%s\n", name, args, string(out))
	return string(out), err
}

func RunCommandSucceed(name string, args ...string) string {
	out, err := RunCommand(name, args...)
	Expect(err).Should(Succeed(), "command failed: %s %v\noutput: %s", name, args, out)
	return out
}
