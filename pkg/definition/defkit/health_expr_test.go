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

package defkit

import (
	"strings"
	"testing"
)

func TestConditionIsTrue(t *testing.T) {
	h := Health()
	expr := h.Condition("Ready").IsTrue()

	preamble := expr.Preamble()
	if !strings.Contains(preamble, `_readyCond:`) {
		t.Errorf("Expected preamble to contain _readyCond, got: %s", preamble)
	}
	if !strings.Contains(preamble, `c.type == "Ready"`) {
		t.Errorf("Expected preamble to filter by Ready type, got: %s", preamble)
	}

	cue := expr.ToCUE()
	if !strings.Contains(cue, `status == "True"`) {
		t.Errorf("Expected ToCUE to check status == True, got: %s", cue)
	}
}

func TestConditionIsFalse(t *testing.T) {
	h := Health()
	expr := h.Condition("Stalled").IsFalse()
	cue := expr.ToCUE()
	if !strings.Contains(cue, `status == "False"`) {
		t.Errorf("Expected ToCUE to check status == False, got: %s", cue)
	}
}

func TestConditionExists(t *testing.T) {
	h := Health()
	expr := h.Condition("Initialized").Exists()
	cue := expr.ToCUE()
	if !strings.Contains(cue, `len(_initializedCond) > 0`) {
		t.Errorf("Expected ToCUE to check length > 0, got: %s", cue)
	}
	// Should NOT check status
	if strings.Contains(cue, "status") {
		t.Errorf("Exists() should not check status, got: %s", cue)
	}
}

func TestConditionReasonIs(t *testing.T) {
	h := Health()
	expr := h.Condition("Ready").ReasonIs("Available")
	cue := expr.ToCUE()
	if !strings.Contains(cue, `reason == "Available"`) {
		t.Errorf("Expected ToCUE to check reason, got: %s", cue)
	}
}

func TestAllTrue(t *testing.T) {
	h := Health()
	policy := h.Policy(h.AllTrue("Ready", "Synced"))

	// Should have preambles for both conditions
	if !strings.Contains(policy, "_readyCond:") {
		t.Errorf("Expected policy to contain _readyCond, got: %s", policy)
	}
	if !strings.Contains(policy, "_syncedCond:") {
		t.Errorf("Expected policy to contain _syncedCond, got: %s", policy)
	}

	// Should combine with AND
	if !strings.Contains(policy, "&&") {
		t.Errorf("Expected policy to use && for AllTrue, got: %s", policy)
	}
}

func TestAnyTrue(t *testing.T) {
	h := Health()
	policy := h.Policy(h.AnyTrue("Ready", "Available"))

	// Should combine with OR
	if !strings.Contains(policy, "||") {
		t.Errorf("Expected policy to use || for AnyTrue, got: %s", policy)
	}
}

func TestPhase(t *testing.T) {
	h := Health()
	expr := h.Phase("Running", "Succeeded")

	if expr.Preamble() != "" {
		t.Errorf("Phase should have no preamble, got: %s", expr.Preamble())
	}

	cue := expr.ToCUE()
	if !strings.Contains(cue, `context.output.status.phase == "Running"`) {
		t.Errorf("Expected ToCUE to check Running phase, got: %s", cue)
	}
	if !strings.Contains(cue, `context.output.status.phase == "Succeeded"`) {
		t.Errorf("Expected ToCUE to check Succeeded phase, got: %s", cue)
	}
	if !strings.Contains(cue, "||") {
		t.Errorf("Expected ToCUE to use || for multiple phases, got: %s", cue)
	}
}

func TestPhaseSingle(t *testing.T) {
	h := Health()
	expr := h.Phase("Running")
	cue := expr.ToCUE()

	// Single phase should not have ||
	if strings.Contains(cue, "||") {
		t.Errorf("Single phase should not use ||, got: %s", cue)
	}
	if cue != `context.output.status.phase == "Running"` {
		t.Errorf("Unexpected CUE for single phase: %s", cue)
	}
}

func TestPhaseField(t *testing.T) {
	h := Health()
	expr := h.PhaseField("status.currentPhase", "Active")
	cue := expr.ToCUE()
	if !strings.Contains(cue, "context.output.status.currentPhase") {
		t.Errorf("Expected custom path, got: %s", cue)
	}
}

