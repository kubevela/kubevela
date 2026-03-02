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

	Context("NewResource", func() {
		It("should create a resource with API version and kind", func() {
			r := defkit.NewResource("apps/v1", "Deployment")
			Expect(r.APIVersion()).To(Equal("apps/v1"))
			Expect(r.Kind()).To(Equal("Deployment"))
			Expect(r.Ops()).To(BeEmpty())
		})
	})

	Context("Set", func() {
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

	Context("SetIf", func() {
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

	Context("If/EndIf Block", func() {
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

	Context("NewResourceWithConditionalVersion", func() {
		It("should create a resource with conditional version", func() {
			r := defkit.NewResourceWithConditionalVersion("CronJob")
			Expect(r.Kind()).To(Equal("CronJob"))
			Expect(r.APIVersion()).To(BeEmpty())
			Expect(r.HasVersionConditionals()).To(BeFalse())
		})

		It("should add version conditionals with VersionIf", func() {
			vela := defkit.VelaCtx()
			r := defkit.NewResourceWithConditionalVersion("CronJob").
				VersionIf(defkit.Lt(vela.ClusterVersion().Minor(), defkit.Lit(25)), "batch/v1beta1").
				VersionIf(defkit.Ge(vela.ClusterVersion().Minor(), defkit.Lit(25)), "batch/v1")
			Expect(r.HasVersionConditionals()).To(BeTrue())
			Expect(r.VersionConditionals()).To(HaveLen(2))
		})

		It("should return version conditionals correctly", func() {
			vela := defkit.VelaCtx()
			cond := defkit.Lt(vela.ClusterVersion().Minor(), defkit.Lit(25))
			r := defkit.NewResourceWithConditionalVersion("CronJob").
				VersionIf(cond, "batch/v1beta1")
			conditionals := r.VersionConditionals()
			Expect(conditionals).To(HaveLen(1))
			Expect(conditionals[0].Condition).To(Equal(cond))
			Expect(conditionals[0].ApiVersion).To(Equal("batch/v1beta1"))
		})
	})

	Context("SpreadIf", func() {
		It("should record a SpreadIf operation", func() {
			labels := defkit.Object("labels")
			r := defkit.NewResource("apps/v1", "Deployment").
				SpreadIf(labels.IsSet(), "spec.template.metadata.labels", labels)
			Expect(r.Ops()).To(HaveLen(1))
			spreadIfOp, ok := r.Ops()[0].(*defkit.SpreadIfOp)
			Expect(ok).To(BeTrue())
			Expect(spreadIfOp.Path()).To(Equal("spec.template.metadata.labels"))
			Expect(spreadIfOp.Value()).To(Equal(labels))
			Expect(spreadIfOp.Cond()).NotTo(BeNil())
		})

		It("should combine SpreadIf with Set operations", func() {
			vela := defkit.VelaCtx()
			labels := defkit.Object("labels")
			r := defkit.NewResource("apps/v1", "Deployment").
				Set("spec.template.metadata.labels[app.oam.dev/name]", vela.AppName()).
				SpreadIf(labels.IsSet(), "spec.template.metadata.labels", labels)
			Expect(r.Ops()).To(HaveLen(2))
			_, isSetOp := r.Ops()[0].(*defkit.SetOp)
			_, isSpreadIf := r.Ops()[1].(*defkit.SpreadIfOp)
			Expect(isSetOp).To(BeTrue())
			Expect(isSpreadIf).To(BeTrue())
		})

		It("should record SpreadIf within If block", func() {
			enabled := defkit.Bool("enabled")
			labels := defkit.Object("labels")
			outerCond := defkit.Eq(enabled, defkit.Lit(true))
			r := defkit.NewResource("apps/v1", "Deployment").
				If(outerCond).
				SpreadIf(labels.IsSet(), "spec.template.metadata.labels", labels).
				EndIf()
			Expect(r.Ops()).To(HaveLen(1))
			ifBlock := r.Ops()[0].(*defkit.IfBlock)
			Expect(ifBlock.Ops()).To(HaveLen(1))
			_, isSpreadIf := ifBlock.Ops()[0].(*defkit.SpreadIfOp)
			Expect(isSpreadIf).To(BeTrue())
		})
	})

	Context("Directive", func() {
		It("should record a Directive operation", func() {
			r := defkit.NewResource("apps/v1", "DaemonSet").
				Directive("spec.template.spec.hostAliases", "patchKey=ip")
			Expect(r.Ops()).To(HaveLen(1))
			dirOp, ok := r.Ops()[0].(*defkit.DirectiveOp)
			Expect(ok).To(BeTrue())
			Expect(dirOp.Path()).To(Equal("spec.template.spec.hostAliases"))
			Expect(dirOp.GetDirective()).To(Equal("patchKey=ip"))
		})

		It("should record Directive within If block", func() {
			hostAliases := defkit.Object("hostAliases")
			r := defkit.NewResource("apps/v1", "DaemonSet").
				If(hostAliases.IsSet()).
				Set("spec.template.spec.hostAliases", hostAliases).
				Directive("spec.template.spec.hostAliases", "patchKey=ip").
				EndIf()
			Expect(r.Ops()).To(HaveLen(1))
			ifBlock, ok := r.Ops()[0].(*defkit.IfBlock)
			Expect(ok).To(BeTrue())
			Expect(ifBlock.Ops()).To(HaveLen(2))
			_, isDirOp := ifBlock.Ops()[1].(*defkit.DirectiveOp)
			Expect(isDirOp).To(BeTrue())
		})

		It("should combine with Set operations", func() {
			hostAliases := defkit.Object("hostAliases")
			r := defkit.NewResource("apps/v1", "DaemonSet").
				SetIf(hostAliases.IsSet(), "spec.template.spec.hostAliases", hostAliases).
				Directive("spec.template.spec.hostAliases", "patchKey=ip")
			Expect(r.Ops()).To(HaveLen(2))
			_, isSetIf := r.Ops()[0].(*defkit.SetIfOp)
			_, isDirOp := r.Ops()[1].(*defkit.DirectiveOp)
			Expect(isSetIf).To(BeTrue())
			Expect(isDirOp).To(BeTrue())
		})
	})
})
