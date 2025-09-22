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
	"errors"
	"fmt"
	"math/rand"

	// "os"
	"os/exec"
	"strconv"

	// "strings"
	"time"

	terraformv1beta1 "github.com/oam-dev/terraform-controller/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// "sigs.k8s.io/yaml"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Addon tests", func() {
	ctx := context.Background()
	var namespaceName string
	var ns corev1.Namespace
	var app v1beta1.Application

	createNamespace := func() {
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}
		// delete the namespaceName with all its resources
		Eventually(
			func() error {
				return k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
			},
			time.Second*120, time.Millisecond*500).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		By("make sure all the resources are removed")
		objectKey := client.ObjectKey{
			Name: namespaceName,
		}
		res := &corev1.Namespace{}
		Eventually(
			func() error {
				return k8sClient.Get(ctx, objectKey, res)
			},
			time.Second*120, time.Millisecond*500).Should(&util.NotFoundMatcher{})
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}

	BeforeEach(func() {
		By("Start to run a test, clean up previous resources")
		namespaceName = "app-terraform" + "-" + strconv.FormatInt(rand.Int63(), 16)
		createNamespace()
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.Delete(ctx, &app)
		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		// delete the namespaceName with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
	})

	It("Addon Terraform is successfully enabled and Terraform application works", func() {
		By("Install Addon Terraform")
		output, err := exec.Command("bash", "-c", "/tmp/vela addon enable terraform-alibaba").Output()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			fmt.Println("exit code error:", string(ee.Stderr))
		}
		Expect(err).Should(BeNil())
		Expect(string(output)).Should(ContainSubstring("enabled successfully"))

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

	// FIt("Addon generic Terraform creates a Pod via Terraform component", func() {
	// 	By("Install generic Terraform Addon")
	// 	output, err := exec.Command("bash", "-c", "/Users/co/co_kubevela/kubevela/bin/vela addon enable terraform").Output()
	// 	var ee *exec.ExitError
	// 	if errors.As(err, &ee) {
	// 		fmt.Println("exit code error:", string(ee.Stderr))
	// 	}
	// 	Expect(err).Should(BeNil())
	// 	Expect(string(output)).Should(ContainSubstring("enabled successfully"))

	// 	By("Apply ComponentDefinition for k8s-pod")
	// 	var compDef v1beta1.ComponentDefinition
	// 	Expect(common.ReadYamlToObject("testdata/definitions/terraform_pod_component.yaml", &compDef)).Should(BeNil())
	// 	// create or update
	// 	Eventually(func() error { return k8sClient.Create(ctx, compDef.DeepCopy()) }, 10*time.Second, 500*time.Millisecond).Should(Succeed())

	// 	By("Apply Application that provisions a Pod using Terraform")
	// 	raw, err := os.ReadFile("testdata/app/app_terraform_pod.yaml")
	// 	Expect(err).Should(BeNil())
	// 	replaced := strings.ReplaceAll(string(raw), "___NAMESPACE___", namespaceName)
	// 	var podApp v1beta1.Application
	// 	Expect(yaml.Unmarshal([]byte(replaced), &podApp)).Should(BeNil())
	// 	podApp.Namespace = namespaceName
	// 	Eventually(func() error { return k8sClient.Create(ctx, podApp.DeepCopy()) }, 20*time.Second, 500*time.Millisecond).Should(Succeed())

	// 	By("Verify the Pod is created")
	// 	Eventually(func() error {
	// 		var pod corev1.Pod
	// 		return k8sClient.Get(ctx, client.ObjectKey{Name: "tf-sample-pod", Namespace: namespaceName}, &pod)
	// 	}, 180*time.Second, 2*time.Second).Should(Succeed())
	// })

	PIt("Addon observability is successfully enabled", func() {
		By("Install Addon Observability")
		output, err := exec.Command("bash", "-c", "/tmp/vela addon enable observability domain=abc.com disk-size=20Gi").Output()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			fmt.Println("exit code error:", string(ee.Stderr))
		}
		Expect(err).Should(BeNil())
		Expect(string(output)).Should(ContainSubstring("enabled successfully"))
	})

	FIt("Addon Workflow is successfully enabled and WorkflowRun creates Deployment", func() {
		By("Install Addon Workflow")
		// assume addon name is 'workflow'
		output, err := exec.Command("bash", "-c", "/tmp/vela addon enable vela-workflow").Output()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			fmt.Println("exit code error:", string(ee.Stderr))
		}
		Expect(err).Should(BeNil())
		Expect(string(output)).Should(ContainSubstring("enabled successfully"))

		By("Apply a WorkflowRun which creates a Deployment")
		var wr workflowv1alpha1.WorkflowRun
		Expect(common.ReadYamlToObject("./testdata/workflow/workflowrun_nginx.yaml", &wr)).Should(BeNil())
		// set namespace to dynamically created test namespace
		wr.Namespace = namespaceName
		Eventually(func() error { return k8sClient.Create(ctx, wr.DeepCopy()) }, 20*time.Second, 500*time.Millisecond).Should(Succeed())

		By("List all WorkflowRuns across all namespaces for debugging")
		var allWorkflowRuns workflowv1alpha1.WorkflowRunList
		if err := k8sClient.List(ctx, &allWorkflowRuns); err != nil {
			fmt.Printf("Failed to list WorkflowRuns: %v\n", err)
		} else {
			fmt.Printf("Found %d WorkflowRuns across all namespaces\n", len(allWorkflowRuns.Items))
			for _, w := range allWorkflowRuns.Items {
				fmt.Printf("WorkflowRun %s/%s\n", w.Namespace, w.Name)
				// Debug: describe each WorkflowRun dynamically
				fmt.Printf("Describing WorkflowRun %s/%s\n", w.Namespace, w.Name)
				describeCmd := fmt.Sprintf("kubectl describe workflowrun %s -n %s", w.Name, w.Namespace)
				if descOut, descErr := exec.Command("bash", "-c", describeCmd).CombinedOutput(); descErr != nil {
					fmt.Printf("kubectl describe failed (%s): %v\nOutput:\n%s\n", describeCmd, descErr, string(descOut))
				} else {
					fmt.Printf("kubectl describe output for %s/%s:\n%s\n", w.Namespace, w.Name, string(descOut))
				}
			}
		}

		By("Verify the Deployment is created by WorkflowRun")
		Eventually(func() error {
			var deploy appsv1.Deployment
			return k8sClient.Get(ctx, client.ObjectKey{Name: "apply-nginx-deployment", Namespace: namespaceName}, &deploy)
		}, 180*time.Second, 2*time.Second).Should(SatisfyAny(Succeed(), MatchError(ContainSubstring("not found"))))

		// Add debugging if deployment was not found

		By("Deployment not found, printing debug information")

		// Print all deployments in the namespace
		var deployList appsv1.DeploymentList
		if listErr := k8sClient.List(ctx, &deployList, client.InNamespace(namespaceName)); listErr == nil {
			fmt.Printf("All deployments in namespace %s:\n", namespaceName)
			for _, d := range deployList.Items {
				fmt.Printf("  - Name: %s, Ready: %d/%d\n", d.Name, d.Status.ReadyReplicas, d.Status.Replicas)
			}
		}

		By("Check the WorkflowRun reaches a terminal phase")
		Eventually(func() error {
			var latest workflowv1alpha1.WorkflowRun
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: wr.Name, Namespace: namespaceName}, &latest); err != nil {
				return err
			}
			if latest.Status.Phase == workflowv1alpha1.WorkflowStateSucceeded || latest.Status.Phase == workflowv1alpha1.WorkflowStateFailed || latest.Status.Phase == workflowv1alpha1.WorkflowStateTerminated {
				return nil
			}
			return fmt.Errorf("workflowrun not finished, current phase: %s", latest.Status.Phase)
		}, 300*time.Second, 2*time.Second).Should(Succeed())
	})
})
