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

package utils

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/inherit"
)

const finalParent = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: context.name
	spec: template: spec: {
		// +final
		securityContext: {runAsNonRoot: true}
		containers: [{name: context.name, image: parameter.image}]
	}
}
parameter: image: string
`

var _ = Describe("ValidateInheritedTemplate and +final", func() {
	parent := []inherit.Level{{Name: "secure-web", Template: finalParent}}

	It("accepts a child that adds next to a final field", func() {
		child := "$super: properties: {image: parameter.image}\nparameter: image: *\"nginx\" | string\n" +
			"output: metadata: labels: team: \"a\"\n"
		_, err := ValidateInheritedTemplate(context.Background(), "team-web", child, parent, inherit.ComponentSurface)
		Expect(err).NotTo(HaveOccurred())
	})

	It("refuses a child that adds to a final field when it is written", func() {
		child := "$super: properties: {image: parameter.image}\nparameter: image: *\"nginx\" | string\n" +
			"output: spec: template: spec: securityContext: runAsUser: 0\n"
		_, err := ValidateInheritedTemplate(context.Background(), "team-web", child, parent, inherit.ComponentSurface)
		Expect(err).To(MatchError(ContainSubstring(
			"component team-web adds to output.spec.template.spec.securityContext, which secure-web marks +final")))
	})

	It("refuses a grandchild that adds to the root's final field", func() {
		ancestors := []inherit.Level{
			{Name: "middle-web", Template: "$super: properties: {image: parameter.image}\nparameter: image: string\n"},
			{Name: "secure-web", Template: finalParent},
		}
		child := "$super: properties: {image: \"nginx\"}\n" +
			"output: spec: template: spec: securityContext: runAsUser: 0\n"
		_, err := ValidateInheritedTemplate(context.Background(), "leaf-web", child, ancestors, inherit.ComponentSurface)
		Expect(err).To(MatchError(ContainSubstring("which secure-web marks +final")))
	})
})
