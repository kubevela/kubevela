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

func TestStatusFieldDefault(t *testing.T) {
	s := Status()
	field := s.Field("status.readyReplicas").Default(0)

	preamble := field.Preamble()
	if !strings.Contains(preamble, "_readyReplicas:") {
		t.Errorf("Expected preamble to contain _readyReplicas, got: %s", preamble)
	}
	if !strings.Contains(preamble, "*0 | int") {
		t.Errorf("Expected preamble to have default *0 | int, got: %s", preamble)
	}
	if !strings.Contains(preamble, "context.output.status.readyReplicas") {
		t.Errorf("Expected preamble to reference context.output.status.readyReplicas, got: %s", preamble)
	}
}

func TestStatusFieldNoDefault(t *testing.T) {
	s := Status()
	field := s.Field("status.phase")

	// No default means no preamble
	preamble := field.Preamble()
	if preamble != "" {
		t.Errorf("Expected empty preamble for field without default, got: %s", preamble)
	}

	cue := field.ToCUE()
	if !strings.Contains(cue, "context.output.status.phase") {
		t.Errorf("Expected ToCUE to reference field directly, got: %s", cue)
	}
}

func TestStatusSpecField(t *testing.T) {
	s := Status()
	field := s.SpecField("spec.replicas")

	cue := field.ToCUE()
	if !strings.Contains(cue, "context.output.spec.replicas") {
		t.Errorf("Expected ToCUE to reference spec.replicas, got: %s", cue)
	}
}

func TestStatusConditionMessage(t *testing.T) {
	s := Status()
	condExpr := s.Condition("Ready").Message()

	preamble := condExpr.Preamble()
	if !strings.Contains(preamble, "_readyCond:") {
		t.Errorf("Expected preamble to contain _readyCond, got: %s", preamble)
	}
	if !strings.Contains(preamble, `cond.type == "Ready"`) {
		t.Errorf("Expected preamble to filter by Ready type, got: %s", preamble)
	}
	if !strings.Contains(preamble, "_readyMessage:") {
		t.Errorf("Expected preamble to define _readyMessage, got: %s", preamble)
	}

	cue := condExpr.ToCUE()
	if !strings.Contains(cue, "_readyMessage") {
		t.Errorf("Expected ToCUE to reference _readyMessage, got: %s", cue)
	}
}

func TestStatusConditionStatus(t *testing.T) {
	s := Status()
	condExpr := s.Condition("Ready").StatusValue()

	preamble := condExpr.Preamble()
	if !strings.Contains(preamble, "_readyStatus:") {
		t.Errorf("Expected preamble to define _readyStatus, got: %s", preamble)
	}

	cue := condExpr.ToCUE()
	if !strings.Contains(cue, "_readyStatus") {
		t.Errorf("Expected ToCUE to reference _readyStatus, got: %s", cue)
	}
}

func TestStatusConditionReason(t *testing.T) {
	s := Status()
	condExpr := s.Condition("Ready").Reason()

	preamble := condExpr.Preamble()
	if !strings.Contains(preamble, "_readyReason:") {
		t.Errorf("Expected preamble to define _readyReason, got: %s", preamble)
	}
}

func TestStatusLiteral(t *testing.T) {
	s := Status()
	lit := s.Literal("Service is running")

	if lit.Preamble() != "" {
		t.Errorf("Literal should have empty preamble, got: %s", lit.Preamble())
	}

	cue := lit.ToCUE()
	if cue != `"Service is running"` {
		t.Errorf("Expected literal string, got: %s", cue)
	}
}

func TestStatusConcat(t *testing.T) {
	s := Status()
	expr := s.Concat("Ready: ", s.Field("status.ready").Default(0), "/", s.SpecField("spec.replicas"))

	preamble := expr.Preamble()
	if !strings.Contains(preamble, "_ready:") {
		t.Errorf("Expected preamble to contain field definition, got: %s", preamble)
	}

	cue := expr.ToCUE()
	if !strings.Contains(cue, "Ready:") {
		t.Errorf("Expected concat to include literal 'Ready:', got: %s", cue)
	}
}

