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

// --- Schema Constraint Tests ---

func TestStringParamPattern(t *testing.T) {
	p := String("name").Pattern("^[a-z][a-z0-9-]*$")

	if p.GetPattern() != "^[a-z][a-z0-9-]*$" {
		t.Errorf("GetPattern() = %q, want %q", p.GetPattern(), "^[a-z][a-z0-9-]*$")
	}

	// Test CUE generation
	gen := NewCUEGenerator()
	comp := NewComponent("test").Params(p)
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, `=~"^[a-z][a-z0-9-]*$"`) {
		t.Errorf("Generated CUE should contain pattern constraint, got:\n%s", cue)
	}
}

func TestStringParamMinMaxLen(t *testing.T) {
	p := String("name").MinLen(3).MaxLen(63)

	minLen := p.GetMinLen()
	maxLen := p.GetMaxLen()

	if minLen == nil || *minLen != 3 {
		t.Errorf("GetMinLen() = %v, want 3", minLen)
	}
	if maxLen == nil || *maxLen != 63 {
		t.Errorf("GetMaxLen() = %v, want 63", maxLen)
	}

	// Test CUE generation
	gen := NewCUEGenerator()
	comp := NewComponent("test").Params(p)
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, "strings.MinRunes(3)") {
		t.Errorf("Generated CUE should contain MinRunes, got:\n%s", cue)
	}
	if !strings.Contains(cue, "strings.MaxRunes(63)") {
		t.Errorf("Generated CUE should contain MaxRunes, got:\n%s", cue)
	}
}

func TestIntParamMinMax(t *testing.T) {
	p := Int("replicas").Min(1).Max(100)

	minVal := p.GetMin()
	maxVal := p.GetMax()

	if minVal == nil || *minVal != 1 {
		t.Errorf("GetMin() = %v, want 1", minVal)
	}
	if maxVal == nil || *maxVal != 100 {
		t.Errorf("GetMax() = %v, want 100", maxVal)
	}

	// Test CUE generation
	gen := NewCUEGenerator()
	comp := NewComponent("test").Params(p)
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, ">=1") {
		t.Errorf("Generated CUE should contain >=1, got:\n%s", cue)
	}
	if !strings.Contains(cue, "<=100") {
		t.Errorf("Generated CUE should contain <=100, got:\n%s", cue)
	}
}

func TestFloatParamMinMax(t *testing.T) {
	p := Float("ratio").Min(0.0).Max(1.0)

	minVal := p.GetMin()
	maxVal := p.GetMax()

	if minVal == nil || *minVal != 0.0 {
		t.Errorf("GetMin() = %v, want 0.0", minVal)
	}
	if maxVal == nil || *maxVal != 1.0 {
		t.Errorf("GetMax() = %v, want 1.0", maxVal)
	}

	// Test CUE generation
	gen := NewCUEGenerator()
	comp := NewComponent("test").Params(p)
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, ">=0") {
		t.Errorf("Generated CUE should contain >=0, got:\n%s", cue)
	}
	if !strings.Contains(cue, "<=1") {
		t.Errorf("Generated CUE should contain <=1, got:\n%s", cue)
	}
}

func TestArrayParamMinMaxItems(t *testing.T) {
	p := Array("tags").Of(ParamTypeString).MinItems(1).MaxItems(10)

	minItems := p.GetMinItems()
	maxItems := p.GetMaxItems()

	if minItems == nil || *minItems != 1 {
		t.Errorf("GetMinItems() = %v, want 1", minItems)
	}
	if maxItems == nil || *maxItems != 10 {
		t.Errorf("GetMaxItems() = %v, want 10", maxItems)
	}

	// Test CUE generation
	gen := NewCUEGenerator()
	comp := NewComponent("test").Params(p)
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, "list.MinItems(1)") {
		t.Errorf("Generated CUE should contain MinItems, got:\n%s", cue)
	}
	if !strings.Contains(cue, "list.MaxItems(10)") {
		t.Errorf("Generated CUE should contain MaxItems, got:\n%s", cue)
	}
}

