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

	"github.com/oam-dev/kubevela/pkg/cue/definition/health"
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

// TestConditionExprHandlesMissingStatus reproduces the fixtures from the
// GitHub issue: a health policy built with AllTrue must evaluate to false
// (not a CUE evaluation error) when status or status.conditions is absent,
// and must skip condition entries that have no type instead of erroring.
func TestConditionExprHandlesMissingStatus(t *testing.T) {
	h := Health()
	policy := h.Policy(h.And(
		h.Exists("status"),
		h.Exists("status.conditions"),
		h.AllTrue("Ready", "Synced"),
	))

	cases := []struct {
		name   string
		output map[string]interface{}
		want   bool
	}{
		{
			name:   "no status",
			output: map[string]interface{}{},
			want:   false,
		},
		{
			name:   "empty status",
			output: map[string]interface{}{"status": map[string]interface{}{}},
			want:   false,
		},
		{
			name: "ready and synced",
			output: map[string]interface{}{"status": map[string]interface{}{"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
				map[string]interface{}{"type": "Synced", "status": "True"},
			}}},
			want: true,
		},
		{
			name: "ready false",
			output: map[string]interface{}{"status": map[string]interface{}{"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "False"},
				map[string]interface{}{"type": "Synced", "status": "True"},
			}}},
			want: false,
		},
		{
			name: "condition entry without type",
			output: map[string]interface{}{"status": map[string]interface{}{"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
				map[string]interface{}{"type": "Synced", "status": "True"},
				map[string]interface{}{"status": "True"},
			}}},
			want: true,
		},
		{
			name: "matched condition without status",
			output: map[string]interface{}{"status": map[string]interface{}{"conditions": []interface{}{
				map[string]interface{}{"type": "Ready"},
				map[string]interface{}{"type": "Synced", "status": "True"},
			}}},
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			templateContext := map[string]interface{}{"output": c.output}
			got, err := health.CheckHealth(templateContext, policy, nil)
			if err != nil {
				t.Fatalf("CheckHealth returned an error instead of a health verdict: %v", err)
			}
			if got != c.want {
				t.Errorf("expected healthy=%v, got %v", c.want, got)
			}
		})
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

func TestHealthScopedCondition(t *testing.T) {
	h := Health()
	vela := VelaCtx()
	expr := h.At(vela.Outputs("securityGroup")).Condition("Ready").IsTrue()

	preamble := expr.Preamble()
	if !strings.Contains(preamble, "context.outputs.securityGroup.status.conditions") {
		t.Errorf("expected scoped root in preamble, got: %s", preamble)
	}
	if !strings.Contains(preamble, "_securityGroupReadyCond") {
		t.Errorf("expected scoped var name, got: %s", preamble)
	}
}

func TestHealthScopedField(t *testing.T) {
	h := Health()
	vela := VelaCtx()
	cue := h.At(vela.Outputs("filesystem")).Field("status.state").Eq("available").ToCUE()

	expected := `context.outputs.filesystem.status.state == "available"`
	if cue != expected {
		t.Errorf("got %q, want %q", cue, expected)
	}
}

func TestHealthDefaultRootUnchanged(t *testing.T) {
	h := Health()
	cue := h.Field("status.phase").Eq("Running").ToCUE()

	expected := `context.output.status.phase == "Running"`
	if cue != expected {
		t.Errorf("got %q, want %q", cue, expected)
	}
}

func TestHealthScopedConditionNoCollision(t *testing.T) {
	h := Health()
	vela := VelaCtx()
	a := h.Condition("Ready").IsTrue()
	b := h.At(vela.Outputs("securityGroup")).Condition("Ready").IsTrue()

	if a.Preamble() == b.Preamble() {
		t.Error("scoped and default Ready conditions produced identical preambles")
	}
}

func TestHealthAtPrimaryOutput(t *testing.T) {
	h := Health()
	vela := VelaCtx()
	cue := h.At(vela.Output()).Field("status.phase").Eq("Running").ToCUE()

	expected := `context.output.status.phase == "Running"`
	if cue != expected {
		t.Errorf("got %q, want %q", cue, expected)
	}
}

func TestHealthAtNilRef(t *testing.T) {
	h := Health()
	cue := h.At(nil).Field("status.phase").Eq("Running").ToCUE()

	expected := `context.output.status.phase == "Running"`
	if cue != expected {
		t.Errorf("got %q, want %q", cue, expected)
	}
}

