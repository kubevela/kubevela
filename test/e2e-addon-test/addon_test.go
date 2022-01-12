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
	"os/exec"
	"strconv"
	"time"

	terraformv1beta1 "github.com/oam-dev/terraform-controller/api/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
		output, err := exec.Command("bash", "-c", "/tmp/vela addon enable terraform-alibaba ALICLOUD_ACCESS_KEY=xxx ALICLOUD_SECRET_KEY=yyy ALICLOUD_REGION=cn-beijing").Output()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			fmt.Println("exit code error:", string(ee.Stderr))
		}
		Expect(err).Should(BeNil())
		Expect(string(output)).Should(ContainSubstring("enabled Successfully"))

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
		output, err := exec.Command("bash", "-c", "/tmp/vela addon enable observability domain=abc.com disk-size=20Gi").Output()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			fmt.Println("exit code error:", string(ee.Stderr))
		}
		Expect(err).Should(BeNil())
		Expect(string(output)).Should(ContainSubstring("enabled Successfully"))
	})
})