// --- Runtime Condition Tests ---

func TestStringParamContains(t *testing.T) {
	p := String("name")
	cond := p.Contains("prod")

	gen := NewCUEGenerator()
	cueStr := gen.conditionToCUE(cond)

	expected := `strings.Contains(parameter.name, "prod")`
	if cueStr != expected {
		t.Errorf("conditionToCUE() = %q, want %q", cueStr, expected)
	}
}

func TestStringParamMatches(t *testing.T) {
	p := String("name")
	cond := p.Matches("^prod-")

	gen := NewCUEGenerator()
	cueStr := gen.conditionToCUE(cond)

	expected := `parameter.name =~ "^prod-"`
	if cueStr != expected {
		t.Errorf("conditionToCUE() = %q, want %q", cueStr, expected)
	}
}

func TestStringParamStartsWith(t *testing.T) {
	p := String("name")
	cond := p.StartsWith("prod-")

	gen := NewCUEGenerator()
	cueStr := gen.conditionToCUE(cond)

	expected := `strings.HasPrefix(parameter.name, "prod-")`
	if cueStr != expected {
		t.Errorf("conditionToCUE() = %q, want %q", cueStr, expected)
	}
}

func TestStringParamEndsWith(t *testing.T) {
	p := String("name")
	cond := p.EndsWith("-prod")

	gen := NewCUEGenerator()
	cueStr := gen.conditionToCUE(cond)

	expected := `strings.HasSuffix(parameter.name, "-prod")`
	if cueStr != expected {
		t.Errorf("conditionToCUE() = %q, want %q", cueStr, expected)
	}
}

func TestStringParamIn(t *testing.T) {
	p := String("name")
	cond := p.In("api", "web", "worker")

	gen := NewCUEGenerator()
	cueStr := gen.conditionToCUE(cond)

	// Should contain all values
	if !strings.Contains(cueStr, `parameter.name == "api"`) {
		t.Errorf("conditionToCUE() should contain 'api', got: %s", cueStr)
	}
	if !strings.Contains(cueStr, `parameter.name == "web"`) {
		t.Errorf("conditionToCUE() should contain 'web', got: %s", cueStr)
	}
	if !strings.Contains(cueStr, " || ") {
		t.Errorf("conditionToCUE() should contain '||', got: %s", cueStr)
	}
}

func TestIntParamIn(t *testing.T) {
	p := Int("port")
	cond := p.In(80, 443, 8080)

	gen := NewCUEGenerator()
	cueStr := gen.conditionToCUE(cond)

	if !strings.Contains(cueStr, "parameter.port == 80") {
		t.Errorf("conditionToCUE() should contain '80', got: %s", cueStr)
	}
	if !strings.Contains(cueStr, "parameter.port == 443") {
		t.Errorf("conditionToCUE() should contain '443', got: %s", cueStr)
	}
}

func TestStringParamLenConditions(t *testing.T) {
	p := String("name")

	tests := []struct {
		name     string
		cond     Condition
		expected string
	}{
		{"LenEq", p.LenEq(5), "len(parameter.name) == 5"},
		{"LenGt", p.LenGt(5), "len(parameter.name) > 5"},
		{"LenGte", p.LenGte(5), "len(parameter.name) >= 5"},
		{"LenLt", p.LenLt(5), "len(parameter.name) < 5"},
		{"LenLte", p.LenLte(5), "len(parameter.name) <= 5"},
	}

	gen := NewCUEGenerator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cueStr := gen.conditionToCUE(tt.cond)
			if cueStr != tt.expected {
				t.Errorf("conditionToCUE() = %q, want %q", cueStr, tt.expected)
			}
		})
	}
}

