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

// Phase 1: Validator + FieldRef tests

func TestValidatorBasic(t *testing.T) {
	// Simple validator: fail when condition is true
	v := Validate("tenantName must not end with a hyphen").
		WithName("_validateTenantName").
		FailWhen(ScopedField("tenantName").Matches(".*-$"))

	comp := NewComponent("test").
		Params(String("tenantName")).
		Validators(v)

	gen := NewCUEGenerator()
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, "_validateTenantName:") {
		t.Errorf("Expected _validateTenantName block, got:\n%s", cue)
	}
	if !strings.Contains(cue, `"tenantName must not end with a hyphen": true`) {
		t.Errorf("Expected message set to true, got:\n%s", cue)
	}
	if !strings.Contains(cue, `tenantName =~ ".*-$"`) {
		t.Errorf("Expected scoped field match condition, got:\n%s", cue)
	}
	if !strings.Contains(cue, `"tenantName must not end with a hyphen": false`) {
		t.Errorf("Expected message set to false in fail block, got:\n%s", cue)
	}
}

func TestValidatorGuarded(t *testing.T) {
	// Guarded validator: only active when guard is true
	replConfig := Object("replicationConfiguration").Optional()
	objectLock := Object("objectLock").Optional()
	versioningEnabled := Bool("versioningEnabled").Default(true)

	v := Validate("Require versioningEnabled to be true when replication or object lock is configured").
		WithName("_validateVersioning").
		OnlyWhen(Or(replConfig.IsSet(), objectLock.IsSet())).
		FailWhen(versioningEnabled.Eq(false))

	comp := NewComponent("test").
		Params(replConfig, objectLock, versioningEnabled).
		Validators(v)

	gen := NewCUEGenerator()
	cue := gen.GenerateParameterSchema(comp)

	// Should have the outer guard condition
	if !strings.Contains(cue, `replicationConfiguration"] != _|_`) || !strings.Contains(cue, `objectLock"] != _|_`) {
		t.Errorf("Expected guard condition with both IsSet checks, got:\n%s", cue)
	}
	// Should have the inner fail condition
	if !strings.Contains(cue, "parameter.versioningEnabled == false") {
		t.Errorf("Expected fail condition, got:\n%s", cue)
	}
}

func TestValidatorMutualExclusion(t *testing.T) {
	// Mutual exclusion: two fields cannot both be set
	v := Validate("Principal and NotPrincipal cannot be used together").
		WithName("_validatePrincipal").
		FailWhen(And(ScopedField("Principal").IsSet(), ScopedField("NotPrincipal").IsSet()))

	comp := NewComponent("test").
		Params(
			Object("statement").WithFields(
				String("Principal").Optional(),
				String("NotPrincipal").Optional(),
			).Validators(v),
		)

	gen := NewCUEGenerator()
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, "_validatePrincipal:") {
		t.Errorf("Expected _validatePrincipal block, got:\n%s", cue)
	}
	if !strings.Contains(cue, "Principal != _|_") && !strings.Contains(cue, `Principal != _|_`) {
		t.Errorf("Expected Principal IsSet condition, got:\n%s", cue)
	}
}

func TestMapParamValidators(t *testing.T) {
	// Validators inside a map param
	v := Validate("name is required").
		WithName("_validateName").
		FailWhen(ScopedField("name").Eq(""))

	mp := Object("governance").WithFields(
		String("name"),
		String("department"),
	).Validators(v)

	comp := NewComponent("test").Params(mp)
	gen := NewCUEGenerator()
	cue := gen.GenerateParameterSchema(comp)

	// Validator should be inside the governance struct
	if !strings.Contains(cue, "governance: {") {
		t.Errorf("Expected governance struct, got:\n%s", cue)
	}
	if !strings.Contains(cue, "_validateName:") {
		t.Errorf("Expected _validateName inside governance, got:\n%s", cue)
	}
}

func TestArrayParamValidators(t *testing.T) {
	// Validators inside array element struct
	v := Validate("action is required").
		WithName("_validateAction").
		FailWhen(ScopedField("action").Eq(""))

	arr := Array("rules").WithFields(
		String("action"),
		String("resource"),
	).Validators(v)

	comp := NewComponent("test").Params(arr)
	gen := NewCUEGenerator()
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, "[...{") {
		t.Errorf("Expected array of structs, got:\n%s", cue)
	}
	if !strings.Contains(cue, "_validateAction:") {
		t.Errorf("Expected _validateAction inside array elements, got:\n%s", cue)
	}
}

func TestArrayNonEmpty(t *testing.T) {
	arr := Array("allowedMethods").OfEnum("GET", "PUT", "HEAD", "POST", "DELETE").
		NonEmpty("allowedMethods cannot be empty - at least one method is required")

	comp := NewComponent("test").Params(arr)
	gen := NewCUEGenerator()
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, "if len(allowedMethods) == 0") {
		t.Errorf("Expected non-empty check, got:\n%s", cue)
	}
	if !strings.Contains(cue, `_|_("allowedMethods cannot be empty - at least one method is required")`) {
		t.Errorf("Expected error message in non-empty check, got:\n%s", cue)
	}
}

