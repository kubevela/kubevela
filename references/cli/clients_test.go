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
	"github.com/kubevela/workflow/pkg/cue/packages"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

var _ = Describe("Init tool clients", func() {
	var (
		cliBackup client.Client
		cfgBackup *rest.Config
		dmBackup  discoverymapper.DiscoveryMapper
		pdBackup  *packages.PackageDiscover
	)

	BeforeEach(func() {
		cliBackup = cli
		cfgBackup = cfg
		dmBackup = dm
		pdBackup = pd
	})

	AfterEach(func() {
		cli = cliBackup
		cfg = cfgBackup
		dm = dmBackup
		pd = pdBackup
	})

	Context("Test InitClients", func() {
		var opt Option
		BeforeEach(func() {
			cli = nil
			cfg = nil
			dm = nil
			pd = nil
			opt = Option{}
		})

		It("vela status", func() {
			command := []string{"vela", "status"}
			opt = parseOption(command)
			Expect(opt.Must).Should(Equal([]Tool{DynamicClient}))

			opt.init()
			Expect(cli).ShouldNot(BeNil())
		})

		It("vela def gen-api", func() {
			command := []string{"vela", "def", "gen-api"}
			opt = parseOption(command)
			Expect(len(opt.Must)).Should(Equal(0))
		})

		It("vela log", func() {
			command := []string{"vela", "logs"}
			opt = parseOption(command)
			Expect(opt.MustAll).Should(BeTrue())

			opt.init()
			for _, tool := range []any{cli, cfg, dm, pd} {
				Expect(tool).ShouldNot(BeNil())
			}
		})
	})

})