func TestArrayParamConditions(t *testing.T) {
	p := Array("tags").Of(ParamTypeString)

	tests := []struct {
		name     string
		cond     Condition
		expected string
	}{
		{"LenEq", p.LenEq(5), "len(parameter.tags) == 5"},
		{"LenGt", p.LenGt(0), "len(parameter.tags) > 0"},
		{"IsEmpty", p.IsEmpty(), "len(parameter.tags) == 0"},
		{"IsNotEmpty", p.IsNotEmpty(), "len(parameter.tags) > 0"},
		{"Contains", p.Contains("gpu"), `list.Contains(parameter.tags, "gpu")`},
	}

	gen := NewCUEGenerator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cueStr := gen.conditionToCUE(tt.cond)
			if cueStr != tt.expected {
				t.Errorf("conditionToCUE() = %q, want %q", cueStr, tt.expected)
			}
		})
	}
}

func TestMapParamConditions(t *testing.T) {
	p := Map("config")

	tests := []struct {
		name     string
		cond     Condition
		expected string
	}{
		{"HasKey", p.HasKey("debug"), "parameter.config.debug != _|_"},
		{"LenEq", p.LenEq(5), "len(parameter.config) == 5"},
		{"LenGt", p.LenGt(0), "len(parameter.config) > 0"},
		{"IsEmpty", p.IsEmpty(), "len(parameter.config) == 0"},
		{"IsNotEmpty", p.IsNotEmpty(), "len(parameter.config) > 0"},
	}

	gen := NewCUEGenerator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cueStr := gen.conditionToCUE(tt.cond)
			if cueStr != tt.expected {
				t.Errorf("conditionToCUE() = %q, want %q", cueStr, tt.expected)
			}
		})
	}
}

func TestBoolParamIsFalse(t *testing.T) {
	p := Bool("enabled")
	cond := p.IsFalse()

	gen := NewCUEGenerator()
	cueStr := gen.conditionToCUE(cond)

	expected := "!parameter.enabled"
	if cueStr != expected {
		t.Errorf("conditionToCUE() = %q, want %q", cueStr, expected)
	}
}

// --- Chaining Tests ---

func TestSchemaConstraintChaining(t *testing.T) {
	// Test that schema constraint methods can be chained
	strP := String("name").
		Pattern("^[a-z]+$").
		MinLen(3).
		MaxLen(63).
		Description("The name")

	if strP.GetPattern() != "^[a-z]+$" {
		t.Error("Pattern not set correctly after chaining")
	}
	if strP.GetMinLen() == nil || *strP.GetMinLen() != 3 {
		t.Error("MinLen not set correctly after chaining")
	}
	if strP.GetMaxLen() == nil || *strP.GetMaxLen() != 63 {
		t.Error("MaxLen not set correctly after chaining")
	}
	if strP.GetDescription() != "The name" {
		t.Error("Description not set correctly after chaining")
	}

	intP := Int("replicas").
		Min(1).
		Max(100).
		Default(3)

	if intP.GetMin() == nil || *intP.GetMin() != 1 {
		t.Error("Min not set correctly after chaining")
	}
	if intP.GetMax() == nil || *intP.GetMax() != 100 {
		t.Error("Max not set correctly after chaining")
	}
	if !intP.HasDefault() || intP.GetDefault() != 3 {
		t.Error("Default not set correctly after chaining")
	}
}

// --- Combined Schema + Runtime Test ---

func TestCombinedSchemaAndRuntimeConditions(t *testing.T) {
	// Schema constraints define WHAT values are valid
	replicas := Int("replicas").Min(1).Max(100).Default(3)

	// Runtime conditions control WHAT resources are generated
	cond := replicas.Gt(5)

	gen := NewCUEGenerator()
	comp := NewComponent("test").Params(replicas)

	// Check schema generation
	schema := gen.GenerateParameterSchema(comp)
	if !strings.Contains(schema, ">=1") {
		t.Errorf("Schema should contain >=1, got:\n%s", schema)
	}
	if !strings.Contains(schema, "<=100") {
		t.Errorf("Schema should contain <=100, got:\n%s", schema)
	}
	if !strings.Contains(schema, "*3") {
		t.Errorf("Schema should contain default *3, got:\n%s", schema)
	}

	// Check runtime condition
	condStr := gen.conditionToCUE(cond)
	expected := "parameter.replicas > 5"
	if condStr != expected {
		t.Errorf("Runtime condition = %q, want %q", condStr, expected)
	}
}

