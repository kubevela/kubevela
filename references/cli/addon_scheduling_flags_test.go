/*
Copyright 2026 The KubeVela Authors.

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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
)

var _ = Describe("Addon scheduling flags on a reused command", func() {
	AfterEach(func() {
		addonNodeSelector = nil
		addonSchedulingFile = ""
	})

	It("parses --nodeSelector again after the flags are cleared", func() {
		flags := pflag.NewFlagSet("enable", pflag.ContinueOnError)
		flags.StringToStringVar(&addonNodeSelector, "nodeSelector", nil, "")

		Expect(flags.Parse([]string{"--nodeSelector=disktype=ssd"})).To(Succeed())
		first := map[string]interface{}{}
		Expect(injectSchedulingArgs(first)).To(Succeed())

		Expect(func() {
			Expect(flags.Parse([]string{"--nodeSelector=zone=a"})).To(Succeed())
		}).NotTo(Panic())
		second := map[string]interface{}{}
		Expect(injectSchedulingArgs(second)).To(Succeed())

		props, err := pkgaddon.ExtractSchedulingFromArgs(second)
		Expect(err).NotTo(HaveOccurred())
		Expect(props.NodeSelector).To(Equal(map[string]string{"zone": "a"}))
	})
})