func TestStatusFormat(t *testing.T) {
	s := Status()
	ready := s.Field("status.ready").Default(0)
	total := s.SpecField("spec.replicas")
	expr := s.Format("Ready: %v/%v", ready, total)

	preamble := expr.Preamble()
	if !strings.Contains(preamble, "_ready:") {
		t.Errorf("Expected preamble to contain field definition, got: %s", preamble)
	}

	cue := expr.ToCUE()
	if !strings.Contains(cue, "Ready:") {
		t.Errorf("Expected format to include 'Ready:', got: %s", cue)
	}
}

func TestStatusExists(t *testing.T) {
	s := Status()
	expr := s.Exists("status.endpoint")

	cond := expr.ToCUECondition()
	expected := `context.output.status.endpoint != _|_`
	if cond != expected {
		t.Errorf("Expected %s, got: %s", expected, cond)
	}
}

func TestStatusNotExists(t *testing.T) {
	s := Status()
	expr := s.NotExists("status.error")

	cond := expr.ToCUECondition()
	expected := `context.output.status.error == _|_`
	if cond != expected {
		t.Errorf("Expected %s, got: %s", expected, cond)
	}
}

func TestStatusFieldConditionEq(t *testing.T) {
	s := Status()
	field := s.Field("status.phase")
	cond := field.Eq("Running")

	condCUE := cond.ToCUECondition()
	expected := `context.output.status.phase == "Running"`
	if condCUE != expected {
		t.Errorf("Expected %s, got: %s", expected, condCUE)
	}
}

func TestStatusFieldConditionGt(t *testing.T) {
	s := Status()
	field := s.Field("status.replicas")
	cond := field.Gt(0)

	condCUE := cond.ToCUECondition()
	expected := `context.output.status.replicas > 0`
	if condCUE != expected {
		t.Errorf("Expected %s, got: %s", expected, condCUE)
	}
}

func TestStatusConditionIs(t *testing.T) {
	s := Status()
	cond := s.Condition("Ready").Is("True")

	condCUE := cond.ToCUECondition()
	if !strings.Contains(condCUE, `status == "True"`) {
		t.Errorf("Expected condition to check status == True, got: %s", condCUE)
	}
	if !strings.Contains(condCUE, "len(_readyCond) > 0") {
		t.Errorf("Expected condition to check length, got: %s", condCUE)
	}
}

func TestStatusSwitch(t *testing.T) {
	s := Status()
	expr := s.Switch(
		s.Case(s.Field("status.phase").Eq("Running"), "Service is running"),
		s.Case(s.Field("status.phase").Eq("Pending"), "Service is starting..."),
		s.Default("Unknown status"),
	)

	cue := expr.(*statusSwitchExpr).BuildFull()

	if !strings.Contains(cue, `message: *"Unknown status"`) {
		t.Errorf("Expected default message, got: %s", cue)
	}
	if !strings.Contains(cue, `status.phase == "Running"`) {
		t.Errorf("Expected Running case, got: %s", cue)
	}
	if !strings.Contains(cue, `status.phase == "Pending"`) {
		t.Errorf("Expected Pending case, got: %s", cue)
	}
	if !strings.Contains(cue, `"Service is running"`) {
		t.Errorf("Expected running message, got: %s", cue)
	}
}

func TestStatusHealthAware(t *testing.T) {
	s := Status()
	expr := s.HealthAware(
		"All systems operational",
		s.Concat("Degraded: ", s.Condition("Ready").Message()),
	)

	cue := expr.(*statusHealthAwareExpr).BuildFull()

	if !strings.Contains(cue, "context.status.healthy") {
		t.Errorf("Expected health-aware to check context.status.healthy, got: %s", cue)
	}
	if !strings.Contains(cue, `"All systems operational"`) {
		t.Errorf("Expected healthy message, got: %s", cue)
	}
}

