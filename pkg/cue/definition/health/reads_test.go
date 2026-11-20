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

package health

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("OutputsRead", func() {
	DescribeTable("names the outputs a status reads",
		func(snippet string, want []string, wantAll bool) {
			keys, all := OutputsRead(snippet)
			Expect(all).To(Equal(wantAll))
			got := []string{}
			for k := range keys {
				got = append(got, k)
			}
			Expect(got).To(ConsistOf(want))
		},
		Entry("by selector", `x: context.outputs.svc.spec`, []string{"svc"}, false),
		Entry("by string index", `x: context.outputs["svc"]`, []string{"svc"}, false),
		Entry("outputs by string index on context", `x: context["outputs"].svc`, []string{"svc"}, false),
		Entry("both by string index", `x: context["outputs"]["svc"]`, []string{"svc"}, false),
		Entry("by a computed index", "parameter: n: string\nx: context.outputs[parameter.n]", []string{}, true),
		Entry("the whole map", `x: context.outputs`, []string{}, true),
		Entry("the whole map, reached by string index", `x: context["outputs"]`, []string{}, true),
		Entry("through an alias of context", "let c = context\nx: c.outputs.svc", []string{}, true),
		Entry("context as a whole value", `x: context`, []string{}, true),
		Entry("context by a computed index", "parameter: k: string\nx: context[parameter.k]", []string{}, true),
		Entry("the output, not the outputs", `x: context.output.status.ready`, []string{}, false),
		Entry("another context field", `x: context.name`, []string{}, false),
		Entry("another context field by string index", `x: context["name"]`, []string{}, false),
		Entry("a field labelled context", "context: 1\ny: x.context.outputs.svc", []string{}, false),
	)
})
