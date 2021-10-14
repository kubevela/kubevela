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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/e2e"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Addon Test", func() {
	args := common.Args{}
	var cmSimpleAddon v1.ConfigMap
	var cmInputAddon v1.ConfigMap
	Context("Prepare test addon", func() {
		k8sClient, err := args.GetClient()
		Expect(err).Should(BeNil())
		It("Apply test addon", func() {
			Expect(yaml.Unmarshal([]byte(testAddon), &cmSimpleAddon)).Should(BeNil())
			err = k8sClient.Create(context.Background(), &cmSimpleAddon)
			Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		})
		It("Apply test input addon", func() {
			Expect(yaml.Unmarshal([]byte(testInputAddon), &cmInputAddon)).Should(BeNil())
			err = k8sClient.Create(context.Background(), &cmInputAddon)
			Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		})
	})

	Context("List addons", func() {
		It("List all addon", func() {
			output, err := e2e.Exec("vela addon list")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("test-addon"))
		})
	})
	Context("Enable addon", func() {
		It("Enable addon test-addon", func() {
			output, err := e2e.Exec("vela addon enable test-addon")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully enable addon"))
		})
	})
	Context("Disable addon", func() {
		It("Disable addon test-addon", func() {
			output, err := e2e.LongTimeExec("vela addon disable test-addon", 600*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully disable addon"))
		})
	})

	Context("Test addon receive input", func() {
		It("Enable addon with input", func() {
			output, err := e2e.LongTimeExec("vela addon enable test-input-addon url=https://charts.bitnami.com/bitnami chart=redis", 300*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully enable addon"))
		})
	})

	Context("Disable addon", func() {
		It("Disable addon test-addon", func() {
			output, err := e2e.LongTimeExec("vela addon disable test-input-addon", 600*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully disable addon"))
		})
	})

	Context("Clean test environment", func() {
		It("Delete test addon and test-input addon", func() {
			k8sClient, err := args.GetClient()
			Expect(err).Should(BeNil())
			err = k8sClient.Delete(context.Background(), &cmSimpleAddon)
			Expect(err).Should(BeNil())
			err = k8sClient.Delete(context.Background(), &cmInputAddon)
			Expect(err).Should(BeNil())
		})
	})

})

var testAddon = `
apiVersion: v1
kind: ConfigMap
metadata:
  annotations:
    addons.oam.dev/description: This is a addon for e2e test
    addons.oam.dev/name: test-addon
  labels:
    addons.oam.dev/type: test-addon
  name: test-addon
  namespace: vela-system
data:
  application: |
    apiVersion: core.oam.dev/v1beta1
    kind: Application
    metadata:
      annotations:
        addons.oam.dev/description: This is a addon for e2e test
      name: test-addon
      namespace: vela-system
    spec:
      workflow:
        steps:
          - name: apply-ns
            type: apply-component
            properties:
              component: ns-test-addon-system
          - name: apply-resources
            type: apply-remaining
      components:
        - name: ns-test-addon-system
          type: raw
          properties:
            apiVersion: v1
            kind: Namespace
            metadata:
              name: test-addon-system
        - name: test-addon-pod
          type: raw
          properties:
            apiVersion: v1
            kind: Pod
            metadata:
              name: test-addon-pod
            spec:
              namespace: test-addon-system
              containers:
                - name: test-addon-pod-container
                  image: nginx
`

var testInputAddon = `
kind: ConfigMap
metadata:
  annotations:
    addons.oam.dev/description: This is a test addon for test addon input
    addons.oam.dev/name: test-input-addon
  labels:
    addons.oam.dev/type: test
  name: test-input-addon
  namespace: vela-system
apiVersion: v1
data:
  application: |
    apiVersion: core.oam.dev/v1beta1
    kind: Application
    metadata:
      annotations:
        addons.oam.dev/description: This is a test addon for test addon input
      name: test-input-addon
      namespace: vela-system
    spec:
      components:
      - name: test-chart
        properties:
          chart: [[ index .Args "chart" ]]
          repoType: helm
          url: [[ index .Args "url" ]]
        type: helm
`