func TestFieldEq(t *testing.T) {
	h := Health()
	expr := h.Field("status.state").Eq("active")
	cue := expr.ToCUE()
	expected := `context.output.status.state == "active"`
	if cue != expected {
		t.Errorf("Expected %s, got: %s", expected, cue)
	}
}

func TestFieldGt(t *testing.T) {
	h := Health()
	expr := h.Field("status.replicas").Gt(0)
	cue := expr.ToCUE()
	expected := `context.output.status.replicas > 0`
	if cue != expected {
		t.Errorf("Expected %s, got: %s", expected, cue)
	}
}

func TestFieldGte(t *testing.T) {
	h := Health()
	expr := h.Field("status.availableReplicas").Gte(1)
	cue := expr.ToCUE()
	expected := `context.output.status.availableReplicas >= 1`
	if cue != expected {
		t.Errorf("Expected %s, got: %s", expected, cue)
	}
}

func TestFieldLt(t *testing.T) {
	h := Health()
	expr := h.Field("status.failedReplicas").Lt(5)
	cue := expr.ToCUE()
	expected := `context.output.status.failedReplicas < 5`
	if cue != expected {
		t.Errorf("Expected %s, got: %s", expected, cue)
	}
}

func TestFieldIn(t *testing.T) {
	h := Health()
	expr := h.Field("status.phase").In("Running", "Succeeded", "Complete")
	cue := expr.ToCUE()

	if !strings.Contains(cue, `== "Running"`) {
		t.Errorf("Expected Running in In(), got: %s", cue)
	}
	if !strings.Contains(cue, `== "Succeeded"`) {
		t.Errorf("Expected Succeeded in In(), got: %s", cue)
	}
	if !strings.Contains(cue, "||") {
		t.Errorf("Expected || in In(), got: %s", cue)
	}
}

func TestFieldRef(t *testing.T) {
	h := Health()
	expr := h.Field("status.readyReplicas").Eq(h.FieldRef("spec.replicas"))
	cue := expr.ToCUE()
	expected := `context.output.status.readyReplicas == context.output.spec.replicas`
	if cue != expected {
		t.Errorf("Expected %s, got: %s", expected, cue)
	}
}

func TestExists(t *testing.T) {
	h := Health()
	expr := h.Exists("status.loadBalancer.ingress")
	cue := expr.ToCUE()
	expected := `context.output.status.loadBalancer.ingress != _|_`
	if cue != expected {
		t.Errorf("Expected %s, got: %s", expected, cue)
	}
}

func TestNotExists(t *testing.T) {
	h := Health()
	expr := h.NotExists("status.error")
	cue := expr.ToCUE()
	expected := `context.output.status.error == _|_`
	if cue != expected {
		t.Errorf("Expected %s, got: %s", expected, cue)
	}
}

func TestAnd(t *testing.T) {
	h := Health()
	expr := h.And(
		h.Field("status.replicas").Gt(0),
		h.Exists("status.endpoint"),
	)
	cue := expr.ToCUE()

	if !strings.Contains(cue, "&&") {
		t.Errorf("Expected && in And(), got: %s", cue)
	}
	if !strings.Contains(cue, "status.replicas > 0") {
		t.Errorf("Expected first expression in And(), got: %s", cue)
	}
	if !strings.Contains(cue, "status.endpoint != _|_") {
		t.Errorf("Expected second expression in And(), got: %s", cue)
	}
}

func TestOr(t *testing.T) {
	h := Health()
	expr := h.Or(
		h.Phase("Running"),
		h.Phase("Succeeded"),
	)
	cue := expr.ToCUE()

	if !strings.Contains(cue, "||") {
		t.Errorf("Expected || in Or(), got: %s", cue)
	}
}

func TestNot(t *testing.T) {
	h := Health()
	expr := h.Not(h.Condition("Stalled").IsTrue())
	cue := expr.ToCUE()

	if !strings.HasPrefix(cue, "!(") {
		t.Errorf("Expected Not() to wrap with !(), got: %s", cue)
	}
}

