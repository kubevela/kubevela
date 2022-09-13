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

package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	pkgutils "github.com/oam-dev/kubevela/pkg/utils/util"
)

var _ = Describe("Test Install Command", func() {
	BeforeEach(func() {
		fluxcd := v1beta1.Application{}
		err := yaml.Unmarshal([]byte(fluxcdYaml), &fluxcd)
		Expect(err).Should(BeNil())
		Expect(k8sClient.Create(context.Background(), &fluxcd)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		rollout := v1beta1.Application{}
		err = yaml.Unmarshal([]byte(rolloutYaml), &rollout)
		Expect(err).Should(BeNil())
		Expect(k8sClient.Create(context.Background(), &rollout)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
	})

	It("Test check addon enabled", func() {
		addons, err := checkInstallAddon(k8sClient)
		Expect(err).Should(BeNil())
		Expect(len(addons)).Should(BeEquivalentTo(2))
	})

	It("Test disable all addons", func() {
		err := forceDisableAddon(context.Background(), k8sClient, cfg)
		Expect(err).Should(BeNil())
		Eventually(func() error {
			addons, err := checkInstallAddon(k8sClient)
			if err != nil {
				return err
			}
			if len(addons) != 0 {
				return fmt.Errorf("%v still exist", addons)
			}
			return nil
		}, 1*time.Minute, 5*time.Second).Should(BeNil())
	})
})

func TestUninstall(t *testing.T) {
	// Test answering NO when prompted. Should just exit.
	cmd := NewUnInstallCommand(common.Args{}, "", pkgutils.IOStreams{
		Out: os.Stdout,
		In:  strings.NewReader("n\n"),
	})
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Nil(t, err, "should just exit if answer is no")
}

var fluxcdYaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: addon-fluxcd
  namespace: vela-system
  labels:
    addons.oam.dev/name: fluxcd
    addons.oam.dev/registry: local
    addons.oam.dev/version: 1.1.0
spec:
  components:
  - name: ns-flux-system
    properties:
      apiVersion: v1
      kind: Namespace
      metadata:
        name: flux-system
`

var fluxcdRemoteYaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: addon-fluxcd
  namespace: vela-system
  labels:
    addons.oam.dev/name: fluxcd
    addons.oam.dev/registry: KubeVela
    addons.oam.dev/version: 1.1.0
spec:
  components:
  - name: ns-flux-system
    properties:
      apiVersion: v1
      kind: Namespace
      metadata:
        name: flux-system
`

var rolloutYaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: addon-rollout
  namespace: vela-system
  labels:
     addons.oam.dev/name: rollout
spec:
  components:
  - name: test-ns
    properties:
      apiVersion: v1
      kind: Namespace
      metadata:
        name: test-ns
`
