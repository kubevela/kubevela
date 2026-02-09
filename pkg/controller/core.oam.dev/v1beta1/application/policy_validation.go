/*
Copyright 2021 The KubeVela Authors.

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
	"cuelang.org/go/cue/cuecontext"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
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
		// Rule 1: No required parameters
		if err := validateNoRequiredParameters(policy); err != nil {
			result.Errors = append(result.Errors, err.Error())
		}

		// Rule 2: Scope must be Application
		if policy.Spec.Scope != v1beta1.ApplicationScope {
			result.Errors = append(result.Errors,
				fmt.Sprintf("global policies must have scope='Application', found scope='%s'", policy.Spec.Scope))
		}

		// Rule 3 (Warning): Priority should be explicit
		if policy.Spec.Priority == 0 {
			result.Warnings = append(result.Warnings,
				"global policy should have explicit priority field (defaults to 0)")
		}

		// Rule 4 (Warning): Priority range recommendation
		if policy.Spec.Priority > 2000 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("priority %d is unusually high (recommended ranges: 1000-1999=security, 500-999=standard, 100-499=optional, 0-99=low)",
					policy.Spec.Priority))
		}
	}

	// Rule 5: CUE schema validation (all policies)
	if err := validateCUESchema(policy); err != nil {
		result.Errors = append(result.Errors, err.Error())
	}

	return result
}

// validateNoRequiredParameters checks that all parameters have default values
// For global policies, ALL parameters must have defaults since users can't provide values
func validateNoRequiredParameters(policy *v1beta1.PolicyDefinition) error {
	cueTemplate := policy.Spec.Schematic.CUE.Template

	// Parse CUE template
	ctx := cuecontext.New()
	value := ctx.CompileString(cueTemplate)
	if value.Err() != nil {
		return errors.Wrap(value.Err(), "failed to compile CUE template")
	}

	// Look up the parameter field
	paramField := value.LookupPath(cue.ParsePath("parameter"))
	if !paramField.Exists() {
		// No parameter field is fine
		return nil
	}

	// Check if parameter is just empty struct
	fields, err := paramField.Fields(cue.All())
	if err != nil {
		return errors.Wrap(err, "failed to inspect parameter fields")
	}

	// Collect parameters without defaults (both required AND optional without defaults)
	var paramsWithoutDefaults []string

	for fields.Next() {
		fieldInfo := fields.Selector()
		fieldValue := fields.Value()

		// Check if field has a default value (has * marker)
		_, hasDefault := fieldValue.Default()
		if !hasDefault {
			// No default = can't compile when auto-applied
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

// validateCUESchema validates the CUE template conforms to expected schema
func validateCUESchema(policy *v1beta1.PolicyDefinition) error {
	cueTemplate := policy.Spec.Schematic.CUE.Template

	// Parse CUE template
	ctx := cuecontext.New()
	value := ctx.CompileString(cueTemplate)
	if value.Err() != nil {
		return errors.Wrap(value.Err(), "CUE template syntax error")
	}

	// Validate 'enabled' field (must be bool or default to true)
	enabledField := value.LookupPath(cue.ParsePath("enabled"))
	if enabledField.Exists() {
		// Check if it unifies with bool
		boolType := ctx.CompileString("bool")
		unified := enabledField.Unify(boolType)
		if unified.Err() != nil {
			return errors.New("'enabled' field must be of type bool")
		}
	}

	// Validate 'transforms' field structure
	transformsField := value.LookupPath(cue.ParsePath("transforms"))
	if transformsField.Exists() {
		if err := validateTransforms(ctx, transformsField); err != nil {
			return err
		}
	}

	return nil
}

// validateTransforms validates the transforms field structure
func validateTransforms(ctx *cue.Context, transforms cue.Value) error {
	// Check labels transform (if exists)
	labelsTransform := transforms.LookupPath(cue.ParsePath("labels"))
	if labelsTransform.Exists() {
		if err := validateMergeOnlyTransform(labelsTransform, "labels"); err != nil {
			return err
		}
	}

	// Check annotations transform (if exists)
	annotationsTransform := transforms.LookupPath(cue.ParsePath("annotations"))
	if annotationsTransform.Exists() {
		if err := validateMergeOnlyTransform(annotationsTransform, "annotations"); err != nil {
			return err
		}
	}

	// Check spec transform (if exists)
	specTransform := transforms.LookupPath(cue.ParsePath("spec"))
	if specTransform.Exists() {
		if err := validateSpecTransform(specTransform); err != nil {
			return err
		}
	}

	return nil
}

// validateMergeOnlyTransform validates that labels/annotations only use "merge" type
func validateMergeOnlyTransform(transform cue.Value, fieldName string) error {
	typeField := transform.LookupPath(cue.ParsePath("type"))
	if !typeField.Exists() {
		return fmt.Errorf("transforms.%s must have 'type' field", fieldName)
	}

	typeStr, err := typeField.String()
	if err != nil {
		return fmt.Errorf("transforms.%s.type must be a string", fieldName)
	}

	if typeStr != "merge" {
		return fmt.Errorf("transforms.%s.type must be 'merge' (not '%s') to prevent removing critical platform metadata",
			fieldName, typeStr)
	}

	// Check value field exists
	valueField := transform.LookupPath(cue.ParsePath("value"))
	if !valueField.Exists() {
		return fmt.Errorf("transforms.%s must have 'value' field", fieldName)
	}

	return nil
}

// validateSpecTransform validates spec transform type is "merge" or "replace"
func validateSpecTransform(transform cue.Value) error {
	typeField := transform.LookupPath(cue.ParsePath("type"))
	if !typeField.Exists() {
		return errors.New("transforms.spec must have 'type' field")
	}

	typeStr, err := typeField.String()
	if err != nil {
		return errors.New("transforms.spec.type must be a string")
	}

	if typeStr != "merge" && typeStr != "replace" {
		return fmt.Errorf("transforms.spec.type must be 'merge' or 'replace' (found '%s')", typeStr)
	}

	// Check value field exists
	valueField := transform.LookupPath(cue.ParsePath("value"))
	if !valueField.Exists() {
		return errors.New("transforms.spec must have 'value' field")
	}

	return nil
}
