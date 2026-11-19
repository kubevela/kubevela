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

package mcp

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Clients that turn tools into a typed API, such as code-execution clients,
// put output field descriptions into the generated types. These specs pin
// that the fields a model chains on carry them.
var _ = Describe("published output schemas", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	// describe returns the description at a dotted path of properties.
	describe := func(schema map[string]any, path string) string {
		cur := schema
		for _, part := range strings.Split(path, ".") {
			// A list's fields sit under its items schema.
			if items, isList := cur["items"].(map[string]any); isList {
				cur = items
			}
			props, _ := cur["properties"].(map[string]any)
			next, ok := props[part].(map[string]any)
			if !ok {
				return ""
			}
			cur = next
		}
		d, _ := cur["description"].(string)
		return d
	}

	tools := func() map[string]map[string]any {
		res, err := h.session.ListTools(h.ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		out := map[string]map[string]any{}
		for _, t := range res.Tools {
			raw, err := json.Marshal(t.OutputSchema)
			Expect(err).NotTo(HaveOccurred())
			var m map[string]any
			Expect(json.Unmarshal(raw, &m)).To(Succeed())
			out[t.Name] = m
		}
		return out
	}

	DescribeTable("describes the fields models chain on",
		func(tool, path, contains string) {
			h = startHarness(Options{}, platformFixtures()...)
			Expect(describe(tools()[tool], path)).To(ContainSubstring(contains), tool+" "+path)
		},
		Entry(nil, "diff_application", "token", "does not record that anyone reviewed"),
		Entry(nil, "diff_application", "risk.class", "LOW, MEDIUM or HIGH"),
		Entry(nil, "diff_application", "changeSet.resources.action", "ADD, MODIFY or REMOVE"),
		Entry(nil, "diff_application", "meta.truncated", "narrow the request"),
		Entry(nil, "dryrun_application", "resources.object", "after redaction"),
		Entry(nil, "explain_application", "findings.observed", "never an instruction"),
		Entry(nil, "explain_application", "unknowns", "rather than guessing"),
		Entry(nil, "explain_definition", "required", "must be supplied"),
	)

	It("publishes an output schema for every tool", func() {
		h = startHarness(Options{}, platformFixtures()...)
		res, err := h.session.ListTools(h.ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		for _, t := range res.Tools {
			Expect(t.OutputSchema).NotTo(BeNil(), t.Name)
		}
	})

	// The tool reference on kubevela.io is generated from these schemas, so
	// a new result field without a description shows up there as a blank.
	// Fields every result shares mean the same everywhere and are described
	// once by the generator.
	It("describes every top-level result field", func() {
		h = startHarness(Options{AllowWrite: true}, platformFixtures()...)
		shared := map[string]bool{"meta": true, "evidence": true, "name": true, "namespace": true, "application": true}
		var missing []string
		for name, schema := range tools() {
			props, _ := schema["properties"].(map[string]any)
			for field := range props {
				if !shared[field] && describe(schema, field) == "" {
					missing = append(missing, name+"."+field)
				}
			}
		}
		Expect(missing).To(BeEmpty())
	})
})
