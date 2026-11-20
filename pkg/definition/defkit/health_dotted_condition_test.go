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
	"cuelang.org/go/cue/parser"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("Health policy with dotted condition types", func() {
	It("generates parseable CUE for ACK-style condition names", func() {
		// ACK controllers name their conditions with dots ("ACK.ResourceSynced").
		h := defkit.Health()
		comp := defkit.NewComponent("ack-resource").
			HealthPolicyExpr(h.Condition("ACK.ResourceSynced").IsTrue())

		policy := comp.GetHealthPolicy()

		// The hidden preamble variable derived from the condition type must be
		// a legal CUE identifier, or the whole definition fails to parse.
		_, err := parser.ParseFile("policy.cue", policy)
		Expect(err).NotTo(HaveOccurred(), "generated health policy must be valid CUE:\n%s", policy)

		// The condition type itself survives untouched inside the string literal.
		Expect(policy).To(ContainSubstring(`"ACK.ResourceSynced"`))
	})

	It("keeps plain condition names byte-identical", func() {
		h := defkit.Health()
		comp := defkit.NewComponent("plain").
			HealthPolicyExpr(h.Condition("Ready").IsTrue())

		Expect(comp.GetHealthPolicy()).To(ContainSubstring("_readyCond:"))
	})

	It("keeps underscore-bearing condition names byte-identical", func() {
		h := defkit.Health()
		comp := defkit.NewComponent("underscore").
			HealthPolicyExpr(h.Condition("Custom_Ready").IsTrue())

		Expect(comp.GetHealthPolicy()).To(ContainSubstring("_custom_readyCond:"))
	})
})
