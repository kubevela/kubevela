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

var _ = Describe("PatchResource", func() {

	Context("NewPatchResource initialization", func() {
		It("should start with empty ops", func() {
			patch := defkit.NewPatchResource()
			Expect(patch.Ops()).To(BeEmpty())
			Expect(patch.Ops()).NotTo(BeNil())
		})
	})

	Context("Fluent chaining", func() {
		It("should chain Set calls fluently", func() {
			replicas := defkit.Int("replicas")
			image := defkit.String("image")

			patch := defkit.NewPatchResource()
			result := patch.
				Set("spec.replicas", replicas).
				Set("spec.template.spec.containers[0].image", image)

			Expect(result).To(BeIdenticalTo(patch))
			Expect(patch.Ops()).To(HaveLen(2))
		})

		It("should chain mixed operations fluently", func() {
			cpu := defkit.String("cpu")
			labels := defkit.Object("labels")

			patch := defkit.NewPatchResource()
			patch.
				Set("spec.replicas", defkit.Lit(3)).
				SetIf(cpu.IsSet(), "spec.resources.limits.cpu", cpu).
				SpreadIf(labels.IsSet(), "metadata.labels", labels).
				ForEach(labels, "metadata.annotations")

			Expect(patch.Ops()).To(HaveLen(4))
		})
	})

	Context("If/EndIf nesting", func() {
		It("should nest multiple operations inside If block", func() {
			enabled := defkit.Bool("enabled")
			cond := defkit.Eq(enabled, defkit.Lit(true))

			patch := defkit.NewPatchResource()
			patch.If(cond).
				Set("spec.replicas", defkit.Lit(3)).
				SetIf(defkit.String("cpu").IsSet(), "spec.resources.limits.cpu", defkit.String("cpu")).
				SpreadIf(defkit.Object("labels").IsSet(), "metadata.labels", defkit.Object("labels")).
				ForEach(defkit.Object("annotations"), "metadata.annotations").
				PatchKey("spec.containers", "name", defkit.NewArrayElement().Set("name", defkit.Lit("sidecar"))).
				EndIf()

			Expect(patch.Ops()).To(HaveLen(1))
			ifBlock, ok := patch.Ops()[0].(*defkit.IfBlock)
			Expect(ok).To(BeTrue())
			Expect(ifBlock.Ops()).To(HaveLen(5))
		})

		It("should handle EndIf with no active If block gracefully", func() {
			patch := defkit.NewPatchResource()
			result := patch.EndIf() // No-op, should not panic
			Expect(result).To(BeIdenticalTo(patch))
			Expect(patch.Ops()).To(BeEmpty())
		})

		It("should allow ops before and after If block", func() {
			enabled := defkit.Bool("enabled")
			cond := defkit.Eq(enabled, defkit.Lit(true))

			patch := defkit.NewPatchResource()
			patch.
				Set("spec.replicas", defkit.Lit(1)).
				If(cond).
				Set("spec.replicas", defkit.Lit(3)).
				EndIf().
				Set("metadata.name", defkit.Lit("test"))

			Expect(patch.Ops()).To(HaveLen(3))
			_, isIfBlock := patch.Ops()[1].(*defkit.IfBlock)
			Expect(isIfBlock).To(BeTrue())
		})
	})

	Context("Passthrough", func() {
		It("should be identifiable as PassthroughOp", func() {
			patch := defkit.NewPatchResource()
			patch.Passthrough()

			Expect(patch.Ops()).To(HaveLen(1))
			_, ok := patch.Ops()[0].(*defkit.PassthroughOp)
			Expect(ok).To(BeTrue())
		})
	})

	Context("ForEachOp accessors", func() {
		It("should expose Path and Source", func() {
			labels := defkit.Object("labels")
			patch := defkit.NewPatchResource()
			patch.ForEach(labels, "metadata.labels")

			op, ok := patch.Ops()[0].(*defkit.ForEachOp)
			Expect(ok).To(BeTrue())
			Expect(op.Path()).To(Equal("metadata.labels"))
			Expect(op.Source()).To(Equal(labels))
		})
	})
})

var _ = Describe("ContextOutputRef", func() {

	Context("ContextOutput factory", func() {
		It("should create a ref with context.output path", func() {
			ref := defkit.ContextOutput()
			Expect(ref.Path()).To(Equal("context.output"))
		})
	})

	Context("Field chaining", func() {
		It("should build deep paths by chaining Field", func() {
			ref := defkit.ContextOutput().
				Field("spec").
				Field("template").
				Field("spec").
				Field("containers")
			Expect(ref.Path()).To(Equal("context.output.spec.template.spec.containers"))
		})

		It("should handle dotted field path in single call", func() {
			ref := defkit.ContextOutput().Field("spec.template.metadata.labels")
			Expect(ref.Path()).To(Equal("context.output.spec.template.metadata.labels"))
		})
	})

	Context("HasPath condition", func() {
		It("should create condition with correct paths", func() {
			cond := defkit.ContextOutput().HasPath("spec.template")
			pathCond, ok := cond.(*defkit.ContextPathExistsCondition)
			Expect(ok).To(BeTrue())
			Expect(pathCond.BasePath()).To(Equal("context.output"))
			Expect(pathCond.FieldPath()).To(Equal("spec.template"))
			Expect(pathCond.FullPath()).To(Equal("context.output.spec.template"))
		})
	})

	Context("IsSet condition", func() {
		It("should create condition for the ref itself", func() {
			cond := defkit.ContextOutput().IsSet()
			pathCond, ok := cond.(*defkit.ContextPathExistsCondition)
			Expect(ok).To(BeTrue())
			Expect(pathCond.FullPath()).To(Equal("context.output"))
		})

		It("should create condition on nested field ref", func() {
			cond := defkit.ContextOutput().Field("spec.template").IsSet()
			pathCond, ok := cond.(*defkit.ContextPathExistsCondition)
			Expect(ok).To(BeTrue())
			Expect(pathCond.FullPath()).To(Equal("context.output.spec.template"))
		})
	})

	Context("ContextPathExistsCondition.FullPath", func() {
		It("should return fieldPath when basePath is empty", func() {
			cond := defkit.ContextOutput().IsSet()
			pathCond := cond.(*defkit.ContextPathExistsCondition)
			// BasePath is empty for IsSet on the ref itself
			Expect(pathCond.BasePath()).To(BeEmpty())
			Expect(pathCond.FullPath()).To(Equal("context.output"))
		})

		It("should join basePath and fieldPath when both present", func() {
			cond := defkit.ContextOutput().HasPath("spec.containers")
			pathCond := cond.(*defkit.ContextPathExistsCondition)
			Expect(pathCond.BasePath()).To(Equal("context.output"))
			Expect(pathCond.FieldPath()).To(Equal("spec.containers"))
			Expect(pathCond.FullPath()).To(Equal("context.output.spec.containers"))
		})
	})
})