func TestAlways(t *testing.T) {
	h := Health()
	expr := h.Always()

	if expr.Preamble() != "" {
		t.Errorf("Always should have no preamble, got: %s", expr.Preamble())
	}
	if expr.ToCUE() != "true" {
		t.Errorf("Always should return true, got: %s", expr.ToCUE())
	}
}

func TestHealthPolicy(t *testing.T) {
	h := Health()
	policy := h.Policy(h.Condition("Ready").IsTrue())

	if !strings.Contains(policy, "isHealth:") {
		t.Errorf("Expected isHealth: in policy, got: %s", policy)
	}
	if !strings.Contains(policy, "_readyCond:") {
		t.Errorf("Expected preamble in policy, got: %s", policy)
	}
}

func TestHealthPolicyNoPreamble(t *testing.T) {
	h := Health()
	policy := h.Policy(h.Always())
	expected := "isHealth: true"
	if policy != expected {
		t.Errorf("Expected %s, got: %s", expected, policy)
	}
}

func TestComplexComposition(t *testing.T) {
	h := Health()
	// Real-world example: Crossplane-style + field check
	expr := h.And(
		h.Condition("Ready").IsTrue(),
		h.Not(h.Condition("Stalled").IsTrue()),
		h.Or(
			h.Field("status.replicas").Gte(1),
			h.Exists("status.endpoint"),
		),
	)

	policy := h.Policy(expr)

	// Should have all preambles
	if !strings.Contains(policy, "_readyCond:") {
		t.Errorf("Missing _readyCond preamble")
	}
	if !strings.Contains(policy, "_stalledCond:") {
		t.Errorf("Missing _stalledCond preamble")
	}

	// Should have complex expression
	if !strings.Contains(policy, "isHealth:") {
		t.Errorf("Missing isHealth:")
	}
	if !strings.Contains(policy, "&&") {
		t.Errorf("Missing && combinator")
	}
	if !strings.Contains(policy, "||") {
		t.Errorf("Missing || combinator")
	}
	if !strings.Contains(policy, "!(") {
		t.Errorf("Missing Not() expression")
	}
}

func TestFieldContains(t *testing.T) {
	h := Health()
	expr := h.Field("status.message").Contains("ready")
	cue := expr.ToCUE()
	expected := `strings.Contains(context.output.status.message, "ready")`
	if cue != expected {
		t.Errorf("Expected %s, got: %s", expected, cue)
	}
}

func TestComponentHealthPolicyExpr(t *testing.T) {
	h := Health()
	// Test that HealthPolicyExpr correctly integrates with ComponentDefinition
	comp := NewComponent("test-component").
		Description("A test component").
		HealthPolicyExpr(h.Condition("Ready").IsTrue())

	policy := comp.GetHealthPolicy()

	// Should contain the preamble and isHealth expression
	if !strings.Contains(policy, "_readyCond:") {
		t.Errorf("Expected policy to contain _readyCond preamble, got: %s", policy)
	}
	if !strings.Contains(policy, "isHealth:") {
		t.Errorf("Expected policy to contain isHealth:, got: %s", policy)
	}
	if !strings.Contains(policy, `status == "True"`) {
		t.Errorf("Expected policy to check status == True, got: %s", policy)
	}
}

func TestComponentHealthPolicyExprComplex(t *testing.T) {
	h := Health()
	// Test complex health expression with ComponentDefinition
	comp := NewComponent("crossplane-resource").
		HealthPolicyExpr(h.And(
			h.Condition("Ready").IsTrue(),
			h.Not(h.Condition("Stalled").IsTrue()),
		))

	policy := comp.GetHealthPolicy()

	// Should have both condition preambles
	if !strings.Contains(policy, "_readyCond:") {
		t.Errorf("Expected _readyCond preamble, got: %s", policy)
	}
	if !strings.Contains(policy, "_stalledCond:") {
		t.Errorf("Expected _stalledCond preamble, got: %s", policy)
	}
	// Should have AND and NOT
	if !strings.Contains(policy, "&&") {
		t.Errorf("Expected && in policy, got: %s", policy)
	}
	if !strings.Contains(policy, "!(") {
		t.Errorf("Expected !( in policy, got: %s", policy)
	}
}