func TestEveryEmptyIsUnhealthy(t *testing.T) {
	h := Health()
	expr := h.Every(OutputsWithPrefix("accessPoint"), func(item *HealthScope) HealthExpression {
		return item.Condition("Ready").IsTrue()
	})
	cue := expr.ToCUE()
	if !strings.Contains(cue, "> 0 &&") {
		t.Errorf("expected empty-match guard (len(...) > 0) in generated CUE, got: %s", cue)
	}
	if !strings.HasPrefix(cue, "len(_accessPointItems) > 0") {
		t.Errorf("expected the guard to lead the expression, got: %s", cue)
	}
}

func TestEveryItemExprHasNoPreamble(t *testing.T) {
	scope := &HealthScope{root: "_x", inline: true}
	if p := scope.Condition("Ready").IsTrue().Preamble(); p != "" {
		t.Errorf("inline scope must not emit a preamble, got %q", p)
	}
}

func TestEveryInlineConditionMergesFilters(t *testing.T) {
	scope := &HealthScope{root: "_x", inline: true}
	cue := scope.Condition("Ready").IsTrue().ToCUE()
	expected := `len([ for c in *_x.status.conditions | [] if c.type != _|_ if c.type == "Ready" if c.status != _|_ if c.status == "True" { c } ]) > 0`
	if cue != expected {
		t.Errorf("got %q, want %q", cue, expected)
	}
}

func TestEveryDistinctPatternsDoNotCollide(t *testing.T) {
	h := Health()
	a := h.Every(OutputsWithPrefix("accessPoint"), func(item *HealthScope) HealthExpression {
		return item.Condition("Ready").IsTrue()
	})
	b := h.Every(OutputsWithPrefix("mountTarget"), func(item *HealthScope) HealthExpression {
		return item.Condition("Ready").IsTrue()
	})
	if a.Preamble() == b.Preamble() {
		t.Errorf("distinct patterns produced identical preambles: %s", a.Preamble())
	}
	if a.ToCUE() == b.ToCUE() {
		t.Errorf("distinct patterns produced identical expressions: %s", a.ToCUE())
	}
}

func TestEveryGolden(t *testing.T) {
	h := Health()
	expr := h.Every(OutputsWithPrefix("accessPoint"), func(item *HealthScope) HealthExpression {
		return item.Condition("Ready").IsTrue()
	})
	policy := HealthPolicy(expr)
	expected := "_accessPointItems: [ for k, v in context.outputs if k =~ \"^accessPoint\" { v } ]\n" +
		"isHealth: len(_accessPointItems) > 0 && len([ for _accessPointItem in _accessPointItems " +
		"if len([ for c in *_accessPointItem.status.conditions | [] if c.type != _|_ if c.type == \"Ready\" " +
		"if c.status != _|_ if c.status == \"True\" { c } ]) > 0 { _accessPointItem } ]) == len(_accessPointItems)"
	if policy != expected {
		t.Errorf("golden mismatch:\n got: %s\nwant: %s", policy, expected)
	}
}

func TestEveryPrefixPreamble(t *testing.T) {
	h := Health()
	expr := h.Every(OutputsWithPrefix("accessPoint"), func(item *HealthScope) HealthExpression {
		return item.Exists("status.id")
	})
	preamble := expr.Preamble()
	expected := `_accessPointItems: [ for k, v in context.outputs if k =~ "^accessPoint" { v } ]`
	if preamble != expected {
		t.Errorf("got %q, want %q", preamble, expected)
	}
}

func TestEveryAllowEmptyIsHealthy(t *testing.T) {
	h := Health()
	expr := h.Every(OutputsWithPrefix("accessPoint"), func(item *HealthScope) HealthExpression {
		return item.Condition("Ready").IsTrue()
	}).AllowEmpty()
	cue := expr.ToCUE()
	if strings.Contains(cue, "> 0 &&") {
		t.Errorf("AllowEmpty must drop the empty-match guard, got: %s", cue)
	}
	if !strings.HasPrefix(cue, "len([ for _accessPointItem") {
		t.Errorf("AllowEmpty should lead with the all-match comprehension, got: %s", cue)
	}
}

// s3filesPolicy builds the composed health policy that mirrors the real
// s3-files component (issue #7290): primary Ready, a scoped-condition, a
// scoped field-existence, an AllowEmpty Every over access points, and a
// default Every over mount targets.
func s3filesPolicy() string {
	h := Health()
	vela := VelaCtx()
	expr := h.And(
		h.Condition("Ready").IsTrue(),
		h.At(vela.Outputs("securityGroup")).Condition("Ready").IsTrue(),
		h.At(vela.Outputs("filesystem")).Exists("status.fileSystemID"),
		h.Every(OutputsWithPrefix("accessPoint"), func(item *HealthScope) HealthExpression {
			return item.Exists("status.id")
		}).AllowEmpty(),
		h.Every(OutputsWithPrefix("mountTarget"), func(item *HealthScope) HealthExpression {
			return item.Condition("Ready").IsTrue()
		}),
	)
	return HealthPolicy(expr)
}

