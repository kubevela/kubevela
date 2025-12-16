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

var _ = Describe("Resource", func() {

	Describe("NewResource", func() {
		It("should create a resource with API version and kind", func() {
			r := defkit.NewResource("apps/v1", "Deployment")
			Expect(r.APIVersion()).To(Equal("apps/v1"))
			Expect(r.Kind()).To(Equal("Deployment"))
			Expect(r.Ops()).To(BeEmpty())
		})
	})

	Describe("Set", func() {
		It("should record a Set operation", func() {
			image := defkit.String("image").Required()
			r := defkit.NewResource("apps/v1", "Deployment").
				Set("spec.template.spec.containers[0].image", image)
			Expect(r.Ops()).To(HaveLen(1))
			setOp, ok := r.Ops()[0].(*defkit.SetOp)
			Expect(ok).To(BeTrue())
			Expect(setOp.Path()).To(Equal("spec.template.spec.containers[0].image"))
			Expect(setOp.Value()).To(Equal(image))
		})

		It("should record multiple Set operations", func() {
			image := defkit.String("image")
			replicas := defkit.Int("replicas")
			r := defkit.NewResource("apps/v1", "Deployment").
				Set("spec.template.spec.containers[0].image", image).
				Set("spec.replicas", replicas)
			Expect(r.Ops()).To(HaveLen(2))
		})

		It("should support literal values", func() {
			r := defkit.NewResource("v1", "Service").
				Set("spec.type", defkit.Lit("ClusterIP"))
			Expect(r.Ops()).To(HaveLen(1))
		})
	})

	Describe("SetIf", func() {
		It("should record a conditional Set operation", func() {
			enabled := defkit.Bool("enabled")
			port := defkit.Int("port")
			cond := defkit.Eq(enabled, defkit.Lit(true))
			r := defkit.NewResource("v1", "Service").
				SetIf(cond, "spec.ports[0].port", port)
			Expect(r.Ops()).To(HaveLen(1))
			setIfOp, ok := r.Ops()[0].(*defkit.SetIfOp)
			Expect(ok).To(BeTrue())
			Expect(setIfOp.Path()).To(Equal("spec.ports[0].port"))
			Expect(setIfOp.Cond()).To(Equal(cond))
		})
	})

	Describe("If/EndIf Block", func() {
		It("should group operations within If block", func() {
			enabled := defkit.Bool("enabled")
			port := defkit.Int("port")
			cond := defkit.Eq(enabled, defkit.Lit(true))
			r := defkit.NewResource("v1", "Service").
				If(cond).
				Set("spec.ports[0].port", port).
				Set("spec.ports[0].protocol", defkit.Lit("TCP")).
				EndIf()
			Expect(r.Ops()).To(HaveLen(1))
			ifBlock, ok := r.Ops()[0].(*defkit.IfBlock)
			Expect(ok).To(BeTrue())
			Expect(ifBlock.Cond()).To(Equal(cond))
			Expect(ifBlock.Ops()).To(HaveLen(2))
		})

		It("should support operations before and after If block", func() {
			name := defkit.String("name")
			enabled := defkit.Bool("enabled")
			port := defkit.Int("port")
			cond := defkit.Eq(enabled, defkit.Lit(true))
			r := defkit.NewResource("v1", "Service").
				Set("metadata.name", name).
				If(cond).
				Set("spec.ports[0].port", port).
				EndIf().
				Set("spec.type", defkit.Lit("ClusterIP"))
			Expect(r.Ops()).To(HaveLen(3))
			_, isSet1 := r.Ops()[0].(*defkit.SetOp)
			_, isIf := r.Ops()[1].(*defkit.IfBlock)
			_, isSet2 := r.Ops()[2].(*defkit.SetOp)
			Expect(isSet1).To(BeTrue())
			Expect(isIf).To(BeTrue())
			Expect(isSet2).To(BeTrue())
		})

		It("should handle SetIf within If block", func() {
			enabled := defkit.Bool("enabled")
			debug := defkit.Bool("debug")
			port := defkit.Int("port")
			outerCond := defkit.Eq(enabled, defkit.Lit(true))
			innerCond := defkit.Eq(debug, defkit.Lit(true))
			r := defkit.NewResource("v1", "Service").
				If(outerCond).
				SetIf(innerCond, "spec.ports[0].port", port).
				EndIf()
			Expect(r.Ops()).To(HaveLen(1))
			ifBlock := r.Ops()[0].(*defkit.IfBlock)
			Expect(ifBlock.Ops()).To(HaveLen(1))
			_, isSetIf := ifBlock.Ops()[0].(*defkit.SetIfOp)
			Expect(isSetIf).To(BeTrue())
		})
	})
})
