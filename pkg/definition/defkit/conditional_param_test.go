/*
Copyright 2025 The KubeVela Authors.

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

package defkit_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("ConditionalParam", func() {

	Context("ConditionalParamBlock", func() {
		It("should create a block with branches", func() {
			b1 := defkit.WhenParam(defkit.Bool("x").Eq(true)).Params(defkit.String("a"))
			b2 := defkit.WhenParam(defkit.Bool("x").Eq(false)).Params(defkit.String("b"))

			block := defkit.ConditionalParams(b1, b2)
			Expect(block.Branches()).To(HaveLen(2))
			Expect(block.Branches()[0]).To(Equal(b1))
			Expect(block.Branches()[1]).To(Equal(b2))
		})

		It("should create a block with a single branch", func() {
			b1 := defkit.WhenParam(defkit.Bool("x").Eq(true)).Params(defkit.String("a"))
			block := defkit.ConditionalParams(b1)
			Expect(block.Branches()).To(HaveLen(1))
		})

		It("should create an empty block with no branches", func() {
			block := defkit.ConditionalParams()
			Expect(block.Branches()).To(BeEmpty())
		})
	})

	Context("WhenParam / ConditionalBranch", func() {
		It("should create a branch with condition", func() {
			cond := defkit.Bool("flag").Eq(true)
			branch := defkit.WhenParam(cond)
			Expect(branch.Condition()).To(Equal(cond))
			Expect(branch.GetParams()).To(BeEmpty())
			Expect(branch.GetValidators()).To(BeEmpty())
		})

		It("should chain Params and return them via GetParams", func() {
			p1 := defkit.String("name")
			p2 := defkit.Int("count")
			branch := defkit.WhenParam(defkit.Bool("x").Eq(true)).
				Params(p1, p2)

			Expect(branch.GetParams()).To(HaveLen(2))
			Expect(branch.GetParams()[0]).To(Equal(p1))
			Expect(branch.GetParams()[1]).To(Equal(p2))
		})

		It("should accumulate params across multiple Params calls", func() {
			branch := defkit.WhenParam(defkit.Bool("x").Eq(true)).
				Params(defkit.String("a")).
				Params(defkit.String("b"))

			Expect(branch.GetParams()).To(HaveLen(2))
		})

		It("should chain Validators and return them via GetValidators", func() {
			v1 := defkit.Validate("check1").WithName("_v1")
			v2 := defkit.Validate("check2").WithName("_v2")

			branch := defkit.WhenParam(defkit.Bool("x").Eq(true)).
				Params(defkit.String("name")).
				Validators(v1, v2)

			Expect(branch.GetValidators()).To(HaveLen(2))
			Expect(branch.GetValidators()[0]).To(Equal(v1))
			Expect(branch.GetValidators()[1]).To(Equal(v2))
		})

		It("should accumulate validators across multiple Validators calls", func() {
			branch := defkit.WhenParam(defkit.Bool("x").Eq(true)).
				Validators(defkit.Validate("a").WithName("_a")).
				Validators(defkit.Validate("b").WithName("_b"))

			Expect(branch.GetValidators()).To(HaveLen(2))
		})

		It("should preserve condition through full chain", func() {
			cond := defkit.Bool("enabled").Eq(false)
			branch := defkit.WhenParam(cond).
				Params(defkit.String("name"), defkit.Int("count")).
				Validators(defkit.Validate("check").WithName("_v"))

			Expect(branch.Condition()).To(Equal(cond))
			Expect(branch.GetParams()).To(HaveLen(2))
			Expect(branch.GetValidators()).To(HaveLen(1))
		})
	})
})
