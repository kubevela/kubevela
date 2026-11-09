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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("stage_definition and definitions by handle", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	stage := func(doc string) StageDefinitionOutput {
		var out StageDefinitionOutput
		res := h.call("stage_definition", map[string]any{"definition": doc}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		return out
	}

	It("stages a CUE definition and renders it by handle, same as inline", func() {
		h = startHarness(Options{}, platformFixtures()...)
		staged := stage(bucketCUE)
		Expect(staged.Kind).To(Equal("ComponentDefinition"))
		Expect(staged.Name).To(Equal("aws-s3-bucket"))
		Expect(staged.Handle).To(HavePrefix(defHandlePrefix))
		Expect(staged.Evidence).To(ContainElement(ContainSubstring("nothing was sent to the cluster")))

		var byHandle, inline DryRunOutput
		res := h.call("dryrun_application", map[string]any{"application": bucketDoc("acme-logs"), "definitions": []string{staged.Handle}}, &byHandle)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		res = h.call("dryrun_application", map[string]any{"application": bucketDoc("acme-logs"), "definitions": []string{bucketCUE}}, &inline)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(byHandle.Resources).To(Equal(inline.Resources), "a handle renders exactly what its content renders")
		Expect(byHandle.Evidence).To(ContainElement(ContainSubstring("1 local definitions took priority")))
	})

	It("stages the same content to the same handle", func() {
		h = startHarness(Options{}, platformFixtures()...)
		Expect(stage(bucketCUE).Handle).To(Equal(stage(bucketCUE).Handle))
	})

	It("expires a handle to an error that names the tool to call again", func() {
		old := stageTTL
		stageTTL = time.Millisecond
		DeferCleanup(func() { stageTTL = old })
		h = startHarness(Options{}, platformFixtures()...)
		staged := stage(bucketCUE)
		time.Sleep(5 * time.Millisecond)
		res := h.call("dryrun_application", map[string]any{"application": bucketDoc("acme-logs"), "definitions": []string{staged.Handle}}, nil)
		Expect(errorText(res)).To(ContainSubstring("stage_definition"))
	})

	It("assesses definition impact by handle", func() {
		h = startHarness(Options{}, append(platformFixtures(), consumerApp("shop"))...)
		staged := stage(proposedWebservice(labeledWebserviceTemplate))
		var out DefinitionImpactOutput
		res := h.call("diff_definition_impact", map[string]any{"name": "webservice", "definition": staged.Handle}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.DefinitionChanged).To(HaveValue(BeTrue()))
		Expect(out.Impacts).To(HaveLen(1))
		Expect(out.Impacts[0].ChangeSet).NotTo(BeNil())
	})

	It("refuses to stage what is not a definition", func() {
		h = startHarness(Options{}, platformFixtures()...)
		res := h.call("stage_definition", map[string]any{"definition": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: not-a-def\n"}, nil)
		Expect(errorText(res)).To(ContainSubstring("only definition kinds are accepted"))
	})

	It("bounds the cache and keeps the newest entries usable", func() {
		old := maxStagedDefinitions
		maxStagedDefinitions = 1
		DeferCleanup(func() { maxStagedDefinitions = old })
		h = startHarness(Options{}, platformFixtures()...)
		first := stage(bucketCUE)
		second := stage(proposedWebservice(labeledWebserviceTemplate))
		res := h.call("dryrun_application", map[string]any{"application": bucketDoc("acme-logs"), "definitions": []string{first.Handle}}, nil)
		Expect(errorText(res)).To(ContainSubstring("not staged or has expired"), "the oldest entry was evicted")
		var out DefinitionImpactOutput
		res = h.call("diff_definition_impact", map[string]any{"name": "webservice", "definition": second.Handle}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
	})
})
