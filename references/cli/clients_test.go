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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("parse Options", func() {
	var opt Option
	BeforeEach(func() {
		cli = nil
		cfg = nil
		dm = nil
		pd = nil
		opt = Option{}
	})

	It("parse `vela status`", func() {
		command := []string{"vela", "status"}
		opt = parseOption(command)
		Expect(opt.Must).Should(Equal([]Tool{DynamicClient}))
	})

	It("parse `vela def gen-api`", func() {
		command := []string{"vela", "def", "gen-api"}
		parseOption(command)
		Expect(opt.Must).Should(Equal([]Tool{}))
	})

})
