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
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/pkg/util/rand"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("Helmchart Component Reconciliation", Ordered, func() {
	ctx := context.Background()
	var (
		namespace    string
		appNamespace string
		app          *v1beta1.Application
		appKey       client.ObjectKey
	)

	BeforeAll(func() {
		namespace = "helm-e2e-" + rand.RandomString(4)
		appNamespace = "default"

		By("Creating target namespace for Helm release")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, ns)).Should(SatisfyAny(Succeed(), Not(HaveOccurred())))
	})

	AfterAll(func() {
		By("Deleting Application if it exists")
		if app != nil {
			_ = k8sClient.Delete(ctx, app)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, appKey, &v1beta1.Application{})
				return err != nil
			}, 60*time.Second, 2*time.Second).Should(BeTrue())
		}

		By("Deleting target namespace")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		_ = k8sClient.Delete(ctx, ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
	})

	deployApp := func() {
		By("Deploying helmchart Application")
		raw, err := os.ReadFile("testdata/helm/app_helmchart_podinfo.yaml")
		Expect(err).Should(BeNil())

		// Replace placeholder namespace with test namespace
		raw = bytes.ReplaceAll(raw, []byte("PLACEHOLDER_NS"), []byte(namespace))

		app = &v1beta1.Application{}
		Expect(yaml.Unmarshal(raw, app)).Should(BeNil())
		app.SetNamespace(appNamespace)
		app.SetName("podinfo-helm-test-" + rand.RandomString(4))

		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		appKey = client.ObjectKeyFromObject(app)

		By("Waiting for Application to reach running state")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, app)).Should(Succeed())
			g.Expect(app.Status.Phase).Should(Equal(common2.ApplicationRunning))
		}, 120*time.Second, 3*time.Second).Should(Succeed())
	}

	waitForDeploymentReady := func() {
		By("Waiting for Deployment to be ready with 2 replicas")
		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: namespace,
				Name:      "podinfo",
			}, deploy)).Should(Succeed())
			g.Expect(deploy.Status.ReadyReplicas).Should(Equal(int32(2)))
		}, 120*time.Second, 3*time.Second).Should(Succeed())
	}

	getHelmSecrets := func() *corev1.SecretList {
		secrets := &corev1.SecretList{}
		Expect(k8sClient.List(ctx, secrets,
			client.InNamespace(namespace),
			client.MatchingLabels{"owner": "helm", "name": "podinfo"},
		)).Should(Succeed())
		return secrets
	}

	Context("Scenario 1: Delete a Single Managed Resource (Deployment)", func() {
		It("should deploy podinfo successfully", func() {
			deployApp()
			waitForDeploymentReady()

			By("Verifying Helm release secret exists")
			secrets := getHelmSecrets()
			Expect(len(secrets.Items)).Should(BeNumerically(">=", 1))
		})

		It("should recover when the Deployment is deleted", func() {
			By("Recording initial Helm secret count")
			initialSecrets := getHelmSecrets()
			initialCount := len(initialSecrets.Items)
			// Record the latest revision secret name for comparison
			var latestSecret string
			for _, s := range initialSecrets.Items {
				if latestSecret == "" || s.Name > latestSecret {
					latestSecret = s.Name
				}
			}

			By("Deleting the Deployment")
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "podinfo",
					Namespace: namespace,
				},
			}
			Expect(k8sClient.Delete(ctx, deploy)).Should(Succeed())

			By("Verifying Deployment is gone")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: namespace,
					Name:      "podinfo",
				}, &appsv1.Deployment{})
				return err != nil
			}, 10*time.Second, time.Second).Should(BeTrue())

			By("Triggering reconciliation")
			RequestReconcileNow(ctx, app)

			By("Verifying KubeVela recreates the Deployment")
			waitForDeploymentReady()

			By("Verifying Helm revision did NOT increment (recovery is via ResourceTracker)")
			secrets := getHelmSecrets()
			Expect(len(secrets.Items)).Should(Equal(initialCount))
			// The latest secret should still be the same — no new revision
			var currentLatest string
			for _, s := range secrets.Items {
				if currentLatest == "" || s.Name > currentLatest {
					currentLatest = s.Name
				}
			}
			Expect(currentLatest).Should(Equal(latestSecret),
				"Helm revision should not increment — recovery is via ResourceTracker, not helm upgrade")

			By("Verifying pods are back to desired replica count")
			deploy = &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: namespace,
				Name:      "podinfo",
			}, deploy)).Should(Succeed())
			Expect(*deploy.Spec.Replicas).Should(Equal(int32(2)))
		})
	})
})

func init() {
	// Ensure the describe block is registered
	_ = fmt.Sprintf("helmchart tests registered")
}
