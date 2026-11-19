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

// The parent's message reads an output the child replaced with one that lacks
// the label it reads, which is what a child taking over its message is for.
const readsReplacedOutput = `message: "ingress: " + context.outputs.netpol.metadata.labels["example.com/policy"]`

var _ = Describe("a status message composed down an inheritance chain", func() {
	ctx := map[string]interface{}{
		"output":  map[string]interface{}{"spec": map[string]interface{}{"replicas": 1}},
		"outputs": map[string]interface{}{"netpol": map[string]interface{}{"metadata": map[string]interface{}{"name": "shop"}}},
	}

	It("uses the child's own message without evaluating the parent's", func() {
		got, err := statusMessageChain(ctx, []Snippets{
			{Custom: `message: "ingress: from ingress-nginx"`},
			{Custom: readsReplacedOutput},
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("ingress: from ingress-nginx"))
	})

	It("skips a silent middle to reach the child's message", func() {
		got, err := statusMessageChain(ctx, []Snippets{
			{Custom: `message: "from the leaf"`},
			{},
			{Custom: readsReplacedOutput},
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("from the leaf"))
	})

	It("still evaluates the parent when the child reads $super.message", func() {
		_, err := statusMessageChain(ctx, []Snippets{
			{Custom: `message: "\($super.message), and more"`},
			{Custom: readsReplacedOutput},
		}, nil)
		Expect(err).To(HaveOccurred())
	})

	It("extends the parent's message through $super.message across a silent middle", func() {
		got, err := statusMessageChain(ctx, []Snippets{
			{Custom: `message: "\($super.message), and the leaf"`},
			{},
			{Custom: `message: "from the root"`},
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("from the root, and the leaf"))
	})
})

var _ = Describe("which levels a status message reads", func() {
	ctx := map[string]interface{}{
		"output":  map[string]interface{}{"spec": map[string]interface{}{"replicas": 1}},
		"outputs": map[string]interface{}{"netpol": map[string]interface{}{"metadata": map[string]interface{}{"name": "shop"}}},
	}

	It("does not evaluate the parent for $super written inside a string", func() {
		got, err := statusMessageChain(ctx, []Snippets{
			{Custom: `message: "$super is not read here"`},
			{Custom: readsReplacedOutput},
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("$super is not read here"))
	})

	It("reports a $super.message read when no level above states a message", func() {
		_, err := statusMessageChain(ctx, []Snippets{
			{Custom: `message: "\($super.message), and more"`},
			{},
		}, nil)
		Expect(err).To(HaveOccurred())
	})
})
