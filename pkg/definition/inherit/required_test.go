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

package inherit

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// requiredBase marks one of its outputs +required and leaves the other optional.
const requiredBase = `
output: {apiVersion: "apps/v1", kind: "Deployment"}
outputs: {
	// +required
	svc: {apiVersion: "v1", kind: "Service"}
	netpol: {apiVersion: "networking.k8s.io/v1", kind: "NetworkPolicy"}
}
parameter: {}
`

// requiredBlockBase marks the whole outputs block, so every output is required.
const requiredBlockBase = `
output: {apiVersion: "apps/v1", kind: "Deployment"}
// +required
outputs: {
	svc: {apiVersion: "v1", kind: "Service"}
	netpol: {apiVersion: "networking.k8s.io/v1", kind: "NetworkPolicy"}
}
parameter: {}
`

var _ = Describe("+required", func() {
	child := func(tpl string) Level { return Level{Name: "child", Template: silentMiddle + tpl} }

	It("reads a marked output and a marked block", func() {
		Expect(requiredOutputs(Level{Name: "base", Template: requiredBase}, ComponentSurface)).
			To(ConsistOf("outputs.svc"))
		Expect(requiredOutputs(Level{Name: "base", Template: requiredBlockBase}, ComponentSurface)).
			To(ConsistOf("outputs.*"))
	})

	It("refuses a child that leaves out a required output", func() {
		_, err := renderComponent(child("$inherit: {outputs: {svc: false}}\n"), Level{Name: "base", Template: requiredBase})
		Expect(err).To(MatchError(ContainSubstring("component child leaves out outputs.svc, which base marks +required")))
	})

	It("lets a child leave out an optional output", func() {
		_, err := renderComponent(child("$inherit: {outputs: {netpol: false}}\n"), Level{Name: "base", Template: requiredBase})
		Expect(err).NotTo(HaveOccurred())
	})

	It("holds every output of a block marked +required", func() {
		_, err := renderComponent(child("$inherit: {outputs: {netpol: false}}\n"), Level{Name: "base", Template: requiredBlockBase})
		Expect(err).To(MatchError(ContainSubstring("leaves out outputs.netpol, which base marks +required")))
	})

	It("holds a root's required output against a grandchild", func() {
		leaf := child("$inherit: {outputs: {svc: false}}\n")
		middle := Level{Name: "middle", Template: silentMiddle}
		_, err := renderComponent(leaf, middle, Level{Name: "base", Template: requiredBase})
		Expect(err).To(MatchError(ContainSubstring("component child leaves out outputs.svc, which base marks +required")))
	})

	It("matches a generated required output as a pattern", func() {
		patterned := Level{Name: "base", Template: "output: {kind: \"Deployment\"}\noutputs: {\n" +
			"\tfor k, v in {a: 1} {\n\t\t// +required\n\t\t\"extra-\\(k)\": {kind: \"ConfigMap\"}\n\t}\n}\nparameter: {}\n"}
		_, err := renderComponent(child("$inherit: {outputs: {\"extra-a\": false}}\n"), patterned)
		Expect(err).To(MatchError(ContainSubstring("leaves out outputs.extra-a, which base marks +required")))
	})

	It("holds a trait's required patchOutputs", func() {
		traitBase := Level{Name: "base-trait", Template: "patch: {}\npatchOutputs: {\n\t// +required\n\tpdb: {kind: \"PodDisruptionBudget\"}\n}\nparameter: {}\n"}
		_, err := renderTrait(Level{Name: "child-trait", Template: silentMiddle + "$inherit: {patchOutputs: {pdb: false}}\n"}, traitBase)
		Expect(err).To(MatchError(ContainSubstring("trait child-trait leaves out patchOutputs.pdb, which base-trait marks +required")))
	})

	It("is checked by CheckRequired when a definition is written", func() {
		err := CheckRequired("child", child("$inherit: {outputs: {svc: false}}\n"),
			[]Level{{Name: "base", Template: requiredBase}}, ComponentSurface)
		Expect(err).To(MatchError(ContainSubstring("which base marks +required")))
	})
})