// Phase 2: ConditionalParams + ConditionalFields tests

func TestConditionalParamsTopLevel(t *testing.T) {
	existingResources := Bool("existingResources").Default(false)

	comp := NewComponent("test").
		Params(existingResources).
		ConditionalParams(ConditionalParams(
			WhenParam(existingResources.Eq(false)).Params(
				Bool("forceDestroy").Default(false),
				String("sseAlgorithm").Default("AES256").Values("AES256", "aws:kms"),
			),
			WhenParam(existingResources.Eq(true)).Params(
				Bool("forceDestroy").Optional(),
				String("sseAlgorithm").Optional().Values("AES256", "aws:kms"),
			),
		))

	gen := NewCUEGenerator()
	cue := gen.GenerateParameterSchema(comp)

	// Should have conditional blocks
	if !strings.Contains(cue, "if parameter.existingResources == false") {
		t.Errorf("Expected existingResources == false block, got:\n%s", cue)
	}
	if !strings.Contains(cue, "if parameter.existingResources == true") {
		t.Errorf("Expected existingResources == true block, got:\n%s", cue)
	}
	// Should have different defaults in each branch
	if !strings.Contains(cue, `forceDestroy: *false | bool`) {
		t.Errorf("Expected forceDestroy with default false, got:\n%s", cue)
	}
	if !strings.Contains(cue, "forceDestroy?: bool") {
		t.Errorf("Expected optional forceDestroy, got:\n%s", cue)
	}
}

func TestConditionalParamsWithValidators(t *testing.T) {
	existingResources := Bool("existingResources").Default(false)
	kmsMasterKeyId := String("kmsMasterKeyId").Optional()

	comp := NewComponent("test").
		Params(existingResources, kmsMasterKeyId).
		ConditionalParams(ConditionalParams(
			WhenParam(existingResources.Eq(false)).Params(
				String("sseAlgorithm").Default("AES256").Values("AES256", "aws:kms"),
			).Validators(
				Validate("kmsMasterKeyId can only be specified when sseAlgorithm is aws:kms").
					WithName("_validateKms").
					FailWhen(And(
						ScopedField("sseAlgorithm").Ne("aws:kms"),
						kmsMasterKeyId.IsSet(),
					)),
			),
		))

	gen := NewCUEGenerator()
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, "_validateKms:") {
		t.Errorf("Expected _validateKms inside conditional block, got:\n%s", cue)
	}
}

func TestConditionalFieldsInsideMap(t *testing.T) {
	existingResources := Bool("existingResources").Default(false)

	objectLock := Object("objectLock").Optional().ConditionalFields(
		WhenParam(existingResources.Eq(false)).Params(
			Int("retentionDays").Optional().Default(45).Min(1),
			String("retentionMode").Optional().Default("GOVERNANCE").Values("GOVERNANCE", "COMPLIANCE"),
		),
		WhenParam(existingResources.Eq(true)).Params(
			Int("retentionDays").Min(1),
			String("retentionMode").Values("GOVERNANCE", "COMPLIANCE"),
		),
	)

	comp := NewComponent("test").Params(existingResources, objectLock)
	gen := NewCUEGenerator()
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, "objectLock?: {") {
		t.Errorf("Expected optional objectLock struct, got:\n%s", cue)
	}
	if !strings.Contains(cue, "if parameter.existingResources == false") {
		t.Errorf("Expected conditional fields for existingResources == false, got:\n%s", cue)
	}
	if !strings.Contains(cue, `retentionDays?: *45 | int & >=1`) {
		t.Errorf("Expected retentionDays with default 45, got:\n%s", cue)
	}
}

// Phase 3: CUEExpr tests

func TestCUEExprCondition(t *testing.T) {
	existingResources := Bool("existingResources").Default(false)

	v := Validate("Combined name must be less than 64 characters").
		WithName("_validateNameLength").
		OnlyWhen(existingResources.Eq(false)).
		FailWhen(CUEExpr(`len("tenant-"+parameter.governance.tenantName+"-"+name) > 63`))

	comp := NewComponent("test").
		Params(existingResources).
		Validators(v)

	gen := NewCUEGenerator()
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, `len("tenant-"+parameter.governance.tenantName+"-"+name) > 63`) {
		t.Errorf("Expected raw CUE expression in validator, got:\n%s", cue)
	}
	if !strings.Contains(cue, "if parameter.existingResources == false") {
		t.Errorf("Expected guard condition, got:\n%s", cue)
	}
}

