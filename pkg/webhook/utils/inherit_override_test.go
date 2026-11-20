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

const replicasParent = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: context.name
	spec: {
		replicas: parameter.replicas
		template: spec: containers: [{name: context.name, image: parameter.image}]
	}
}
parameter: {
	image:    string
	replicas: *1 | int
}
`

var _ = Describe("ValidateInheritedTemplate and fields the parent set", func() {
	parent := []inherit.Level{{Name: "scaled-web", Template: replicasParent}}
	validate := func(child string) error {
		_, err := ValidateInheritedTemplate(context.Background(), "big-web",
			"$super: properties: {image: parameter.image}\nparameter: image: *\"nginx\" | string\n"+child,
			parent, inherit.ComponentSurface)
		return err
	}

	It("refuses a child that changes a field from a defaulted parameter when it is written", func() {
		Expect(validate("output: spec: replicas: 5\n")).To(MatchError(ContainSubstring(
			"component big-web sets output.spec.replicas, which scaled-web already sets; " +
				"pass a value to scaled-web through `$super: properties` instead")))
	})

	It("refuses $inherit: {output: false} when it is written", func() {
		Expect(validate("$inherit: {output: false}\noutput: {kind: \"Job\"}\n")).To(MatchError(ContainSubstring(
			"`$inherit: {output: false}` is not allowed in big-web")))
	})

	It("accepts a child that adds a label", func() {
		Expect(validate("output: metadata: labels: team: \"a\"\n")).To(Succeed())
	})

	It("refuses a child that leaves out a +required output when it is written", func() {
		parentWithSvc := []inherit.Level{{Name: "svc-web", Template: replicasParent +
			"outputs: {\n\t// +required\n\tsvc: {apiVersion: \"v1\", kind: \"Service\"}\n}\n"}}
		_, err := ValidateInheritedTemplate(context.Background(), "headless-web",
			"$super: properties: {image: parameter.image}\nparameter: image: *\"nginx\" | string\n"+
				"$inherit: {outputs: {svc: false}}\n",
			parentWithSvc, inherit.ComponentSurface)
		Expect(err).To(MatchError(ContainSubstring(
			"component headless-web leaves out outputs.svc, which svc-web marks +required")))
	})
})
