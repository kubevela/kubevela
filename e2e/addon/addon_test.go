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

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/e2e"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Addon Test", func() {
	args := common.Args{}
	var cm v1.ConfigMap
	Context("Prepare test addon", func() {
		It("apply test addon", func() {
			Expect(yaml.Unmarshal([]byte(test_addon), &cm)).Should(BeNil())
			k8sClient, err := args.GetClient()
			Expect(err).Should(BeNil())
			err = k8sClient.Create(context.Background(), &cm)
			Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		})
	})

	Context("list addons", func() {
		It("list all addon", func() {
			output, err := e2e.Exec("vela addon list")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("test-addon"))
		})
	})
	Context("enable addon", func() {
		It("enable addon fluxcd", func() {
			output, err := e2e.Exec("vela addon enable test-addon")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully enable addon"))
		})
	})
	Context("disable addon", func() {
		It("disable addon fluxcd", func() {
			output, err := e2e.LongTimeExec("vela addon disable test-addon", 600*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully disable addon"))
		})
	})

	Context("Clean test environment", func() {
		It("Delete test addon", func() {
			k8sClient, err := args.GetClient()
			Expect(err).Should(BeNil())
			err = k8sClient.Delete(context.Background(), &cm)
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