// --- Integration Tests with SetIf ---

func TestSetIfWithNewConditions(t *testing.T) {
	name := String("name")
	replicas := Int("replicas").Min(1).Max(100)
	tags := Array("tags").Of(ParamTypeString)

	comp := NewComponent("test-app").
		Params(name, replicas, tags).
		Template(func(t *Template) {
			deployment := NewResource("apps/v1", "Deployment").
				Set("metadata.name", name).
				Set("spec.replicas", replicas).
				// Test various conditions with SetIf
				SetIf(name.StartsWith("prod-"), "metadata.labels.env", Lit("production")).
				SetIf(name.Contains("canary"), "metadata.labels.deployment", Lit("canary")).
				SetIf(replicas.Gt(5), "spec.strategy.type", Lit("RollingUpdate")).
				SetIf(tags.IsNotEmpty(), "metadata.annotations.has-tags", Lit("true")).
				SetIf(tags.Contains("gpu"), "spec.template.spec.nodeSelector.accelerator", Lit("nvidia"))

			t.Output(deployment)
		})

	cue := comp.ToCue()

	// Verify conditions are in the output
	expectedConditions := []string{
		`strings.HasPrefix(parameter.name, "prod-")`,
		`strings.Contains(parameter.name, "canary")`,
		`parameter.replicas > 5`,
		`len(parameter.tags) > 0`,
		`list.Contains(parameter.tags, "gpu")`,
	}

	for _, expected := range expectedConditions {
		if !strings.Contains(cue, expected) {
			t.Errorf("Generated CUE should contain %q, got:\n%s", expected, cue)
		}
	}
}

// --- Additional Edge Case Tests ---

func TestFloatParamIn(t *testing.T) {
	p := Float("ratio")
	cond := p.In(0.5, 1.0, 2.0)

	gen := NewCUEGenerator()
	cueStr := gen.conditionToCUE(cond)

	if !strings.Contains(cueStr, "parameter.ratio == 0.5") {
		t.Errorf("conditionToCUE() should contain '0.5', got: %s", cueStr)
	}
	if !strings.Contains(cueStr, "parameter.ratio == 1") {
		t.Errorf("conditionToCUE() should contain '1', got: %s", cueStr)
	}
	if !strings.Contains(cueStr, " || ") {
		t.Errorf("conditionToCUE() should contain '||', got: %s", cueStr)
	}
}

func TestArrayContainsWithDifferentTypes(t *testing.T) {
	gen := NewCUEGenerator()

	// String array with string value
	strArray := Array("tags").Of(ParamTypeString)
	strCond := strArray.Contains("value")
	strResult := gen.conditionToCUE(strCond)
	if strResult != `list.Contains(parameter.tags, "value")` {
		t.Errorf("String contains = %q", strResult)
	}

	// Int array with int value
	intArray := Array("ports").Of(ParamTypeInt)
	intCond := intArray.Contains(8080)
	intResult := gen.conditionToCUE(intCond)
	if intResult != `list.Contains(parameter.ports, 8080)` {
		t.Errorf("Int contains = %q", intResult)
	}

	// Bool value
	boolArray := Array("flags").Of(ParamTypeBool)
	boolCond := boolArray.Contains(true)
	boolResult := gen.conditionToCUE(boolCond)
	if boolResult != `list.Contains(parameter.flags, true)` {
		t.Errorf("Bool contains = %q", boolResult)
	}
}