func readyObj() map[string]interface{} {
	return map[string]interface{}{"status": map[string]interface{}{"conditions": []interface{}{
		map[string]interface{}{"type": "Ready", "status": "True"},
	}}}
}

func notReadyObj() map[string]interface{} {
	return map[string]interface{}{"status": map[string]interface{}{"conditions": []interface{}{
		map[string]interface{}{"type": "Ready", "status": "False"},
	}}}
}

// TestS3FilesPolicyEvaluates runs the composed policy through vela-core's real
// evaluator (health.CheckHealth) across the observed cluster states. This
// proves the generated CUE compiles AND evaluates identically to the
// hand-written policy it replaces.
func TestS3FilesPolicyEvaluates(t *testing.T) {
	policy := s3filesPolicy()
	cases := []struct {
		name    string
		output  map[string]interface{}
		outputs map[string]interface{}
		want    bool
	}{
		{
			name:   "all ready",
			output: readyObj(),
			outputs: map[string]interface{}{
				"securityGroup": readyObj(),
				"filesystem":    map[string]interface{}{"status": map[string]interface{}{"fileSystemID": "fs-1"}},
				"accessPoint1":  map[string]interface{}{"status": map[string]interface{}{"id": "ap-1"}},
				"mountTarget1":  readyObj(),
			},
			want: true,
		},
		{
			name:   "mount target not ready",
			output: readyObj(),
			outputs: map[string]interface{}{
				"securityGroup": readyObj(),
				"filesystem":    map[string]interface{}{"status": map[string]interface{}{"fileSystemID": "fs-1"}},
				"mountTarget1":  readyObj(),
				"mountTarget2":  notReadyObj(),
			},
			want: false,
		},
		{
			name:   "filesystem missing fileSystemID",
			output: readyObj(),
			outputs: map[string]interface{}{
				"securityGroup": readyObj(),
				"filesystem":    readyObj(), // Ready, but no status.fileSystemID
				"mountTarget1":  readyObj(),
			},
			want: false,
		},
		{
			name:   "access point missing id",
			output: readyObj(),
			outputs: map[string]interface{}{
				"securityGroup": readyObj(),
				"filesystem":    map[string]interface{}{"status": map[string]interface{}{"fileSystemID": "fs-1"}},
				"accessPoint1":  map[string]interface{}{}, // exists, no status.id
				"mountTarget1":  readyObj(),
			},
			want: false,
		},
		{
			name:   "no access points requested (AllowEmpty)",
			output: readyObj(),
			outputs: map[string]interface{}{
				"securityGroup": readyObj(),
				"filesystem":    map[string]interface{}{"status": map[string]interface{}{"fileSystemID": "fs-1"}},
				"mountTarget1":  readyObj(),
			},
			want: true,
		},
		{
			name:   "bucket has no status",
			output: map[string]interface{}{},
			outputs: map[string]interface{}{
				"securityGroup": readyObj(),
				"filesystem":    map[string]interface{}{"status": map[string]interface{}{"fileSystemID": "fs-1"}},
				"mountTarget1":  readyObj(),
			},
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			templateContext := map[string]interface{}{"output": c.output, "outputs": c.outputs}
			got, err := health.CheckHealth(templateContext, policy, nil)
			if err != nil {
				t.Fatalf("CheckHealth errored instead of returning a verdict: %v\npolicy:\n%s", err, policy)
			}
			if got != c.want {
				t.Errorf("healthy=%v, want %v\npolicy:\n%s", got, c.want, policy)
			}
		})
	}
}

// --- Branch coverage: ConditionExpr variants, inline branches, scoped leaves, QuoteMeta ---

