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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// finalBase marks a field of its output and one of its outputs +final.
const finalBase = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: template: spec: {
		// +final
		securityContext: {runAsNonRoot: true}
		containers: [{name: "main", image: "nginx"}]
	}
}
outputs: {
	// +final
	netpol: {
		apiVersion: "networking.k8s.io/v1"
		kind:       "NetworkPolicy"
		spec: podSelector: {}
	}
	svc: {
		apiVersion: "v1"
		kind:       "Service"
	}
}
parameter: {}
`

const silentMiddle = "$super: properties: {}\n"

func finalChain(levels ...Level) error {
	_, err := Render(context.Background(), levels, `parameter: {}`, contextFile,
		ComponentSurface, SameCompiler(testCompile()))
	return err
}

func pathsOf(template string, s Surface) []string {
	paths := finalPaths(Level{Name: "base", Template: template}, s)
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, p.String())
	}
	return out
}

var _ = Describe("+final", func() {
	Context("reading the markers off a template", func() {
		It("finds a field marked in block form", func() {
			Expect(pathsOf(finalBase, ComponentSurface)).To(ConsistOf(
				"output.spec.template.spec.securityContext", "outputs.netpol"))
		})

		It("marks the field the comment is written above, not the innermost one of a shorthand", func() {
			Expect(pathsOf("output: spec: {\n\t// +final\n\tsecurityContext: runAsNonRoot: true\n}\n", ComponentSurface)).
				To(ConsistOf("output.spec.securityContext"))
		})

		It("marks a whole surface field", func() {
			Expect(pathsOf("// +final\noutput: {kind: \"Deployment\"}\n", ComponentSurface)).
				To(ConsistOf("output"))
		})

		It("turns an interpolated key into a pattern", func() {
			tpl := "outputs: {\n\tfor k, v in parameter.extra {\n\t\t// +final\n\t\t\"extra-\\(k)\": {kind: \"ConfigMap\"}\n\t}\n}\nparameter: extra: [string]: string\n"
			Expect(pathsOf(tpl, ComponentSurface)).To(ConsistOf("outputs.extra-*"))
		})

		It("ignores fields outside the surface and tags that only start with +final", func() {
			tpl := "// +final\nparameter: x: string\noutput: {\n\t// +finalize\n\tkind: \"Deployment\"\n}\n"
			Expect(pathsOf(tpl, ComponentSurface)).To(BeEmpty())
		})

		It("accepts text after the tag", func() {
			Expect(pathsOf("output: {\n\t// +final keep the pod non-root\n\tspec: {}\n}\n", ComponentSurface)).
				To(ConsistOf("output.spec"))
		})
	})

	Context("rendering a chain", func() {
		base := Level{Name: "base", Template: finalBase}

		It("lets a child add next to a final field", func() {
			child := Level{Name: "child", Template: silentMiddle + "output: metadata: labels: team: \"a\"\n"}
			Expect(finalChain(child, base)).To(Succeed())
		})

		It("refuses a child that adds to a final field", func() {
			child := Level{Name: "child", Template: silentMiddle +
				"output: spec: template: spec: securityContext: runAsUser: 1000\n"}
			err := finalChain(child, base)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(
				"component child adds to output.spec.template.spec.securityContext, which base marks +final"))
		})

		It("refuses a child that restates a final field's own value", func() {
			child := Level{Name: "child", Template: silentMiddle +
				"output: spec: template: spec: securityContext: runAsNonRoot: true\n"}
			Expect(finalChain(child, base)).To(MatchError(ContainSubstring("which base marks +final")))
		})

		It("refuses a child that adds to a final outputs key", func() {
			child := Level{Name: "child", Template: silentMiddle +
				"outputs: netpol: metadata: labels: team: \"a\"\n"}
			Expect(finalChain(child, base)).To(MatchError(ContainSubstring("adds to outputs.netpol, which base marks +final")))
		})

		It("lets a child add to an outputs key the parent did not mark", func() {
			child := Level{Name: "child", Template: silentMiddle + "outputs: svc: metadata: labels: team: \"a\"\n"}
			Expect(finalChain(child, base)).To(Succeed())
		})

		It("refuses a grandchild that adds to the root's final field through a silent middle", func() {
			leaf := Level{Name: "leaf", Template: silentMiddle +
				"output: spec: template: spec: securityContext: runAsUser: 1000\n"}
			middle := Level{Name: "middle", Template: silentMiddle}
			Expect(finalChain(leaf, middle, base)).To(MatchError(ContainSubstring(
				"component leaf adds to output.spec.template.spec.securityContext, which base marks +final")))
		})

		It("refuses a middle level that adds to the root's final field", func() {
			leaf := Level{Name: "leaf", Template: silentMiddle}
			middle := Level{Name: "middle", Template: silentMiddle +
				"output: spec: template: spec: securityContext: runAsUser: 1000\n"}
			Expect(finalChain(leaf, middle, base)).To(MatchError(ContainSubstring("which base marks +final")))
		})

		It("holds a middle level's own marker against the leaf", func() {
			leaf := Level{Name: "leaf", Template: silentMiddle + "output: metadata: labels: team: \"a\"\n"}
			middle := Level{Name: "middle", Template: silentMiddle + "output: {\n\t// +final\n\tmetadata: {}\n}\n"}
			Expect(finalChain(leaf, middle, base)).To(MatchError(ContainSubstring(
				"component leaf adds to output.metadata, which middle marks +final")))
		})

		It("does not hold a marker on an output the child left out with $inherit", func() {
			child := Level{Name: "child", Template: silentMiddle + "$inherit: {outputs: {netpol: false}}\n" +
				"outputs: netpol: {apiVersion: \"networking.k8s.io/v1\", kind: \"NetworkPolicy\"}\n"}
			Expect(finalChain(child, base)).To(Succeed())
		})

		It("does not hold the root's marker past a middle level that left the output out", func() {
			leaf := Level{Name: "leaf", Template: silentMiddle + "outputs: netpol: metadata: name: \"x\"\n"}
			middle := Level{Name: "middle", Template: silentMiddle + "$inherit: {outputs: {netpol: false}}\n" +
				"outputs: netpol: {apiVersion: \"networking.k8s.io/v1\", kind: \"NetworkPolicy\"}\n"}
			Expect(finalChain(leaf, middle, base)).To(Succeed())
		})

		It("matches an interpolated final key as a pattern", func() {
			patterned := Level{Name: "base", Template: "output: {kind: \"Deployment\"}\noutputs: {\n" +
				"\tfor k, v in {a: 1} {\n\t\t// +final\n\t\t\"extra-\\(k)\": {kind: \"ConfigMap\"}\n\t}\n}\nparameter: {}\n"}
			child := Level{Name: "child", Template: silentMiddle + "outputs: \"extra-a\": metadata: name: \"x\"\n"}
			Expect(finalChain(child, patterned)).To(MatchError(ContainSubstring(
				"adds to outputs.extra-a, which base marks +final")))
		})

		It("holds a final field in a trait's patch", func() {
			traitBase := Level{Name: "base-trait", Template: "patch: spec: {\n\t// +final\n\treplicas: 1\n}\nparameter: {}\n"}
			child := Level{Name: "child-trait", Template: silentMiddle + "patch: spec: replicas: 2\n"}
			_, err := Render(context.Background(), []Level{child, traitBase}, `parameter: {}`, contextFile,
				TraitSurface, SameCompiler(testCompile()))
			Expect(err).To(MatchError(ContainSubstring(
				"trait child-trait adds to patch.spec.replicas, which base-trait marks +final")))
		})
	})
})
