/*
Copyright 2025 The KubeVela Authors.

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

package defkit_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("Template", func() {

	Context("NewTemplate initialization", func() {
		It("should initialize all slices and maps", func() {
			tpl := defkit.NewTemplate()
			Expect(tpl.GetHelpers()).To(BeEmpty())
			Expect(tpl.GetHelpers()).NotTo(BeNil())
			Expect(tpl.GetStructArrayHelpers()).To(BeEmpty())
			Expect(tpl.GetStructArrayHelpers()).NotTo(BeNil())
			Expect(tpl.GetConcatHelpers()).To(BeEmpty())
			Expect(tpl.GetConcatHelpers()).NotTo(BeNil())
			Expect(tpl.GetDedupeHelpers()).To(BeEmpty())
			Expect(tpl.GetDedupeHelpers()).NotTo(BeNil())
			Expect(tpl.GetOutputs()).To(BeEmpty())
			Expect(tpl.GetOutputs()).NotTo(BeNil())
			Expect(tpl.GetOutputGroups()).To(BeNil())
		})
	})

	Context("Helper filtering (BeforeOutput / AfterOutput)", func() {
		It("should separate helpers by afterOutput flag", func() {
			ports := defkit.Array("ports")

			tpl := defkit.NewTemplate()
			// Register a helper that appears before output (default)
			tpl.Helper("beforeHelper").
				From(ports).
				Build()
			// Register a helper that appears after output
			tpl.Helper("afterHelper").
				From(ports).
				AfterOutput().
				Build()

			before := tpl.GetHelpersBeforeOutput()
			after := tpl.GetHelpersAfterOutput()
			all := tpl.GetHelpers()

			Expect(all).To(HaveLen(2))
			Expect(before).To(HaveLen(1))
			Expect(after).To(HaveLen(1))
			Expect(before[0].Name()).To(Equal("beforeHelper"))
			Expect(after[0].Name()).To(Equal("afterHelper"))
		})

		It("should return empty slices when no helpers registered", func() {
			tpl := defkit.NewTemplate()
			Expect(tpl.GetHelpersBeforeOutput()).To(BeEmpty())
			Expect(tpl.GetHelpersAfterOutput()).To(BeEmpty())
		})

		It("should return all before when no after helpers exist", func() {
			ports := defkit.Array("ports")

			tpl := defkit.NewTemplate()
			tpl.Helper("h1").From(ports).Build()
			tpl.Helper("h2").From(ports).Build()

			Expect(tpl.GetHelpersBeforeOutput()).To(HaveLen(2))
			Expect(tpl.GetHelpersAfterOutput()).To(BeEmpty())
		})
	})

	Context("OutputsGroupIf", func() {
		It("should create output groups with shared condition", func() {
			enabled := defkit.Bool("enabled")
			cond := defkit.Eq(enabled, defkit.Lit(true))

			tpl := defkit.NewTemplate()
			tpl.OutputsGroupIf(cond, func(g *defkit.OutputGroup) {
				g.Add("service", defkit.NewResource("v1", "Service"))
				g.Add("ingress", defkit.NewResource("networking.k8s.io/v1", "Ingress"))
			})

			groups := tpl.GetOutputGroups()
			Expect(groups).To(HaveLen(1))
		})

		It("should support multiple output groups", func() {
			enabled := defkit.Bool("enabled")
			debug := defkit.Bool("debug")
			cond1 := defkit.Eq(enabled, defkit.Lit(true))
			cond2 := defkit.Eq(debug, defkit.Lit(true))

			tpl := defkit.NewTemplate()
			tpl.OutputsGroupIf(cond1, func(g *defkit.OutputGroup) {
				g.Add("service", defkit.NewResource("v1", "Service"))
			})
			tpl.OutputsGroupIf(cond2, func(g *defkit.OutputGroup) {
				g.Add("configmap", defkit.NewResource("v1", "ConfigMap"))
			})

			groups := tpl.GetOutputGroups()
			Expect(groups).To(HaveLen(2))
		})

		It("should allow chaining Add calls on OutputGroup", func() {
			enabled := defkit.Bool("enabled")
			cond := defkit.Eq(enabled, defkit.Lit(true))

			tpl := defkit.NewTemplate()
			tpl.OutputsGroupIf(cond, func(g *defkit.OutputGroup) {
				g.Add("svc1", defkit.NewResource("v1", "Service")).
					Add("svc2", defkit.NewResource("v1", "Service")).
					Add("svc3", defkit.NewResource("v1", "Service"))
			})

			groups := tpl.GetOutputGroups()
			Expect(groups).To(HaveLen(1))
		})
	})

	Context("Patch methods on Template", func() {
		It("should lazily create PatchResource on first Patch() call", func() {
			tpl := defkit.NewTemplate()
			Expect(tpl.GetPatch()).To(BeNil())
			Expect(tpl.HasPatch()).To(BeFalse())

			patch := tpl.Patch()
			Expect(patch).NotTo(BeNil())
			// Patch created but no ops yet
			Expect(tpl.HasPatch()).To(BeFalse())

			patch.Set("spec.replicas", defkit.Lit(3))
			Expect(tpl.HasPatch()).To(BeTrue())
		})

		It("should return the same PatchResource on repeated calls", func() {
			tpl := defkit.NewTemplate()
			patch1 := tpl.Patch()
			patch2 := tpl.Patch()
			Expect(patch1).To(BeIdenticalTo(patch2))
		})

		It("should support PatchStrategy as fluent builder", func() {
			tpl := defkit.NewTemplate()
			result := tpl.PatchStrategy("jsonMergePatch")
			Expect(result).To(BeIdenticalTo(tpl))
			Expect(tpl.GetPatchStrategy()).To(Equal("jsonMergePatch"))
		})
	})

	Context("Output and Outputs methods", func() {
		It("should overwrite primary output on second call", func() {
			tpl := defkit.NewTemplate()
			r1 := defkit.NewResource("apps/v1", "Deployment")
			r2 := defkit.NewResource("apps/v1", "StatefulSet")
			tpl.Output(r1)
			tpl.Output(r2)
			Expect(tpl.GetOutput().Kind()).To(Equal("StatefulSet"))
		})

		It("should overwrite named output on duplicate name", func() {
			tpl := defkit.NewTemplate()
			r1 := defkit.NewResource("v1", "Service")
			r2 := defkit.NewResource("v1", "ConfigMap")
			tpl.Outputs("svc", r1)
			tpl.Outputs("svc", r2)
			Expect(tpl.GetOutputs()["svc"].Kind()).To(Equal("ConfigMap"))
		})
	})
})
