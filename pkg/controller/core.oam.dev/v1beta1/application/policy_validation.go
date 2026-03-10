/*
Copyright 2026 The KubeVela Authors.

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

package application

import (
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"github.com/pkg/errors"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	webhookutils "github.com/oam-dev/kubevela/pkg/webhook/utils"
)

// PolicyValidationResult contains validation errors and warnings
type PolicyValidationResult struct {
	Errors   []string
	Warnings []string
}

// IsValid returns true if there are no errors
func (r *PolicyValidationResult) IsValid() bool {
	return len(r.Errors) == 0
}

// ValidatePolicyDefinition validates a PolicyDefinition
// Returns validation result with errors (blocking) and warnings (informational)
func ValidatePolicyDefinition(policy *v1beta1.PolicyDefinition) *PolicyValidationResult {
	result := &PolicyValidationResult{
		Errors:   []string{},
		Warnings: []string{},
	}

	// Only validate if policy has a schematic
	if policy.Spec.Schematic == nil || policy.Spec.Schematic.CUE == nil {
		result.Errors = append(result.Errors, "policy must have a CUE schematic")
		return result
	}

	// Validate global policies have specific requirements
	if policy.Spec.Global {
		// No required parameters
		if err := validateNoRequiredParameters(policy); err != nil {
			result.Errors = append(result.Errors, err.Error())
		}

		// Scope must be Application
		if policy.Spec.Scope != v1beta1.ApplicationScope {
			result.Errors = append(result.Errors,
				fmt.Sprintf("global policies must have scope='Application', found scope='%s'", policy.Spec.Scope))
		}

		// (Warning): Priority should be explicit
		if policy.Spec.Priority == 0 {
			result.Warnings = append(result.Warnings,
				"global policy should have explicit priority field (defaults to 0)")
		}

		// (Warning): Priority range recommendation
		if policy.Spec.Priority > 9999 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("priority %d is unusually high",
					policy.Spec.Priority))
		}
	}

	// CUE syntax validation (output field structure is validated at render time)
	if err := webhookutils.ValidateCueTemplate(policy.Spec.Schematic.CUE.Template); err != nil {
		result.Errors = append(result.Errors, err.Error())
	} else if err := validateEnabledFieldType(policy.Spec.Schematic.CUE.Template); err != nil {
		result.Errors = append(result.Errors, err.Error())
	}

	// Feature gate warnings — policy will be created but won't execute until the flag is enabled.
	if policy.Spec.Scope == v1beta1.ApplicationScope && !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableApplicationScopedPolicies) {
		result.Warnings = append(result.Warnings,
			"EnableApplicationScopedPolicies feature gate is disabled; this policy will not execute until it is enabled")
	}
	if policy.Spec.Global && !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableGlobalPolicies) {
		result.Warnings = append(result.Warnings,
			"EnableGlobalPolicies feature gate is disabled; this policy will not be auto-applied until it is enabled")
	}

	return result
}

// validateEnabledFieldType checks that the top-level 'enabled' field, if present, is of type bool.
// Uses the CUE AST to inspect the source without evaluation (no runtime context needed).
func validateEnabledFieldType(cueTemplate string) error {
	f, _ := parser.ParseFile("policy.cue", cueTemplate, parser.ParseComments)
	if f == nil {
		return nil // syntax errors already caught by ValidateCueTemplate
	}
	for _, decl := range f.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		label, _, err := ast.LabelName(field.Label)
		if err != nil || label != "enabled" {
			continue
		}
		// 'enabled' found — check its value expression
		if !isASTBoolExpr(field.Value) {
			return fmt.Errorf("'enabled' field must be of type bool, got a non-bool value")
		}
	}
	return nil
}

// isASTBoolExpr returns true if the AST expression is bool-typed:
// a bool literal (true/false), the identifier 'bool', or a disjunction of bool literals.
func isASTBoolExpr(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Kind == token.TRUE || e.Kind == token.FALSE
	case *ast.Ident:
		return e.Name == "bool"
	case *ast.BinaryExpr:
		// disjunction: true | false, bool | true, etc.
		if e.Op == token.OR {
			return isASTBoolExpr(e.X) && isASTBoolExpr(e.Y)
		}
	case *ast.UnaryExpr:
		// default marker: *true | bool
		return isASTBoolExpr(e.X)
	}
	return false
}

// validateNoRequiredParameters checks that all parameters have default values
// For global policies, ALL parameters must have defaults since users can't provide values
func validateNoRequiredParameters(policy *v1beta1.PolicyDefinition) error {
	cueTemplate := policy.Spec.Schematic.CUE.Template

	ctx := cuecontext.New()
	value := ctx.CompileString(cueTemplate)
	if value.Err() != nil {
		return errors.Wrap(value.Err(), "failed to compile CUE template")
	}

	paramField := value.LookupPath(cue.ParsePath("parameter"))
	if !paramField.Exists() {
		return nil
	}

	fields, err := paramField.Fields(cue.All())
	if err != nil {
		return errors.Wrap(err, "failed to inspect parameter fields")
	}

	var paramsWithoutDefaults []string
	for fields.Next() {
		fieldInfo := fields.Selector()
		fieldValue := fields.Value()
		_, hasDefault := fieldValue.Default()
		if !hasDefault {
			paramsWithoutDefaults = append(paramsWithoutDefaults, fieldInfo.String())
		}
	}

	if len(paramsWithoutDefaults) > 0 {
		return fmt.Errorf("global policy '%s' cannot have parameters without default values. Found parameters without defaults: %v. "+
			"Global policies are auto-applied without user input, so all parameters must have default values using '*'. "+
			"Example: envName: *\"production\" | string",
			policy.Name, paramsWithoutDefaults)
	}

	return nil
}
