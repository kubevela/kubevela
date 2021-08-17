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
			Expect(yaml.Unmarshal([]byte(test_addon), &cmSimpleAddon)).Should(BeNil())
			err = k8sClient.Create(context.Background(), &cmSimpleAddon)
			Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		})
		It("Apply test input addon", func() {
			Expect(yaml.Unmarshal([]byte(test_input_addon), &cmInputAddon)).Should(BeNil())
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
		It("Disable addon fluxcd", func() {
			output, err := e2e.LongTimeExec("vela addon disable test-addon", 600*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully disable addon"))
		})
	})

	Context("Test addon receive input", func() {
		It("Enable addon with input", func() {
			output, err := e2e.LongTimeExec("vela addon enable test-input-addon repoUrl=https://charts.bitnami.com/bitnami chart=redis", 300*time.Second)
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

var test_addon = `
apiVersion: v1
kind: ConfigMap
metadata:
  annotations:
    addons.oam.dev/description: This is a addon for e2e test
  labels:
    addons.oam.dev/type: test-addon
  name: test-addon
  namespace: vela-system
data:
  initializer: |
    apiVersion: core.oam.dev/v1beta1
    kind: Initializer
    metadata:
      annotations:
        addons.oam.dev/description: This is a addon for e2e test
      name: test-addon
      namespace: test-addon-system
    spec:
      appTemplate:
        spec:
          components:
            - name: test-addon-pod
              type: raw
              properties:
                apiVersion: v1
                kind: Pod
                metadata:
                  name: test-addon-pod
                  namespace: test-addon-system
                spec:
                  containers:
                    - name: test-addon-pod-container
                      image: nginx
`

var test_input_addon = `
kind: ConfigMap
metadata:
  annotations:
    addons.oam.dev/description: This is a test addon for test addon input
  labels:
    addons.oam.dev/type: test
  name: test-input-addon
  namespace: vela-system
apiVersion: v1
data:
  initializer: |
    apiVersion: core.oam.dev/v1beta1
    kind: Initializer
    metadata:
      annotations:
        addons.oam.dev/description: This is a test addon for test addon input
      name: test-input-addon
      namespace: vela-system
    spec:
      appTemplate:
        spec:
          components:
          - name: test-chart
            properties:
              chart: [[ index .Args "chart" ]]
              repoType: helm
              repoUrl: [[ index .Args "repoUrl" ]]
            type: helm
        status:
          rollout:
            batchRollingState: ""
            currentBatch: 0
            lastTargetAppRevision: ""
            rollingState: ""
            upgradedReadyReplicas: 0
            upgradedReplicas: 0
      dependsOn:
      - ref:
          apiVersion: core.oam.dev/v1beta1
          kind: Initializer
          name: fluxcd
          namespace: vela-system
    status:
      observedGeneration: 0
`
