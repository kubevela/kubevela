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

var _ = Describe("a child of the webservice-shaped parent", func() {
	renderWith := func(child string) (*Result, error) {
		return Render(context.Background(),
			[]Level{{Name: "tenant-webservice", Template: child}, {Name: "webservice", Template: parentTemplate}},
			`parameter: {image: "NGINX", tenant: "acme"}`, contextFile, ComponentSurface, SameCompiler(testCompile()))
	}

	// The parent sets spec.replicas from `replicas: *1 | int`. Its output crosses
	// as a value, so the field is still a disjunction when the child merges onto
	// it and the child would win. A child adds to its parent and never changes
	// it, so the value has to go up through `$super: properties` instead.
	It("is refused when it changes a value from a defaulted parameter", func() {
		_, err := renderWith(`
$super: properties: {image: parameter.image}

output: spec: replicas: 5

parameter: {
	tenant: string
}
`)
		Expect(err).To(MatchError(ContainSubstring("sets output.spec.replicas, which webservice already sets")))
	})

	It("is refused when it takes the output over", func() {
		_, err := renderWith(`
$inherit: {output: false}

$super: properties: {image: parameter.image}

output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: context.name
}

parameter: {
	tenant: string
}
`)
		Expect(err).To(MatchError(ContainSubstring("`$inherit: {output: false}` is not allowed")))
	})

	// What the parent rendered stays readable on `$super`, so a child can build an
	// output of its own from a piece of it.
	It("can build a new output from what the parent rendered", func() {
		res, err := renderWith(`
$super: properties: {image: parameter.image}

outputs: image: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: context.name + "-image"
	data: image: $super.output.spec.template.spec.containers[0].image
}

parameter: {
	tenant: string
}
`)
		Expect(err).NotTo(HaveOccurred())
		image, err := res.Value.LookupPath(cue.ParsePath("outputs.image.data.image")).String()
		Expect(err).NotTo(HaveOccurred())
		Expect(image).To(Equal("nginx"))
		Expect(res.Value.LookupPath(cue.ParsePath("output.kind")).Exists()).To(BeTrue(), "the parent's output was merged")
		Expect(res.Value.LookupPath(cue.ParsePath("outputs.svc")).Exists()).To(BeTrue(), "and its outputs kept")
	})
})
