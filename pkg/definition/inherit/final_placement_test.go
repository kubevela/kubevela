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

// listFinalBase marks a field inside the first container, where the +final
// check cannot read it.
const listFinalBase = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: template: spec: containers: [{
		name: "main"
		// +final
		securityContext: runAsNonRoot: true
	}]
}
parameter: {}
`

var _ = Describe("+final inside a list item", func() {
	DescribeTable("is refused where the check could not honour it",
		func(template, want string) {
			Expect(CheckFinalPlacement(Level{Name: "base", Template: template}, ComponentSurface)).To(MatchError(want))
		},
		Entry("a field of a container", listFinalBase,
			"component base marks output.spec.template.spec.containers[0].securityContext +final, "+
				"but +final cannot be enforced on a field inside a list item; "+
				"mark the list field, output.spec.template.spec.containers, instead"),
		Entry("a field of a list nested in a list item",
			"outputs: cm: {kind: \"ConfigMap\", data: items: [{tags: [{\n\t// +final\n\tname: \"x\"\n}]}]}\nparameter: {}\n",
			"component base marks outputs.cm.data.items[0].tags[0].name +final, "+
				"but +final cannot be enforced on a field inside a list item; "+
				"mark the list field, outputs.cm.data.items[0].tags, instead"),
	)

	DescribeTable("is accepted where the check reads it",
		func(template string) {
			Expect(CheckFinalPlacement(Level{Name: "base", Template: template}, ComponentSurface)).To(Succeed())
		},
		Entry("the pod-level security context",
			"output: spec: template: spec: {\n\t// +final\n\tsecurityContext: runAsNonRoot: true\n}\nparameter: {}\n"),
		Entry("the list field itself",
			"output: spec: template: spec: {\n\t// +final\n\tcontainers: [{name: \"main\"}]\n}\nparameter: {}\n"),
		Entry("a list item with no marker", "output: spec: template: spec: containers: [{name: \"main\"}]\nparameter: {}\n"),
		Entry("a marker on a field outside the surface", "parameter: {\n\tports: [{\n\t\t// +final\n\t\tport: int\n\t}]\n}\n"),
	)

	It("refuses a child at render when its parent was written with one", func() {
		_, err := renderComponent(Level{Name: "child", Template: silentMiddle + "output: metadata: labels: team: \"a\"\n"},
			Level{Name: "base", Template: listFinalBase})
		Expect(err).To(MatchError(ContainSubstring(
			"component base marks output.spec.template.spec.containers[0].securityContext +final, but +final cannot be enforced")))
	})
})
