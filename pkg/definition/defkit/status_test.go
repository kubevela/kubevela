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

var _ = Describe("Status", func() {

	Context("StatusBuilder", func() {
		It("should create a status builder", func() {
			s := defkit.Status()
			Expect(s).NotTo(BeNil())
		})

		It("should add IntField to status", func() {
			s := defkit.Status().
				IntField("ready.replicas", "status.numberReady", 0)
			cue := s.Build()
			Expect(cue).To(ContainSubstring("ready:"))
			Expect(cue).To(ContainSubstring("replicas:"))
			Expect(cue).To(ContainSubstring("*0 | int"))
			Expect(cue).To(ContainSubstring("context.output.status.numberReady"))
		})

		It("should add StringField to status", func() {
			s := defkit.Status().
				StringField("state", "status.phase", "unknown")
			cue := s.Build()
			Expect(cue).To(ContainSubstring("state:"))
			Expect(cue).To(ContainSubstring(`*"unknown" | string`))
			Expect(cue).To(ContainSubstring("context.output.status.phase"))
		})

		It("should set message template", func() {
			s := defkit.Status().
				IntField("ready.replicas", "status.readyReplicas", 0).
				Message(`Ready:\(ready.replicas)`)
			cue := s.Build()
			Expect(cue).To(ContainSubstring(`message: "Ready:\(ready.replicas)"`))
		})

		It("should support RawCUE override", func() {
			rawCUE := `message: "Custom status"`
			s := defkit.Status().
				IntField("ready.replicas", "status.readyReplicas", 0).
				RawCUE(rawCUE)
			cue := s.Build()
			Expect(cue).To(Equal(rawCUE))
		})
	})

	Context("HealthBuilder", func() {
		It("should create a health builder", func() {
			h := defkit.Health()
			Expect(h).NotTo(BeNil())
		})

		It("should add IntField through HealthBuilder", func() {
			h := defkit.Health().
				IntField("ready.replicas", "status.readyReplicas", 0)
			cue := h.Build()
			Expect(cue).To(ContainSubstring("ready:"))
			Expect(cue).To(ContainSubstring("replicas:"))
			Expect(cue).To(ContainSubstring("*0 | int"))
		})

		It("should add StringField through HealthBuilder", func() {
			h := defkit.Health().
				StringField("state", "status.phase", "unknown")
			cue := h.Build()
			Expect(cue).To(ContainSubstring("state:"))
			Expect(cue).To(ContainSubstring(`*"unknown" | string`))
		})

		It("should add MetadataField to health builder", func() {
			h := defkit.Health().
				MetadataField("generation.metadata", "metadata.generation")
			cue := h.Build()
			Expect(cue).To(ContainSubstring("generation:"))
			Expect(cue).To(ContainSubstring("context.output.metadata.generation"))
		})

		It("should add health conditions with HealthyWhen", func() {
			h := defkit.Health().
				IntField("ready.replicas", "status.readyReplicas", 0).
				IntField("desired.replicas", "status.replicas", 0).
				HealthyWhen("ready.replicas == desired.replicas")
			cue := h.Build()
			Expect(cue).To(ContainSubstring("isHealth: ready.replicas == desired.replicas"))
		})

		It("should add multiple health conditions", func() {
			h := defkit.Health().
				HealthyWhen("condition1", "condition2", "condition3")
			cue := h.Build()
			Expect(cue).To(ContainSubstring("isHealth: condition1 && condition2 && condition3"))
		})

		It("should support RawCUE override on health builder", func() {
			rawCUE := `isHealth: true`
			h := defkit.Health().
				IntField("ready.replicas", "status.readyReplicas", 0).
				RawCUE(rawCUE)
			cue := h.Build()
			Expect(cue).To(Equal(rawCUE))
		})
	})

	Context("Status helper functions", func() {
		It("should create StatusEq expression", func() {
			result := defkit.StatusEq("left.field", "right.field")
			Expect(result).To(Equal("left.field == right.field"))
		})

		It("should create StatusGte expression", func() {
			result := defkit.StatusGte("left.field", "right.field")
			Expect(result).To(Equal("left.field >= right.field"))
		})

		It("should create StatusOr expression", func() {
			result := defkit.StatusOr("cond1", "cond2", "cond3")
			Expect(result).To(Equal("(cond1 || cond2 || cond3)"))
		})

		It("should create StatusAnd expression", func() {
			result := defkit.StatusAnd("cond1", "cond2", "cond3")
			Expect(result).To(Equal("(cond1 && cond2 && cond3)"))
		})
	})

	Context("Predefined Status Builders", func() {
		It("should create DaemonSetStatus builder", func() {
			s := defkit.DaemonSetStatus()
			cue := s.Build()
			Expect(cue).To(ContainSubstring("ready:"))
			Expect(cue).To(ContainSubstring("desired:"))
			Expect(cue).To(ContainSubstring("status.numberReady"))
			Expect(cue).To(ContainSubstring("status.desiredNumberScheduled"))
			Expect(cue).To(ContainSubstring("message:"))
		})

		It("should create DaemonSetHealth builder", func() {
			h := defkit.DaemonSetHealth()
			cue := h.Build()
			Expect(cue).To(ContainSubstring("ready:"))
			Expect(cue).To(ContainSubstring("desired:"))
			Expect(cue).To(ContainSubstring("current:"))
			Expect(cue).To(ContainSubstring("updated:"))
			Expect(cue).To(ContainSubstring("generation:"))
			Expect(cue).To(ContainSubstring("isHealth:"))
		})

		It("should create DeploymentStatus builder", func() {
			s := defkit.DeploymentStatus()
			cue := s.Build()
			Expect(cue).To(ContainSubstring("ready:"))
			Expect(cue).To(ContainSubstring("status.readyReplicas"))
			Expect(cue).To(ContainSubstring("message:"))
		})

		It("should create DeploymentHealth builder with raw CUE", func() {
			h := defkit.DeploymentHealth()
			cue := h.Build()
			Expect(cue).To(ContainSubstring("updatedReplicas:"))
			Expect(cue).To(ContainSubstring("readyReplicas:"))
			Expect(cue).To(ContainSubstring("replicas:"))
			Expect(cue).To(ContainSubstring("observedGeneration:"))
			Expect(cue).To(ContainSubstring("_isHealth:"))
			Expect(cue).To(ContainSubstring("isHealth:"))
		})

		It("should create StatefulSetStatus builder", func() {
			s := defkit.StatefulSetStatus()
			cue := s.Build()
			Expect(cue).To(ContainSubstring("ready:"))
			Expect(cue).To(ContainSubstring("desired:"))
			Expect(cue).To(ContainSubstring("status.readyReplicas"))
			Expect(cue).To(ContainSubstring("message:"))
		})

		It("should create StatefulSetHealth builder", func() {
			h := defkit.StatefulSetHealth()
			cue := h.Build()
			Expect(cue).To(ContainSubstring("ready:"))
			Expect(cue).To(ContainSubstring("updated:"))
			Expect(cue).To(ContainSubstring("desired:"))
			Expect(cue).To(ContainSubstring("generation:"))
			Expect(cue).To(ContainSubstring("isHealth:"))
		})

		It("should create JobHealth builder", func() {
			h := defkit.JobHealth()
			cue := h.Build()
			Expect(cue).To(ContainSubstring("succeeded:"))
			Expect(cue).To(ContainSubstring("failed:"))
			Expect(cue).To(ContainSubstring("status.succeeded"))
			Expect(cue).To(ContainSubstring("status.failed"))
			Expect(cue).To(ContainSubstring("isHealth: succeeded >= 1 || failed >= 1"))
		})

		It("should create CronJobHealth builder", func() {
			h := defkit.CronJobHealth()
			cue := h.Build()
			Expect(cue).To(ContainSubstring("isHealth: true"))
		})
	})

	Context("HealthBuilder Expressions", func() {
		It("should create Condition expression", func() {
			h := defkit.Health()
			cond := h.Condition("Ready")
			Expect(cond).NotTo(BeNil())
		})

		It("should create Field expression", func() {
			h := defkit.Health()
			field := h.Field("status.replicas")
			Expect(field).NotTo(BeNil())
		})

		It("should create FieldRef expression", func() {
			h := defkit.Health()
			ref := h.FieldRef("spec.replicas")
			Expect(ref).NotTo(BeNil())
		})

		It("should create Phase expression", func() {
			h := defkit.Health()
			expr := h.Phase("Running", "Succeeded")
			Expect(expr).NotTo(BeNil())
		})

		It("should create PhaseField expression", func() {
			h := defkit.Health()
			expr := h.PhaseField("status.currentPhase", "Active", "Ready")
			Expect(expr).NotTo(BeNil())
		})

		It("should create Exists expression", func() {
			h := defkit.Health()
			expr := h.Exists("status.loadBalancer.ingress")
			Expect(expr).NotTo(BeNil())
		})

		It("should create NotExists expression", func() {
			h := defkit.Health()
			expr := h.NotExists("status.error")
			Expect(expr).NotTo(BeNil())
		})

		It("should create And expression", func() {
			h := defkit.Health()
			expr1 := h.Condition("Ready").IsTrue()
			expr2 := h.Condition("Synced").IsTrue()
			and := h.And(expr1, expr2)
			Expect(and).NotTo(BeNil())
		})

		It("should create Or expression", func() {
			h := defkit.Health()
			expr1 := h.Phase("Running")
			expr2 := h.Phase("Succeeded")
			or := h.Or(expr1, expr2)
			Expect(or).NotTo(BeNil())
		})

		It("should create Not expression", func() {
			h := defkit.Health()
			expr := h.Condition("Stalled").IsTrue()
			not := h.Not(expr)
			Expect(not).NotTo(BeNil())
		})

		It("should create Always expression", func() {
			h := defkit.Health()
			expr := h.Always()
			Expect(expr).NotTo(BeNil())
		})

		It("should create AllTrue expression", func() {
			h := defkit.Health()
			expr := h.AllTrue("Ready", "Synced", "Available")
			Expect(expr).NotTo(BeNil())
		})

		It("should create AnyTrue expression", func() {
			h := defkit.Health()
			expr := h.AnyTrue("Ready", "Available")
			Expect(expr).NotTo(BeNil())
		})

		It("should set health condition with HealthyWhenExpr", func() {
			h := defkit.Health()
			expr := h.Condition("Ready").IsTrue()
			h.HealthyWhenExpr(expr)
			cue := h.Build()
			Expect(cue).NotTo(BeEmpty())
		})

		It("should generate policy from expression", func() {
			h := defkit.Health()
			expr := h.Condition("Ready").IsTrue()
			policy := h.Policy(expr)
			Expect(policy).NotTo(BeEmpty())
			Expect(policy).To(ContainSubstring("isHealth:"))
		})
	})
})
