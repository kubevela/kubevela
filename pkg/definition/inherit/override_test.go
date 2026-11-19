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
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// overrideBase sets fields three ways: concretely, from a defaulted parameter,
// and as an open constraint it leaves to whoever extends it.
const overrideBase = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: "web"
	spec: {
		replicas: parameter.replicas
		paused:   bool
		template: spec: containers: [{name: "main", image: "nginx"}]
	}
}
outputs: {
	svc: {
		apiVersion: "v1"
		kind:       "Service"
		spec: type: "ClusterIP"
	}
}
parameter: replicas: *1 | int
`

var _ = Describe("a child only adds to what its parent set", func() {
	base := Level{Name: "base", Template: overrideBase}
	child := func(tpl string) Level { return Level{Name: "child", Template: silentMiddle + tpl} }

	DescribeTable("refuses a child that changes a field the parent sets from a parameter, pointing at $super",
		func(tpl, path string) {
			_, err := renderComponent(child(tpl), base)
			Expect(err).To(MatchError(ContainSubstring(
				"component child sets " + path + ", which base already sets; " +
					"pass a value to base through `$super: properties` instead")))
		},
		Entry("a field from a defaulted parameter", "output: spec: replicas: 5\n", "output.spec.replicas"),
		Entry("the default itself, which would pin it", "output: spec: replicas: 1\n", "output.spec.replicas"),
	)

	DescribeTable("refuses a child that changes a field the parent fixes, without pointing at $super",
		func(tpl, path, value string) {
			_, err := renderComponent(child(tpl), base)
			Expect(err).To(MatchError(ContainSubstring(
				"component child sets " + path + ", but base fixes it to " + value + " and a child cannot change it; " +
					"to use another value, write a definition that does not extend base")))
		},
		Entry("a concrete field", "output: metadata: name: \"other\"\n", "output.metadata.name", `"web"`),
		Entry("the kind", "output: kind: \"StatefulSet\"\n", "output.kind", `"Deployment"`),
		Entry("a field of a list element", "output: spec: template: spec: containers: [{image: \"busybox\"}]\n",
			"output.spec.template.spec.containers[0].image", `"nginx"`),
	)

	Describe("a field the parent fixes inside one of its outputs", func() {
		It("points at leaving the output out and building it from $super", func() {
			_, err := renderComponent(child("outputs: svc: spec: type: \"NodePort\"\n"), base)
			Expect(err).To(MatchError(ContainSubstring(
				"component child sets outputs.svc.spec.type, but base fixes it to \"ClusterIP\"; to write your own svc, " +
					"leave base's out with `$inherit: {outputs: {svc: false}}` and build it from `$super.outputs.svc`")))
		})

		It("keeps the separate-definition hint when the output is +required", func() {
			required := Level{Name: "base", Template: strings.Replace(overrideBase, "\tsvc: {", "\t// +required\n\tsvc: {", 1)}
			_, err := renderComponent(child("outputs: svc: spec: type: \"NodePort\"\n"), required)
			Expect(err).To(MatchError(ContainSubstring(
				"component child sets outputs.svc.spec.type, but base fixes it to \"ClusterIP\" and a child cannot change it; " +
					"to use another value, write a definition that does not extend base")))
		})

		It("quotes a key that is not an identifier", func() {
			quoted := Level{Name: "base", Template: "output: {kind: \"Deployment\"}\noutputs: \"my-svc\": {kind: \"Service\", spec: type: \"ClusterIP\"}\nparameter: {}\n"}
			_, err := renderComponent(child("outputs: \"my-svc\": spec: type: \"NodePort\"\n"), quoted)
			Expect(err).To(MatchError(ContainSubstring(
				"leave base's out with `$inherit: {outputs: {\"my-svc\": false}}` and build it from `$super.outputs.\"my-svc\"`")))
		})

		It("names patchOutputs for a trait", func() {
			traitBase := Level{Name: "base-trait", Template: "patch: {}\npatchOutputs: pdb: {kind: \"PodDisruptionBudget\", spec: minAvailable: 1}\nparameter: {}\n"}
			_, err := renderTrait(Level{Name: "child-trait", Template: silentMiddle + "patchOutputs: pdb: spec: minAvailable: 2\n"}, traitBase)
			Expect(err).To(MatchError(ContainSubstring(
				"trait child-trait sets patchOutputs.pdb.spec.minAvailable, but base-trait fixes it to 1; to write your own pdb, " +
					"leave base-trait's out with `$inherit: {patchOutputs: {pdb: false}}` and build it from `$super.patchOutputs.pdb`")))
		})
	})

	It("names the ancestor that fixes a field when a silent middle stands between", func() {
		leaf := Level{Name: "leaf", Template: silentMiddle + "output: kind: \"StatefulSet\"\n"}
		middle := Level{Name: "middle", Template: silentMiddle}
		_, err := renderComponent(leaf, middle, base)
		Expect(err).To(MatchError(ContainSubstring("component leaf sets output.kind, but base fixes it to \"Deployment\"")))
	})

	DescribeTable("accepts a child that only adds",
		func(tpl string) {
			_, err := renderComponent(child(tpl), base)
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("a new field", "output: metadata: labels: team: \"a\"\n"),
		Entry("a field the parent left open", "output: spec: paused: true\n"),
		Entry("a new field inside a list element",
			"output: spec: template: spec: containers: [{resources: limits: cpu: \"1\"}]\n"),
		Entry("the same concrete value again", "output: kind: \"Deployment\"\n"),
		Entry("its own entry for an output it left out",
			"$inherit: {outputs: {svc: false}}\noutputs: svc: {apiVersion: \"v1\", kind: \"Service\", spec: type: \"NodePort\"}\n"),
		Entry("a value passed the right way", "$super: properties: replicas: 5\n"),
		Entry("the parent's own value read back from $super",
			"output: metadata: name: $super.output.metadata.name\n"),
		Entry("an open constraint over a fixed field", "output: metadata: name: string\n"),
	)

	It("keeps refusing any child value over a field from a defaulted parameter, even one read back from $super", func() {
		_, err := renderComponent(child("output: spec: replicas: $super.output.spec.replicas\n"), base)
		Expect(err).To(MatchError(ContainSubstring("component child sets output.spec.replicas, which base already sets")))
	})

	It("refuses a grandchild that changes the root's field through a silent middle", func() {
		leaf := Level{Name: "leaf", Template: silentMiddle + "output: spec: replicas: 5\n"}
		middle := Level{Name: "middle", Template: silentMiddle}
		_, err := renderComponent(leaf, middle, base)
		Expect(err).To(MatchError(ContainSubstring("component leaf sets output.spec.replicas, which middle already sets")))
	})

	It("refuses a trait that changes its parent's patch", func() {
		traitBase := Level{Name: "base-trait", Template: "patch: spec: replicas: *2 | int\nparameter: {}\n"}
		_, err := renderTrait(Level{Name: "child-trait", Template: silentMiddle + "patch: spec: replicas: 3\n"}, traitBase)
		Expect(err).To(MatchError(ContainSubstring("trait child-trait sets patch.spec.replicas, which base-trait already sets")))
	})

	Context("when a definition is written", func() {
		check := func(tpl string) error {
			self := Level{Name: "child", Template: silentMiddle + tpl}
			own := cuecontext.New().CompileString(tpl + "\n$super: _\nparameter: _\ncontext: _\n")
			Expect(own.Err()).NotTo(HaveOccurred())
			return CheckOverrides(context.Background(), "child", self, own, []Level{base}, ComponentSurface, admissionCompile())
		}

		It("refuses a field from a defaulted parameter before any Application renders it", func() {
			Expect(check("output: spec: replicas: 5\n")).To(MatchError(ContainSubstring(
				"component child sets output.spec.replicas, which base already sets")))
		})

		It("says a fixed field is fixed before any Application renders it", func() {
			Expect(check("output: kind: \"StatefulSet\"\n")).To(MatchError(ContainSubstring(
				"component child sets output.kind, but base fixes it to \"Deployment\" and a child cannot change it; " +
					"to use another value, write a definition that does not extend base")))
		})

		It("accepts an addition", func() {
			Expect(check("output: metadata: labels: team: \"a\"\n")).To(Succeed())
		})

		It("leaves a fixed field set from the child's own parameter to the render, where it is bound", func() {
			tpl := "output: metadata: name: parameter.name\nparameter: name: string\n"
			Expect(check(tpl)).To(Succeed())
			_, err := Render(context.Background(), []Level{{Name: "child", Template: silentMiddle + tpl}, base},
				`parameter: name: "other"`, contextFile, ComponentSurface, SameCompiler(testCompile()))
			Expect(err).To(MatchError(ContainSubstring("component child sets output.metadata.name, but base fixes it to \"web\"")))
		})
	})
})

// admissionCompile compiles a level the way admission does: on its own, with
// nothing bound, `parameter` and `context` opened rather than given.
func admissionCompile() CompileFunc {
	return func(_ context.Context, src string) (cue.Value, error) {
		v := cuecontext.New().CompileString(src + "\ncontext: _\nparameter: _\n")
		return v, v.Err()
	}
}
