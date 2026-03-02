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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("Status", func() {

	Context("StatusBuilder", func() {
		It("should create a status builder that produces empty CUE without fields", func() {
			s := defkit.Status()
			Expect(s).NotTo(BeNil())
			cue := s.Build()
			Expect(cue).To(BeEmpty())
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
		It("should create a health builder that produces empty CUE without conditions", func() {
			h := defkit.Health()
			Expect(h).NotTo(BeNil())
			cue := h.Build()
			Expect(cue).To(BeEmpty())
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

		It("should add multiple health conditions with auto-parenthesization", func() {
			h := defkit.Health().
				HealthyWhen("condition1", "condition2", "condition3")
			cue := h.Build()
			Expect(cue).To(ContainSubstring("isHealth: (condition1) && (condition2) && (condition3)"))
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

		It("should create DeploymentHealth builder with consolidated fields", func() {
			h := defkit.DeploymentHealth()
			cue := h.Build()
			// Verify consolidated ready block with all fields
			Expect(cue).To(ContainSubstring("ready: {"))
			Expect(cue).To(ContainSubstring("updatedReplicas:"))
			Expect(cue).To(ContainSubstring("readyReplicas:"))
			Expect(cue).To(ContainSubstring("replicas:"))
			Expect(cue).To(ContainSubstring("observedGeneration:"))
			// Verify _isHealth intermediate pattern
			Expect(cue).To(ContainSubstring("_isHealth:"))
			Expect(cue).To(ContainSubstring("isHealth: *_isHealth | bool"))
			// Verify annotation-based disable override
			Expect(cue).To(ContainSubstring("app.oam.dev/disable-health-check"))
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
			Expect(cue).To(ContainSubstring("status.succeeded"))
			Expect(cue).To(ContainSubstring("isHealth: succeeded == context.output.spec.parallelism"))
		})

		It("should create CronJobHealth builder", func() {
			h := defkit.CronJobHealth()
			cue := h.Build()
			Expect(cue).To(ContainSubstring("isHealth: true"))
		})
	})

	Context("StatusBuilder field grouping", func() {
		It("should consolidate multiple fields with same parent into one block", func() {
			s := defkit.Status().
				IntField("status.active", "status.active", 0).
				IntField("status.failed", "status.failed", 0).
				IntField("status.succeeded", "status.succeeded", 0).
				Message(`Active/Failed/Succeeded:\(status.active)/\(status.failed)/\(status.succeeded)`)
			cue := s.Build()
			Expect(strings.Count(cue, "status: {")).To(Equal(1))
			Expect(cue).To(ContainSubstring("active:"))
			Expect(cue).To(ContainSubstring("failed:"))
			Expect(cue).To(ContainSubstring("succeeded:"))
		})

		It("should keep different parent prefixes as separate blocks", func() {
			s := defkit.Status().
				IntField("ready.replicas", "status.numberReady", 0).
				IntField("desired.replicas", "status.desiredNumberScheduled", 0).
				Message(`Ready:\(ready.replicas)/\(desired.replicas)`)
			cue := s.Build()
			Expect(strings.Count(cue, "ready: {")).To(Equal(1))
			Expect(strings.Count(cue, "desired: {")).To(Equal(1))
		})

		It("should column-align fields within consolidated block", func() {
			s := defkit.Status().
				IntField("status.active", "status.active", 0).
				IntField("status.succeeded", "status.succeeded", 0)
			cue := s.Build()
			// "active" is shorter than "succeeded", so it should be padded
			Expect(cue).To(ContainSubstring("active:    *0 | int"))
			Expect(cue).To(ContainSubstring("succeeded: *0 | int"))
		})

		It("should handle simple (non-nested) fields without grouping", func() {
			s := defkit.Status().
				IntField("succeeded", "status.succeeded", 0)
			cue := s.Build()
			Expect(cue).To(ContainSubstring("succeeded: *0 | int"))
			Expect(cue).To(ContainSubstring("context.output.status.succeeded"))
		})
	})

	Context("HealthBuilder field grouping", func() {
		It("should consolidate multiple fields with same parent into one block", func() {
			h := defkit.Health().
				IntField("ready.replicas", "status.readyReplicas", 0).
				IntField("ready.updated", "status.updatedReplicas", 0).
				HealthyWhen("ready.replicas == ready.updated")
			cue := h.Build()
			// Should produce a single consolidated "ready:" block, not two separate ones
			Expect(strings.Count(cue, "ready: {")).To(Equal(1))
			Expect(cue).To(ContainSubstring("replicas:"))
			Expect(cue).To(ContainSubstring("updated:"))
		})

		It("should keep single-field groups as consolidated blocks", func() {
			h := defkit.Health().
				IntField("ready.replicas", "status.readyReplicas", 0).
				IntField("desired.replicas", "status.replicas", 0).
				HealthyWhen("ready.replicas == desired.replicas")
			cue := h.Build()
			Expect(cue).To(ContainSubstring("ready: {"))
			Expect(cue).To(ContainSubstring("desired: {"))
		})

		It("should handle simple (non-nested) fields without grouping", func() {
			h := defkit.Health().
				IntField("succeeded", "status.succeeded", 0).
				HealthyWhen("succeeded >= 1")
			cue := h.Build()
			Expect(cue).To(ContainSubstring("succeeded:"))
			Expect(cue).To(ContainSubstring("isHealth: succeeded >= 1"))
		})

		It("should place metadata fields in defaults block of consolidated group", func() {
			h := defkit.Health().
				MetadataField("generation.metadata", "metadata.generation").
				IntField("generation.observed", "status.observedGeneration", 0)
			cue := h.Build()
			// Should be one consolidated generation: block
			Expect(strings.Count(cue, "generation: {")).To(Equal(1))
			// Metadata field should appear as a direct reference (no default value)
			Expect(cue).To(ContainSubstring("metadata: context.output.metadata.generation"))
			// Int field should have default
			Expect(cue).To(ContainSubstring("observed:"))
			Expect(cue).To(ContainSubstring("*0 | int"))
		})

		It("should column-align fields in consolidated blocks", func() {
			h := defkit.Health().
				IntField("ready.a", "status.a", 0).
				IntField("ready.longFieldName", "status.longFieldName", 0)
			cue := h.Build()
			// The shorter field "a" should have more padding than "longFieldName"
			Expect(cue).To(ContainSubstring("a:"))
			Expect(cue).To(ContainSubstring("longFieldName:"))
		})
	})

	Context("HealthBuilder auto-parenthesization", func() {
		It("should NOT parenthesize a single condition", func() {
			h := defkit.Health().
				HealthyWhen("ready.replicas == desired.replicas")
			cue := h.Build()
			Expect(cue).To(ContainSubstring("isHealth: ready.replicas == desired.replicas"))
		})

		It("should auto-parenthesize multiple conditions", func() {
			h := defkit.Health().
				HealthyWhen("a == b", "c == d")
			cue := h.Build()
			Expect(cue).To(ContainSubstring("isHealth: (a == b) && (c == d)"))
		})

		It("should NOT double-wrap already-parenthesized conditions", func() {
			h := defkit.Health().
				HealthyWhen("a == b", defkit.StatusOr("x == y", "x > y"))
			cue := h.Build()
			// StatusOr returns "(x == y || x > y)" which is already fully parenthesized
			Expect(cue).To(ContainSubstring("(a == b) && (x == y || x > y)"))
		})

		It("should NOT treat partially-parenthesized strings as fully wrapped", func() {
			// "(a) && (b)" starts with ( and ends with ) but is not fully enclosed
			h := defkit.Health().
				HealthyWhen("first", "(a) && (b)")
			cue := h.Build()
			Expect(cue).To(ContainSubstring("(first) && ((a) && (b))"))
		})

		It("should work with StatusEq without manual parens", func() {
			h := defkit.Health().
				HealthyWhen(
					defkit.StatusEq("spec.replicas", "ready.replicas"),
					defkit.StatusEq("spec.replicas", "ready.updated"),
				)
			cue := h.Build()
			Expect(cue).To(ContainSubstring("(spec.replicas == ready.replicas) && (spec.replicas == ready.updated)"))
		})

		It("should work with WithDefault and StatusEq", func() {
			h := defkit.Health().
				HealthyWhen(
					defkit.StatusEq("a", "b"),
					defkit.StatusEq("c", "d"),
				).
				WithDefault()
			cue := h.Build()
			Expect(cue).To(ContainSubstring("_isHealth: (a == b) && (c == d)"))
			Expect(cue).To(ContainSubstring("isHealth: *_isHealth | bool"))
		})

		It("should produce correct CUE for DaemonSetHealth with auto-parens", func() {
			h := defkit.DaemonSetHealth()
			cue := h.Build()
			// Each equality condition should be parenthesized
			Expect(cue).To(ContainSubstring("(desired.replicas == ready.replicas)"))
			Expect(cue).To(ContainSubstring("(desired.replicas == updated.replicas)"))
			Expect(cue).To(ContainSubstring("(desired.replicas == current.replicas)"))
			// StatusOr is already wrapped, should not be double-wrapped
			Expect(cue).To(ContainSubstring("(generation.observed == generation.metadata || generation.observed > generation.metadata)"))
		})

		It("should produce correct CUE for DeploymentHealth with auto-parens", func() {
			h := defkit.DeploymentHealth()
			cue := h.Build()
			Expect(cue).To(ContainSubstring("(context.output.spec.replicas == ready.readyReplicas)"))
			Expect(cue).To(ContainSubstring("(context.output.spec.replicas == ready.updatedReplicas)"))
			Expect(cue).To(ContainSubstring("(context.output.spec.replicas == ready.replicas)"))
			Expect(cue).To(ContainSubstring("_isHealth:"))
		})
	})

	Context("HealthBuilder Expressions", func() {
		It("should generate Condition expression that checks condition status", func() {
			h := defkit.Health()
			expr := h.Condition("Ready").IsTrue()
			policy := h.Policy(expr)
			Expect(policy).To(ContainSubstring("Ready"))
			Expect(policy).To(ContainSubstring("isHealth:"))
			Expect(policy).To(ContainSubstring(`"True"`))
		})

		It("should generate Phase expression that checks status.phase", func() {
			h := defkit.Health()
			expr := h.Phase("Running", "Succeeded")
			policy := h.Policy(expr)
			Expect(policy).To(ContainSubstring("isHealth:"))
			Expect(policy).To(ContainSubstring("Running"))
			Expect(policy).To(ContainSubstring("Succeeded"))
			Expect(policy).To(ContainSubstring("context.output.status.phase"))
		})

		It("should generate PhaseField expression with custom field path", func() {
			h := defkit.Health()
			expr := h.PhaseField("status.currentPhase", "Active", "Ready")
			policy := h.Policy(expr)
			Expect(policy).To(ContainSubstring("isHealth:"))
			Expect(policy).To(ContainSubstring("context.output.status.currentPhase"))
			Expect(policy).To(ContainSubstring("Active"))
			Expect(policy).To(ContainSubstring("Ready"))
		})

		It("should generate Exists expression that checks field != _|_", func() {
			h := defkit.Health()
			expr := h.Exists("status.loadBalancer.ingress")
			policy := h.Policy(expr)
			Expect(policy).To(ContainSubstring("isHealth:"))
			Expect(policy).To(ContainSubstring("status.loadBalancer.ingress"))
			Expect(policy).To(ContainSubstring("!= _|_"))
		})

		It("should generate NotExists expression that checks field == _|_", func() {
			h := defkit.Health()
			expr := h.NotExists("status.error")
			policy := h.Policy(expr)
			Expect(policy).To(ContainSubstring("isHealth:"))
			Expect(policy).To(ContainSubstring("status.error"))
			Expect(policy).To(ContainSubstring("== _|_"))
		})

		It("should generate And expression combining multiple conditions", func() {
			h := defkit.Health()
			expr1 := h.Condition("Ready").IsTrue()
			expr2 := h.Condition("Synced").IsTrue()
			and := h.And(expr1, expr2)
			policy := h.Policy(and)
			Expect(policy).To(ContainSubstring("Ready"))
			Expect(policy).To(ContainSubstring("Synced"))
			Expect(policy).To(ContainSubstring("&&"))
		})

		It("should generate Or expression combining multiple conditions", func() {
			h := defkit.Health()
			expr1 := h.Phase("Running")
			expr2 := h.Phase("Succeeded")
			or := h.Or(expr1, expr2)
			policy := h.Policy(or)
			Expect(policy).To(ContainSubstring("Running"))
			Expect(policy).To(ContainSubstring("Succeeded"))
			Expect(policy).To(ContainSubstring("||"))
		})

		It("should generate Not expression negating a condition", func() {
			h := defkit.Health()
			expr := h.Condition("Stalled").IsTrue()
			not := h.Not(expr)
			policy := h.Policy(not)
			Expect(policy).To(ContainSubstring("Stalled"))
			Expect(policy).To(ContainSubstring("!"))
		})

		It("should generate Always expression as isHealth: true", func() {
			h := defkit.Health()
			expr := h.Always()
			policy := h.Policy(expr)
			Expect(policy).To(ContainSubstring("isHealth: true"))
		})

		It("should generate AllTrue expression checking multiple conditions are True", func() {
			h := defkit.Health()
			expr := h.AllTrue("Ready", "Synced", "Available")
			policy := h.Policy(expr)
			Expect(policy).To(ContainSubstring("Ready"))
			Expect(policy).To(ContainSubstring("Synced"))
			Expect(policy).To(ContainSubstring("Available"))
			Expect(policy).To(ContainSubstring("&&"))
		})

		It("should generate AnyTrue expression checking any condition is True", func() {
			h := defkit.Health()
			expr := h.AnyTrue("Ready", "Available")
			policy := h.Policy(expr)
			Expect(policy).To(ContainSubstring("Ready"))
			Expect(policy).To(ContainSubstring("Available"))
			Expect(policy).To(ContainSubstring("||"))
		})

		It("should set health condition with HealthyWhenExpr and generate correct CUE", func() {
			h := defkit.Health()
			expr := h.Condition("Ready").IsTrue()
			h.HealthyWhenExpr(expr)
			cue := h.Build()
			Expect(cue).To(ContainSubstring("isHealth:"))
			Expect(cue).To(ContainSubstring("Ready"))
			Expect(cue).To(ContainSubstring(`"True"`))
		})

		It("should generate policy with correct isHealth expression from Condition", func() {
			h := defkit.Health()
			expr := h.Condition("Ready").IsTrue()
			policy := h.Policy(expr)
			Expect(policy).To(ContainSubstring("isHealth:"))
			Expect(policy).To(ContainSubstring("Ready"))
			Expect(policy).To(ContainSubstring(`"True"`))
		})
	})
})
