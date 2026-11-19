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

package controllers_test

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	terraformv1beta1 "github.com/oam-dev/terraform-controller/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Addon tests", func() {
	ctx := context.Background()
	var namespaceName string
	var ns corev1.Namespace
	var app v1beta1.Application
	var namespaceCreated bool

	BeforeEach(func() {
		namespaceCreated = false
		app = v1beta1.Application{}
		namespaceName = uniqueAddonNamespace()
		var err error
		ns, err = createAddonNamespace(ctx, k8sClient, namespaceName)
		Expect(err).To(Succeed())
		namespaceCreated = true
	})

	AfterEach(func() {
		if !namespaceCreated {
			return
		}
		By("Clean up resources after a test")
		if app.Name != "" {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &app))).To(Succeed())
		}
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground)))).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, &corev1.Namespace{}))
		}, 120*time.Second, 500*time.Millisecond).Should(BeTrue(), "namespace %s did not finish terminating", namespaceName)
	})

	It("Addon Terraform is successfully enabled and Terraform application works", func() {
		By("Checking Provider")
		Eventually(func() error {
			var provider terraformv1beta1.Provider
			return k8sClient.Get(ctx, client.ObjectKey{Name: "default", Namespace: "default"}, &provider)
		}, time.Second*120, time.Millisecond*500).Should(BeNil())

		By("Apply an application with Terraform Component")
		var terraformApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/app_terraform_oss.yaml", &terraformApp)).Should(BeNil())
		terraformApp.Namespace = namespaceName
		Eventually(func() error {
			return k8sClient.Create(ctx, terraformApp.DeepCopy())
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Check status.services of the application")
		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: terraformApp.Namespace, Name: terraformApp.Name}, &app)
				if len(app.Status.Services) == 1 {
					return nil
				}
				return errors.New("expect 1 service")
			},
			time.Second*30, time.Millisecond*500).ShouldNot(BeNil())
	})

	PIt("Addon observability is successfully enabled", func() {
		By("Install Addon Observability")
		enableAddonForSuite("observability", "domain=abc.com", "disk-size=20Gi")
	})

	It("Addon Workflow is successfully enabled and WorkflowRun creates Deployment", func() {
		By("Apply a WorkflowRun which creates a Deployment")
		var wr workflowv1alpha1.WorkflowRun
		Expect(common.ReadYamlToObject("./testdata/workflow/workflowrun_nginx.yaml", &wr)).Should(BeNil())
		// set namespace to dynamically created test namespace
		wr.Namespace = namespaceName
		Eventually(func() error { return k8sClient.Create(ctx, wr.DeepCopy()) }, 20*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verify the Deployment is created by WorkflowRun")
		Eventually(func() error {
			var deploy appsv1.Deployment
			return k8sClient.Get(ctx, client.ObjectKey{Name: "apply-nginx-deployment", Namespace: namespaceName}, &deploy)
		}, 180*time.Second, 2*time.Second).Should(Succeed())

		By("Check the WorkflowRun reaches a succeeded phase")
		Eventually(func() error {
			var latest workflowv1alpha1.WorkflowRun
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: wr.Name, Namespace: namespaceName}, &latest); err != nil {
				return err
			}
			if latest.Status.Phase == workflowv1alpha1.WorkflowStateSucceeded {
				return nil
			}
			return fmt.Errorf("workflowrun not finished, current phase: %s", latest.Status.Phase)
		}, 300*time.Second, 2*time.Second).Should(Succeed())
	})
})

func uniqueAddonNamespace() string {
	var token [12]byte
	if _, err := cryptorand.Read(token[:]); err != nil {
		panic(fmt.Errorf("generate addon test namespace: %w", err))
	}
	return fmt.Sprintf("app-addon-p%d-%s", GinkgoParallelProcess(), hex.EncodeToString(token[:]))
}

func createAddonNamespace(ctx context.Context, cli client.Client, name string) (corev1.Namespace, error) {
	ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := cli.Create(ctx, &ns); err != nil {
		return corev1.Namespace{}, err
	}
	return ns, nil
}