func TestStatusPolicy(t *testing.T) {
	s := Status()
	expr := s.Concat("Ready: ", s.Field("status.ready").Default(0))

	policy := StatusPolicy(expr)

	if !strings.Contains(policy, "message:") {
		t.Errorf("Expected policy to contain message:, got: %s", policy)
	}
	if !strings.Contains(policy, "_ready:") {
		t.Errorf("Expected policy to contain preamble, got: %s", policy)
	}
}

func TestStatusPolicyNoPreamble(t *testing.T) {
	s := Status()
	policy := StatusPolicy(s.Literal("Always OK"))

	expected := `message: "Always OK"`
	if policy != expected {
		t.Errorf("Expected %s, got: %s", expected, policy)
	}
}

func TestCustomStatusExpr(t *testing.T) {
	s := Status()
	cue := CustomStatusExpr(s.Literal("Service ready"))

	if !strings.Contains(cue, "message:") {
		t.Errorf("Expected CustomStatusExpr to generate message:, got: %s", cue)
	}
}

func TestCustomStatusExprWithSwitch(t *testing.T) {
	s := Status()
	cue := CustomStatusExpr(s.Switch(
		s.Case(s.Field("status.phase").Eq("Running"), "OK"),
		s.Default("Unknown"),
	))

	if !strings.Contains(cue, "message:") {
		t.Errorf("Expected CustomStatusExpr to generate message:, got: %s", cue)
	}
	if !strings.Contains(cue, `"Running"`) {
		t.Errorf("Expected switch case in output, got: %s", cue)
	}
}

func TestStatusBuilderStatusExpr(t *testing.T) {
	s := Status()
	s.StatusExpr(s.Concat("State: ", s.Field("status.state").Default("unknown")))

	cue := s.Build()

	if !strings.Contains(cue, "message:") {
		t.Errorf("Expected Build to return status expression, got: %s", cue)
	}
	if !strings.Contains(cue, "_state:") {
		t.Errorf("Expected preamble in output, got: %s", cue)
	}
}

func TestStatusBuilderRawCUE(t *testing.T) {
	s := Status()
	s.RawCUE(`message: "Custom CUE"`)

	cue := s.Build()
	expected := `message: "Custom CUE"`
	if cue != expected {
		t.Errorf("Expected %s, got: %s", expected, cue)
	}
}

func TestStatusBuilderRawCUEOverridesFields(t *testing.T) {
	s := Status()
	// Set both fields and rawCUE - rawCUE should win
	s.IntField("ready.replicas", "status.readyReplicas", 0)
	s.Message("Ready: \\(ready.replicas)")
	s.RawCUE(`message: "Override"`)

	cue := s.Build()
	expected := `message: "Override"`
	if cue != expected {
		t.Errorf("RawCUE should override field-based building, got: %s", cue)
	}
}

func TestComplexStatusExpression(t *testing.T) {
	s := Status()
	// Real-world example: Database CRD status
	expr := s.Concat(
		"State: ", s.Field("status.state").Default("initializing"),
		" | Connections: ", s.Field("status.connections").Default(0),
		" | Endpoint: ", s.Field("status.endpoint").Default("pending"),
	)

	cue := CustomStatusExpr(expr)

	// Should have all field preambles
	if !strings.Contains(cue, "_state:") {
		t.Errorf("Expected _state preamble, got: %s", cue)
	}
	if !strings.Contains(cue, "_connections:") {
		t.Errorf("Expected _connections preamble, got: %s", cue)
	}
	if !strings.Contains(cue, "_endpoint:") {
		t.Errorf("Expected _endpoint preamble, got: %s", cue)
	}
	if !strings.Contains(cue, "message:") {
		t.Errorf("Expected message: in output, got: %s", cue)
	}
}