func TestConditionVariantsToCUE(t *testing.T) {
	h := Health()
	cases := []struct {
		name string
		expr HealthExpression
		want string
	}{
		{"IsFalse", h.Condition("Ready").IsFalse(),
			`len([ for c in _readyCond if c.status != _|_ if c.status == "False" { c } ]) > 0`},
		{"Is custom", h.Condition("Ready").Is("Degraded"),
			`len([ for c in _readyCond if c.status != _|_ if c.status == "Degraded" { c } ]) > 0`},
		{"ReasonIs", h.Condition("Ready").ReasonIs("LowDisk"),
			`len([ for c in _readyCond if c.status != _|_ if c.status == "True" if c.reason != _|_ if c.reason == "LowDisk" { c } ]) > 0`},
		{"Exists", h.Condition("Ready").Exists(),
			`len(_readyCond) > 0`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.expr.ToCUE(); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestInlineConditionBranches(t *testing.T) {
	scope := &HealthScope{root: "_x", inline: true}

	reason := scope.Condition("Ready").ReasonIs("LowDisk").ToCUE()
	wantReason := `len([ for c in *_x.status.conditions | [] if c.type != _|_ if c.type == "Ready" if c.status != _|_ if c.status == "True" if c.reason != _|_ if c.reason == "LowDisk" { c } ]) > 0`
	if reason != wantReason {
		t.Errorf("inline ReasonIs:\n got %q\nwant %q", reason, wantReason)
	}

	exists := scope.Condition("Ready").Exists().ToCUE()
	wantExists := `len([ for c in *_x.status.conditions | [] if c.type != _|_ if c.type == "Ready" { c } ]) > 0`
	if exists != wantExists {
		t.Errorf("inline Exists:\n got %q\nwant %q", exists, wantExists)
	}
}

func TestScopedLeavesToCUE(t *testing.T) {
	h := Health()
	vela := VelaCtx()
	x := h.At(vela.Outputs("x"))
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"Exists", x.Exists("status.id").ToCUE(), `context.outputs.x.status.id != _|_`},
		{"NotExists", x.NotExists("status.err").ToCUE(), `context.outputs.x.status.err == _|_`},
		{"Phase", x.Phase("Running").ToCUE(), `context.outputs.x.status.phase == "Running"`},
		{"PhaseField", x.PhaseField("status.p", "A", "B").ToCUE(),
			`context.outputs.x.status.p == "A" || context.outputs.x.status.p == "B"`},
		{"CrossResourceFieldRef",
			h.At(vela.Outputs("a")).Field("status.x").Eq(h.At(vela.Outputs("b")).FieldRef("status.y")).ToCUE(),
			`context.outputs.a.status.x == context.outputs.b.status.y`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("got %q, want %q", c.got, c.want)
			}
		})
	}
}

func TestConditionReasonEvaluates(t *testing.T) {
	h := Health()
	policy := HealthPolicy(h.Condition("Ready").IsTrue().(*ConditionExpr).ReasonIs("AllGood"))
	cond := func(reason string) map[string]interface{} {
		return map[string]interface{}{"status": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{"type": "Ready", "status": "True", "reason": reason},
		}}}
	}
	for _, c := range []struct {
		reason string
		want   bool
	}{{"AllGood", true}, {"SomethingElse", false}} {
		got, err := health.CheckHealth(map[string]interface{}{"output": cond(c.reason)}, policy, nil)
		if err != nil {
			t.Fatalf("CheckHealth error: %v", err)
		}
		if got != c.want {
			t.Errorf("reason=%q: got %v, want %v", c.reason, got, c.want)
		}
	}
}

// TestEveryPrefixQuoteMetaEvaluates proves QuoteMeta makes the prefix a LITERAL
// match: the decoy output "axb1" would match an unescaped "^a.b" regex (dot as
// wildcard) but must NOT match the escaped "^a\.b". With QuoteMeta the match set
// is empty, so AllowEmpty => healthy; without it, "axb1" (no status.id) => false.
func TestEveryPrefixQuoteMetaEvaluates(t *testing.T) {
	h := Health()
	policy := HealthPolicy(
		h.Every(OutputsWithPrefix("a.b"), func(item *HealthScope) HealthExpression {
			return item.Exists("status.id")
		}).AllowEmpty(),
	)
	ctx := map[string]interface{}{"outputs": map[string]interface{}{
		"axb1": map[string]interface{}{"status": map[string]interface{}{}}, // no status.id
	}}
	got, err := health.CheckHealth(ctx, policy, nil)
	if err != nil {
		t.Fatalf("CheckHealth error: %v\npolicy:\n%s", err, policy)
	}
	if !got {
		t.Errorf("expected healthy=true (decoy axb1 must not match literal ^a\\.b), got false\npolicy:\n%s", policy)
	}
}

func TestNewHealthScope(t *testing.T) {
	s := NewHealthScope("context.outputs.securityGroup")
	cue := s.Field("status.state").Eq("active").ToCUE()
	expected := `context.outputs.securityGroup.status.state == "active"`
	if cue != expected {
		t.Errorf("got %q, want %q", cue, expected)
	}
	// empty root falls back to the primary output
	if got := NewHealthScope("").Field("status.x").Eq(1).ToCUE(); got != "context.output.status.x == 1" {
		t.Errorf("empty-root scope: got %q", got)
	}
}

func TestUpperFirst(t *testing.T) {
	if got := upperFirst(""); got != "" {
		t.Errorf(`upperFirst("") = %q, want ""`, got)
	}
	if got := upperFirst("ready"); got != "Ready" {
		t.Errorf(`upperFirst("ready") = %q, want "Ready"`, got)
	}
}