func TestCUEExprInAndCondition(t *testing.T) {
	v := Validate("complex check").
		WithName("_validateComplex").
		FailWhen(And(
			CUEExpr(`len(parameter.name) > 10`),
			CUEExpr(`parameter.name =~ "^test"`),
		))

	comp := NewComponent("test").Validators(v)
	gen := NewCUEGenerator()
	cue := gen.GenerateParameterSchema(comp)

	if !strings.Contains(cue, "len(parameter.name) > 10") {
		t.Errorf("Expected first raw CUE expr, got:\n%s", cue)
	}
	if !strings.Contains(cue, `parameter.name =~ "^test"`) {
		t.Errorf("Expected second raw CUE expr, got:\n%s", cue)
	}
}

// Phase 4: ConditionalStruct on Resource tests

func TestConditionalStructOnResource(t *testing.T) {
	replConfig := Object("replicationConfiguration").Optional()

	comp := NewComponent("test").
		Params(replConfig).
		Workload("apps/v1", "Deployment").
		Template(func(tpl *Template) {
			output := tpl.Output(NewResource("v1", "ConfigMap"))
			output.Set("metadata.name", Lit("test")).
				ConditionalStruct(replConfig.IsSet(), "spec.replicationConfiguration", func(b *OutputStructBuilder) {
					b.Set("role", Reference("parameter.replicationConfiguration.role"))
					b.SetIf(replConfig.IsSet(), "enabled", Lit(true))
				})
		})

	gen := NewCUEGenerator()
	cue := gen.GenerateTemplate(comp)

	if !strings.Contains(cue, `if parameter["replicationConfiguration"] != _|_`) {
		t.Errorf("Expected conditional guard for replication config, got:\n%s", cue)
	}
	if !strings.Contains(cue, "replicationConfiguration:") {
		t.Errorf("Expected replicationConfiguration struct, got:\n%s", cue)
	}
	if !strings.Contains(cue, "role: parameter.replicationConfiguration.role") {
		t.Errorf("Expected role field, got:\n%s", cue)
	}
}

func TestConditionalStructWithSetIf(t *testing.T) {
	existingResources := Bool("existingResources").Default(false)
	replConfig := Object("replicationConfiguration").Optional()

	comp := NewComponent("test").
		Params(existingResources, replConfig).
		Workload("apps/v1", "Deployment").
		Template(func(tpl *Template) {
			output := tpl.Output(NewResource("v1", "ConfigMap"))
			output.Set("metadata.name", Lit("test")).
				ConditionalStruct(replConfig.IsSet(), "spec.replication", func(b *OutputStructBuilder) {
					b.Set("role", Reference("parameter.replicationConfiguration.role"))
					b.SetIf(existingResources.Eq(false), "destinationBucketName", Lit("replica-bucket"))
				})
		})

	gen := NewCUEGenerator()
	cue := gen.GenerateTemplate(comp)

	if !strings.Contains(cue, "parameter.existingResources == false") {
		t.Errorf("Expected conditional SetIf inside conditional struct, got:\n%s", cue)
	}
	if !strings.Contains(cue, "destinationBucketName:") {
		t.Errorf("Expected destinationBucketName field, got:\n%s", cue)
	}
}

// Integration: All features combined

func TestAllFeaturesIntegrated(t *testing.T) {
	existingResources := Bool("existingResources").Default(false)
	governance := Object("governance").Closed().WithFields(
		String("tenantName").NotEmpty().NegativePattern(`.*-$`),
		String("departmentCode").NotEmpty(),
	).Validators(
		Validate("tenantName must not end with a hyphen").
			WithName("_validateTenant").
			FailWhen(ScopedField("tenantName").Matches(".*-$")),
	)

	comp := NewComponent("s3-bucket").
		Params(existingResources, governance).
		ConditionalParams(ConditionalParams(
			WhenParam(existingResources.Eq(false)).Params(
				Bool("forceDestroy").Default(false),
			),
			WhenParam(existingResources.Eq(true)).Params(
				Bool("forceDestroy").Optional(),
			),
		)).
		Validators(
			Validate("Combined name check").
				WithName("_validateName").
				OnlyWhen(existingResources.Eq(false)).
				FailWhen(CUEExpr(`len(parameter.governance.tenantName) > 63`)),
		)

	gen := NewCUEGenerator()
	cue := gen.GenerateParameterSchema(comp)

	// Verify all features present
	checks := []struct {
		desc     string
		expected string
	}{
		{"existingResources param", "existingResources: *false | bool"},
		{"governance struct", "governance: close({"},
		{"tenantName not empty", `!=""`},
		{"tenantName negative pattern", `!~".*-$"`},
		{"validator inside governance", "_validateTenant:"},
		{"conditional params false branch", "if parameter.existingResources == false"},
		{"conditional params true branch", "if parameter.existingResources == true"},
		{"default forceDestroy", "forceDestroy: *false | bool"},
		{"optional forceDestroy", "forceDestroy?: bool"},
		{"guarded validator", "_validateName:"},
		{"raw CUE expr", `len(parameter.governance.tenantName) > 63`},
	}

	for _, check := range checks {
		if !strings.Contains(cue, check.expected) {
			t.Errorf("%s: expected %q in output, got:\n%s", check.desc, check.expected, cue)
		}
	}
}
