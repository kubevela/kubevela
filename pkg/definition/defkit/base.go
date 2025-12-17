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
	"github.com/oam-dev/kubevela/pkg/definition/defkit/placement"
)

// baseDefinition contains fields and methods common to all X-Definition types.
// This struct is embedded in TraitDefinition, ComponentDefinition, and other
// definition types to eliminate code duplication and ensure consistent behavior.
//
// Common fields include:
// - name: the definition name (e.g., "scaler", "webservice")
// - description: human-readable description
// - params: parameter schema definitions
// - template: template function for generating output
// - customStatus: CUE expression for custom status
// - healthPolicy: CUE expression for health checking
// - helperDefinitions: helper type definitions like #HealthProbe
// - rawCUE: escape hatch for complex CUE not expressible via fluent API
// - imports: CUE imports needed by the definition
type baseDefinition struct {
	name              string
	description       string
	params            []Param
	template          func(tpl *Template)
	customStatus      string
	healthPolicy      string
	helperDefinitions []HelperDefinition
	rawCUE            string
	imports           []string
	// Placement constraints for cluster-aware definition deployment
	runOn    []placement.Condition
	notRunOn []placement.Condition
}

// --- Builder methods (used by embedding types) ---

// setDescription sets the definition description.
func (b *baseDefinition) setDescription(desc string) {
	b.description = desc
}

// addParams adds parameter definitions.
func (b *baseDefinition) addParams(params ...Param) {
	b.params = append(b.params, params...)
}

// setTemplate sets the template function.
func (b *baseDefinition) setTemplate(fn func(tpl *Template)) {
	b.template = fn
}

// setCustomStatus sets the custom status CUE expression.
func (b *baseDefinition) setCustomStatus(expr string) {
	b.customStatus = expr
}

// setHealthPolicy sets the health policy CUE expression.
func (b *baseDefinition) setHealthPolicy(expr string) {
	b.healthPolicy = expr
}

// setHealthPolicyExpr sets the health policy using a composable HealthExpression.
func (b *baseDefinition) setHealthPolicyExpr(expr HealthExpression) {
	b.healthPolicy = HealthPolicy(expr)
}

// addHelper adds a helper type definition using fluent API.
func (b *baseDefinition) addHelper(name string, param Param) {
	b.helperDefinitions = append(b.helperDefinitions, HelperDefinition{name: name, param: param})
}

// setRawCUE sets raw CUE for complex definitions.
func (b *baseDefinition) setRawCUE(cue string) {
	b.rawCUE = cue
}

// addImports adds CUE imports.
func (b *baseDefinition) addImports(imports ...string) {
	b.imports = append(b.imports, imports...)
}

// addRunOn adds placement conditions specifying where this definition should run.
func (b *baseDefinition) addRunOn(conditions ...placement.Condition) {
	b.runOn = append(b.runOn, conditions...)
}

// addNotRunOn adds placement conditions specifying where this definition should NOT run.
func (b *baseDefinition) addNotRunOn(conditions ...placement.Condition) {
	b.notRunOn = append(b.notRunOn, conditions...)
}

// --- Getter methods (shared by all definition types) ---

// GetName returns the definition name.
func (b *baseDefinition) GetName() string {
	return b.name
}

// GetDescription returns the definition description.
func (b *baseDefinition) GetDescription() string {
	return b.description
}

// GetParams returns all parameter definitions.
func (b *baseDefinition) GetParams() []Param {
	return b.params
}

// GetTemplate returns the template function.
func (b *baseDefinition) GetTemplate() func(tpl *Template) {
	return b.template
}

// GetCustomStatus returns the custom status CUE expression.
func (b *baseDefinition) GetCustomStatus() string {
	return b.customStatus
}

// GetHealthPolicy returns the health policy CUE expression.
func (b *baseDefinition) GetHealthPolicy() string {
	return b.healthPolicy
}

// GetHelperDefinitions returns all helper type definitions.
func (b *baseDefinition) GetHelperDefinitions() []HelperDefinition {
	return b.helperDefinitions
}

// GetRawCUE returns the raw CUE template if set.
func (b *baseDefinition) GetRawCUE() string {
	return b.rawCUE
}

// GetImports returns the CUE imports.
func (b *baseDefinition) GetImports() []string {
	return b.imports
}

// HasTemplate returns true if the definition has a template function set.
func (b *baseDefinition) HasTemplate() bool {
	return b.template != nil
}

// HasRawCUE returns true if raw CUE is set.
func (b *baseDefinition) HasRawCUE() bool {
	return b.rawCUE != ""
}

// GetRunOn returns the RunOn placement conditions.
func (b *baseDefinition) GetRunOn() []placement.Condition {
	return b.runOn
}

// GetNotRunOn returns the NotRunOn placement conditions.
func (b *baseDefinition) GetNotRunOn() []placement.Condition {
	return b.notRunOn
}

// GetPlacement returns the complete placement spec for this definition.
func (b *baseDefinition) GetPlacement() placement.PlacementSpec {
	return placement.PlacementSpec{
		RunOn:    b.runOn,
		NotRunOn: b.notRunOn,
	}
}

// HasPlacement returns true if the definition has any placement constraints.
func (b *baseDefinition) HasPlacement() bool {
	return len(b.runOn) > 0 || len(b.notRunOn) > 0
}

// GetRawCUEWithName returns the raw CUE with the definition name rewritten
// to match the name set via NewComponent/NewTrait/etc.
// This ensures the name passed to the fluent builder takes precedence over
// any name embedded in the raw CUE string.
func (b *baseDefinition) GetRawCUEWithName() string {
	return RewriteRawCUEName(b.rawCUE, b.name)
}

// RewriteRawCUEName rewrites the first definition name in a raw CUE string
// to use the specified name. This handles patterns like:
//   - "old-name": { ... }           -> "new-name": { ... }
//   - "old-name.v1": { ... }        -> "new-name": { ... }
//
// The function finds the first quoted string followed by a colon and replaces it.
func RewriteRawCUEName(rawCUE, newName string) string {
	if rawCUE == "" || newName == "" {
		return rawCUE
	}

	// Find the first quoted definition name pattern: "name": {
	// We look for: optional whitespace, quote, name, quote, colon
	inQuote := false
	quoteStart := -1
	quoteEnd := -1

	for i, c := range rawCUE {
		if c == '"' {
			if !inQuote {
				inQuote = true
				quoteStart = i
			} else {
				quoteEnd = i
				// Check if followed by colon (with optional whitespace)
				rest := rawCUE[quoteEnd+1:]
				for j, r := range rest {
					if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
						continue
					}
					if r == ':' {
						// Found the definition name pattern - replace it
						return rawCUE[:quoteStart+1] + newName + rawCUE[quoteEnd:]
					}
					// Not followed by colon, keep looking
					_ = j
					break
				}
				inQuote = false
				quoteStart = -1
				quoteEnd = -1
			}
		}
	}

	return rawCUE
}