func TestCombinedStringConstraints(t *testing.T) {
	// Test all string constraints together
	p := String("hostname").
		Pattern("^[a-z][a-z0-9-]*$").
		MinLen(3).
		MaxLen(63)

	gen := NewCUEGenerator()
	comp := NewComponent("test").Params(p)
	cue := gen.GenerateParameterSchema(comp)

	// All constraints should be present
	if !strings.Contains(cue, `=~"^[a-z][a-z0-9-]*$"`) {
		t.Errorf("Should contain pattern, got:\n%s", cue)
	}
	if !strings.Contains(cue, "strings.MinRunes(3)") {
		t.Errorf("Should contain MinRunes, got:\n%s", cue)
	}
	if !strings.Contains(cue, "strings.MaxRunes(63)") {
		t.Errorf("Should contain MaxRunes, got:\n%s", cue)
	}
	// They should be combined with &
	if !strings.Contains(cue, " & ") {
		t.Errorf("Constraints should be combined with &, got:\n%s", cue)
	}
}

func TestStringConstraintsWithDefault(t *testing.T) {
	p := String("env").
		Pattern("^(dev|staging|prod)$").
		Default("dev")

	gen := NewCUEGenerator()
	comp := NewComponent("test").Params(p)
	cue := gen.GenerateParameterSchema(comp)

	// Should have both default and pattern
	if !strings.Contains(cue, `*"dev"`) {
		t.Errorf("Should contain default, got:\n%s", cue)
	}
	if !strings.Contains(cue, `=~"^(dev|staging|prod)$"`) {
		t.Errorf("Should contain pattern, got:\n%s", cue)
	}
}

func TestIntConstraintsWithDefault(t *testing.T) {
	p := Int("port").
		Min(1).
		Max(65535).
		Default(8080)

	gen := NewCUEGenerator()
	comp := NewComponent("test").Params(p)
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, "*8080") {
		t.Errorf("Should contain default, got:\n%s", cue)
	}
	if !strings.Contains(cue, ">=1") {
		t.Errorf("Should contain min, got:\n%s", cue)
	}
	if !strings.Contains(cue, "<=65535") {
		t.Errorf("Should contain max, got:\n%s", cue)
	}
}

func TestEdgeCaseZeroValues(t *testing.T) {
	// Min of 0 should still be generated
	p := Int("count").Min(0).Max(10)

	gen := NewCUEGenerator()
	comp := NewComponent("test").Params(p)
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, ">=0") {
		t.Errorf("Should contain >=0, got:\n%s", cue)
	}
}

func TestEdgeCaseEmptyStringConditions(t *testing.T) {
	gen := NewCUEGenerator()
	p := String("name")

	// Empty string checks should work
	containsEmpty := p.Contains("")
	result := gen.conditionToCUE(containsEmpty)
	if result != `strings.Contains(parameter.name, "")` {
		t.Errorf("Empty contains = %q", result)
	}

	startsEmpty := p.StartsWith("")
	result2 := gen.conditionToCUE(startsEmpty)
	if result2 != `strings.HasPrefix(parameter.name, "")` {
		t.Errorf("Empty startsWith = %q", result2)
	}
}

func TestSingleValueIn(t *testing.T) {
	gen := NewCUEGenerator()
	p := String("status")

	// Single value In should work (even if it's equivalent to Eq)
	cond := p.In("active")
	result := gen.conditionToCUE(cond)

	if result != `parameter.status == "active"` {
		t.Errorf("Single value In = %q, want parameter.status == \"active\"", result)
	}
}

func TestSpecialCharactersInPattern(t *testing.T) {
	// Patterns with special regex characters
	p := String("email").Pattern(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	gen := NewCUEGenerator()
	comp := NewComponent("test").Params(p)
	cue := gen.GenerateParameterSchema(comp)

	// Pattern should be properly quoted
	if !strings.Contains(cue, `=~"^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$"`) {
		t.Errorf("Pattern with special chars should be escaped, got:\n%s", cue)
	}
}