func TestCrossplaneStyleStatus(t *testing.T) {
	s := Status()
	// Crossplane-style status with conditions
	expr := s.Switch(
		s.Case(s.Condition("Ready").Is("True"),
			s.Concat("Ready: ", s.Condition("Ready").Message())),
		s.Case(s.Condition("Synced").Is("False"),
			s.Concat("Syncing: ", s.Condition("Synced").Message())),
		s.Default(s.Concat(
			"Ready: ", s.Condition("Ready").StatusValue(),
			" | Synced: ", s.Condition("Synced").StatusValue(),
		)),
	)

	cue := CustomStatusExpr(expr)

	// Should have condition preambles
	if !strings.Contains(cue, "_readyCond:") {
		t.Errorf("Expected _readyCond preamble, got: %s", cue)
	}
	if !strings.Contains(cue, "_syncedCond:") {
		t.Errorf("Expected _syncedCond preamble, got: %s", cue)
	}
}

func TestComponentDefinitionWithStatusExpr(t *testing.T) {
	s := Status()

	// Test that StatusExpr integrates with ComponentDefinition
	comp := NewComponent("test-component").
		Description("A test component").
		CustomStatus(CustomStatusExpr(s.Concat(
			"Ready: ", s.Field("status.ready").Default(0),
		)))

	status := comp.GetCustomStatus()

	if !strings.Contains(status, "message:") {
		t.Errorf("Expected custom status to contain message:, got: %s", status)
	}
	if !strings.Contains(status, "_ready:") {
		t.Errorf("Expected custom status preamble, got: %s", status)
	}
}

func TestStatusFieldStringDefault(t *testing.T) {
	s := Status()
	field := s.Field("status.phase").Default("Unknown")

	preamble := field.Preamble()
	if !strings.Contains(preamble, `*"Unknown" | string`) {
		t.Errorf("Expected string default with quotes, got: %s", preamble)
	}
}

func TestStatusFieldBoolDefault(t *testing.T) {
	s := Status()
	field := s.Field("status.ready").Default(false)

	preamble := field.Preamble()
	if !strings.Contains(preamble, `*false | bool`) {
		t.Errorf("Expected bool default, got: %s", preamble)
	}
}

func TestWithDetails(t *testing.T) {
	s := Status()
	expr := s.WithDetails(
		s.Format("Ready: %v/%v", s.Field("status.readyReplicas").Default(0), s.SpecField("spec.replicas").Default(1)),
		s.Detail("endpoint", s.Field("status.endpoint").Default("pending")),
		s.Detail("version", s.Field("status.version").Default("unknown")),
	)

	// Verify it returns a valid StatusExpression
	if expr == nil {
		t.Fatal("WithDetails returned nil")
	}

	// Verify ToCUE returns the message part
	cue := expr.ToCUE()
	if cue == "" {
		t.Error("Expected ToCUE to return non-empty string")
	}

	// Verify preamble includes field extractions from message and details
	preamble := expr.Preamble()
	if !strings.Contains(preamble, "_readyReplicas:") {
		t.Errorf("Expected preamble to include _readyReplicas, got: %s", preamble)
	}
	if !strings.Contains(preamble, "_endpoint:") {
		t.Errorf("Expected preamble to include _endpoint, got: %s", preamble)
	}
	if !strings.Contains(preamble, "_version:") {
		t.Errorf("Expected preamble to include _version, got: %s", preamble)
	}
}

func TestWithDetailsInCustomStatusExpr(t *testing.T) {
	s := Status()
	cue := CustomStatusExpr(s.WithDetails(
		s.Literal("Service ready"),
		s.Detail("port", s.Field("status.port").Default(80)),
	))

	if !strings.Contains(cue, "message:") {
		t.Errorf("Expected message in output, got: %s", cue)
	}
	if !strings.Contains(cue, "_port:") {
		t.Errorf("Expected _port preamble in output, got: %s", cue)
	}
}
