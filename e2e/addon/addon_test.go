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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Netflix/go-expect"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/e2e"
	"github.com/oam-dev/kubevela/pkg/addon"
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

		It("Enable addon with specified registry ", func() {
			output, err := e2e.LongTimeExec("vela addon enable KubeVela/test-addon", 300*time.Second)
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
		})

		It("Test Change default namespace can work", func() {
			output, err := e2e.LongTimeExecWithEnv("vela addon list", 600*time.Second, []string{"DEFAULT_VELA_NS=test-vela"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("test-addon"))
			Expect(output).To(ContainSubstring("-"))

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

		It("Enable fluxcd-test-version whose version can't suit system requirements", func() {
			output, err := e2e.InteractiveExec("vela addon enable fluxcd-test-version", func(c *expect.Console) {
				_, err = c.SendLine("y")
				Expect(err).NotTo(HaveOccurred())
			})
			Expect(output).To(ContainSubstring("enabled successfully"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("Disable addon fluxcd-test-version", func() {
			output, err := e2e.LongTimeExec("vela addon disable fluxcd-test-version", 600*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully disable addon"))
		})

		It("Enable fluxcd-test-version whose version can't suit system requirements with 'n' input", func() {
			output, err := e2e.InteractiveExec("vela addon enable fluxcd-test-version", func(c *expect.Console) {
				_, err = c.SendLine("n")
				Expect(err).NotTo(HaveOccurred())
			})
			Expect(output).To(ContainSubstring("you can try another version by command"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("Test addon dependency with specified clusters", func() {
			const clusterName = "k3s-default"
			// enable addon
			output, err := e2e.Exec("vela addon enable mock-dependence --clusters local myparam=test")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("enabled successfully."))
			output1, err := e2e.Exec("vela ls -A")
			Expect(err).NotTo(HaveOccurred())
			Expect(output1).To(ContainSubstring("mock-dependence"))
			output2, err := e2e.Exec("vela addon list")
			Expect(err).NotTo(HaveOccurred())
			Expect(output2).To(ContainSubstring("mock-dependence"))
			// check dependence application parameter
			Eventually(func(g Gomega) {
				// check parameter
				sec := &v1.Secret{}
				g.Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: "addon-secret-mock-dependence", Namespace: "vela-system"}, sec)).Should(Succeed())
				parameters := map[string]interface{}{}
				json.Unmarshal(sec.Data[addon.AddonParameterDataKey], &parameters)
				g.Expect(parameters).Should(BeEquivalentTo(map[string]interface{}{
					"clusters": []interface{}{"local"},
					"myparam":  "test",
				}))
				// check application render cluster
				app := &v1beta1.Application{}
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: "addon-mock-dependence", Namespace: "vela-system"}, app)).Should(Succeed())
				topologyPolicyValue := map[string]interface{}{}
				for _, policy := range app.Spec.Policies {
					if policy.Type == "topology" {
						Expect(json.Unmarshal(policy.Properties.Raw, &topologyPolicyValue)).Should(Succeed())
						break
					}
				}
				fluxcdYaml, err1 := e2e.Exec("vela status addon-mock-dependence -n vela-system -oyaml")
				Expect(err1).NotTo(HaveOccurred())
				Expect(fluxcdYaml).To(ContainSubstring("mock-dependence"))
				fluxcdStatus, err2 := e2e.Exec("vela addon status mock-dependence -v")
				Expect(err2).NotTo(HaveOccurred())
				Expect(fluxcdStatus).To(ContainSubstring("mock-dependence"))
				Expect(topologyPolicyValue["clusters"]).Should(Equal([]interface{}{"local"}))
			}, 600*time.Second).Should(Succeed())
			// enable addon which rely on mock-dependence addon
			e2e.Exec("vela addon enable mock-dependence-rely --clusters local," + clusterName)
			// check mock-dependence application parameter
			Eventually(func(g Gomega) {
				sec := &v1.Secret{}
				g.Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: "addon-secret-mock-dependence", Namespace: "vela-system"}, sec)).Should(Succeed())
				parameters := map[string]interface{}{}
				json.Unmarshal(sec.Data[addon.AddonParameterDataKey], &parameters)
				g.Expect(parameters).Should(BeEquivalentTo(map[string]interface{}{
					"clusters": []interface{}{"local", clusterName},
					"myparam":  "test",
				}))
				app := &v1beta1.Application{}
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: "addon-mock-dependence", Namespace: "vela-system"}, app)).Should(Succeed())
				topologyPolicyValue := map[string]interface{}{}
				for _, policy := range app.Spec.Policies {
					if policy.Type == "topology" {
						Expect(json.Unmarshal(policy.Properties.Raw, &topologyPolicyValue)).Should(Succeed())
						break
					}
				}
				Expect(topologyPolicyValue["clusters"]).Should(Equal([]interface{}{"local", clusterName}))
			}, 30*time.Second).Should(Succeed())
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
