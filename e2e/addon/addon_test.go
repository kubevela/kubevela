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

package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/e2e"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Addon Test", func() {
	args := common.Args{Schema: common.Scheme}
	k8sClient, err := args.GetClient()
	Expect(err).Should(BeNil())

	Context("List addons", func() {
		It("List all addon", func() {
			output, err := e2e.Exec("vela addon list")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("test-addon"))
		})

		It("Enable addon test-addon", func() {
			output, err := e2e.Exec("vela addon enable test-addon")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("enabled successfully."))
		})

		It("Upgrade addon test-addon", func() {
			output, err := e2e.Exec("vela addon upgrade test-addon")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("enabled successfully."))
		})

		It("Disable addon test-addon", func() {
			output, err := e2e.LongTimeExec("vela addon disable test-addon", 600*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully disable addon"))
			Eventually(func(g Gomega) {
				g.Expect(apierrors.IsNotFound(k8sClient.Get(context.Background(), types.NamespacedName{Name: "addon-test-addon", Namespace: "vela-system"}, &v1beta1.Application{}))).Should(BeTrue())
				g.Expect(apierrors.IsNotFound(k8sClient.Get(context.Background(), types.NamespacedName{Name: "addon-secret-test-addon", Namespace: "vela-system"}, &v1.Secret{}))).Should(BeTrue())
			}, 60*time.Second).Should(Succeed())
		})

		It("Enable addon with input", func() {
			output, err := e2e.LongTimeExec("vela addon enable test-addon example=redis", 300*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("enabled successfully."))
		})

		It("Disable addon test-addon", func() {
			output, err := e2e.LongTimeExec("vela addon disable test-addon", 600*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully disable addon"))
			Eventually(func(g Gomega) {
				g.Expect(apierrors.IsNotFound(k8sClient.Get(context.Background(), types.NamespacedName{Name: "addon-test-addon", Namespace: "vela-system"}, &v1beta1.Application{}))).Should(BeTrue())
			}, 60*time.Second).Should(Succeed())
		})

		It("Enable local addon with . as path", func() {
			output, err := e2e.LongTimeExec("vela addon enable ../../e2e/addon/mock/testdata/sample/.", 600*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("sample enabled successfully."))
			Expect(output).To(ContainSubstring("access sample from"))
		})

		It("Test Change default namespace can work", func() {
			output, err := e2e.LongTimeExecWithEnv("vela addon list", 600*time.Second, []string{"DEFAULT_VELA_NS=test-vela"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("test-addon"))
			Expect(output).To(ContainSubstring("disabled"))

			output, err = e2e.LongTimeExecWithEnv("vela addon enable test-addon", 600*time.Second, []string{"DEFAULT_VELA_NS=test-vela"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("enabled successfully."))

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: "addon-test-addon", Namespace: "test-vela"}, &v1beta1.Application{})).Should(BeNil())
			}, 60*time.Second).Should(Succeed())

			output, err = e2e.LongTimeExecWithEnv("vela addon disable test-addon", 600*time.Second, []string{"DEFAULT_VELA_NS=test-vela"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully disable addon"))
			Eventually(func(g Gomega) {
				g.Expect(apierrors.IsNotFound(k8sClient.Get(context.Background(), types.NamespacedName{Name: "addon-test-addon", Namespace: "test-vela"}, &v1beta1.Application{}))).Should(BeTrue())
			}, 60*time.Second).Should(Succeed())
		})
	})

	Context("Addon registry test", func() {
		It("List all addon registry", func() {
			output, err := e2e.Exec("vela addon registry list")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("KubeVela"))
		})

		It("Get addon registry", func() {
			output, err := e2e.Exec("vela addon registry get KubeVela")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("KubeVela"))
		})

		It("Add test addon registry", func() {
			output, err := e2e.LongTimeExec("vela addon registry add my-repo --type=git --endpoint=https://github.com/oam-dev/catalog --path=/experimental/addons", 600*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully add an addon registry my-repo"))

			Eventually(func() error {
				output, err := e2e.LongTimeExec("vela addon registry update my-repo --type=git --endpoint=https://github.com/oam-dev/catalog --path=/addons", 300*time.Second)
				if err != nil {
					return err
				}
				if !strings.Contains(output, "Successfully update an addon registry my-repo") {
					return fmt.Errorf("cannot update addon registry")
				}
				return nil
			}, 30*time.Second, 300*time.Millisecond).Should(BeNil())

			output, err = e2e.LongTimeExec("vela addon registry delete my-repo", 600*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully delete an addon registry my-repo"))
		})
	})
})
