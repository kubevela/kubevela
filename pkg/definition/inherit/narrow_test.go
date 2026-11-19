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
	"context"

	"cuelang.org/go/cue"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const narrowBase = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: "web"
}
outputs: {
	svc: {
		apiVersion: "v1"
		kind:       "Service"
		spec: type: "ClusterIP"
	}
	netpol: {
		apiVersion: "networking.k8s.io/v1"
		kind:       "NetworkPolicy"
	}
}
parameter: {}
`

const narrowTraitBase = `
patch: metadata: labels: managed: "true"
patchOutputs: {
	pdb: {kind: "PodDisruptionBudget"}
	hpa: {kind: "HorizontalPodAutoscaler"}
}
parameter: {}
`

func renderComponent(levels ...Level) (*Result, error) {
	return Render(context.Background(), levels, `parameter: {}`, contextFile,
		ComponentSurface, SameCompiler(testCompile()))
}

func renderTrait(levels ...Level) (*Result, error) {
	return Render(context.Background(), levels, `parameter: {}`, contextFile,
		TraitSurface, SameCompiler(testCompile()))
}

func lookup(v cue.Value, path string) cue.Value { return v.LookupPath(cue.ParsePath(path)) }

var _ = Describe("$inherit after #7607", func() {
	base := Level{Name: "base", Template: narrowBase}
	child := func(tpl string) Level { return Level{Name: "child", Template: silentMiddle + tpl} }

	DescribeTable("refuses a directive that replaces what the parent renders",
		func(directive, want string) {
			_, err := renderComponent(child(directive), base)
			Expect(err).To(MatchError(ContainSubstring(want)))
		},
		Entry("$inherit: false", "$inherit: false\n",
			"`$inherit: false` is not allowed"),
		Entry("output", "$inherit: {output: false}\n",
			"`$inherit: {output: false}` is not allowed"),
		Entry("output spelled as a string", "$inherit: {\"out\\u0070ut\": false}\n",
			"`$inherit: {output: false}` is not allowed"),
		Entry("the whole of outputs", "$inherit: {outputs: false}\n",
			"leave out one output at a time with `$inherit: {outputs: {<name>: false}}`"),
	)

	It("refuses a trait that drops its parent's patch", func() {
		_, err := renderTrait(Level{Name: "child-trait", Template: silentMiddle + "$inherit: {patch: false}\n"},
			Level{Name: "base-trait", Template: narrowTraitBase})
		Expect(err).To(MatchError(ContainSubstring("`$inherit: {patch: false}` is not allowed")))
	})

	It("drops one of the parent's outputs and keeps the rest", func() {
		res, err := renderComponent(child("$inherit: {outputs: {svc: false}}\n"), base)
		Expect(err).NotTo(HaveOccurred())
		Expect(lookup(res.Value, "outputs.svc").Exists()).To(BeFalse())
		Expect(lookup(res.Value, "outputs.netpol.kind").String()).To(Equal("NetworkPolicy"))
		Expect(lookup(res.Value, "output.kind").String()).To(Equal("Deployment"))
	})

	It("lets a child write its own resource in place of the output it dropped", func() {
		res, err := renderComponent(child("$inherit: {outputs: {svc: false}}\n"+
			"outputs: svc: {apiVersion: \"v1\", kind: \"Service\", spec: type: \"NodePort\"}\n"), base)
		Expect(err).NotTo(HaveOccurred())
		Expect(lookup(res.Value, "outputs.svc.spec.type").String()).To(Equal("NodePort"))
	})

	It("drops one of a trait's patchOutputs", func() {
		res, err := renderTrait(Level{Name: "child-trait", Template: silentMiddle + "$inherit: {patchOutputs: {hpa: false}}\n"},
			Level{Name: "base-trait", Template: narrowTraitBase})
		Expect(err).NotTo(HaveOccurred())
		Expect(lookup(res.Value, "patchOutputs.hpa").Exists()).To(BeFalse())
		Expect(lookup(res.Value, "patchOutputs.pdb.kind").String()).To(Equal("PodDisruptionBudget"))
	})

	It("keeps an output whose drop is spelled true", func() {
		res, err := renderComponent(child("$inherit: {outputs: {svc: true}}\n"), base)
		Expect(err).NotTo(HaveOccurred())
		Expect(lookup(res.Value, "outputs.svc.kind").String()).To(Equal("Service"))
	})

	It("refuses a per-output drop that is not a boolean", func() {
		_, err := renderComponent(child("$inherit: {outputs: {svc: \"no\"}}\n"), base)
		Expect(err).To(MatchError(ContainSubstring("outputs.svc must be true or false")))
	})

	DescribeTable("refuses a per-output drop whose label names no single output",
		func(directive string) {
			_, err := renderComponent(child(directive), base)
			Expect(err).To(MatchError(ContainSubstring("must name one output")))
		},
		Entry("a pattern", "$inherit: {outputs: {[string]: false}}\n"),
		Entry("a computed label", "$inherit: {outputs: {(\"s\" + \"vc\"): false}}\n"),
		Entry("an interpolated label", "_n: \"svc\"\n$inherit: {outputs: {\"\\(_n)\": false}}\n"),
		Entry("an empty label", "$inherit: {outputs: {\"\": false}}\n"),
	)

	It("refuses a field it does not know", func() {
		_, err := renderComponent(child("$inherit: {spec: false}\n"), base)
		Expect(err).To(MatchError(ContainSubstring("$inherit in child: unknown field spec")))
	})

	It("leaves a definition field among the parent's outputs alone when dropping one", func() {
		withDef := Level{Name: "base", Template: "output: {kind: \"Deployment\"}\noutputs: {\n\t#shared: {apiVersion: \"v1\", kind: \"ConfigMap\"}\n\tsvc: #shared\n\tnetpol: {kind: \"NetworkPolicy\"}\n}\nparameter: {}\n"}
		res, err := renderComponent(child("$inherit: {outputs: {netpol: false}}\n"), withDef)
		Expect(err).NotTo(HaveOccurred())
		Expect(lookup(res.Value, "outputs.svc.kind").String()).To(Equal("ConfigMap"))
	})

	It("reports a surface field written as a list instead of panicking", func() {
		_, err := renderComponent(child("outputs: [{kind: \"ConfigMap\"}]\n"), base)
		Expect(err).To(HaveOccurred())
	})
})
