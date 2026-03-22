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

package appfile

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"github.com/jeremywohl/flatten/v2"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	cueutils "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/features"

	"github.com/pkg/errors"

	"github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
)

// ValidateCUESchematicAppfile validates CUE schematic workloads in an Appfile
func (p *Parser) ValidateCUESchematicAppfile(a *Appfile) error {
	for _, wl := range a.ParsedComponents {
		// because helm & kube schematic has no CUE template
		// it only validates CUE schematic workload
		if wl.CapabilityCategory != types.CUECategory || wl.Type == v1alpha1.RefObjectsComponentType {
			continue
		}

		ctxData := GenerateContextDataFromAppFile(a, wl.Name)
		if utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableCueValidation) {
			err := p.ValidateComponentParams(ctxData, wl, a)
			if err != nil {
				return err
			}
		}

		// Collect workflow-supplied params for this component upfront
		workflowParams := getWorkflowAndPolicySuppliedParams(a)

		// Only augment if component has traits AND workflow supplies params (issue 7022)
		originalParams := wl.Params
		if len(wl.Traits) > 0 && len(workflowParams) > 0 {
			shouldSkip, augmented := p.augmentComponentParamsForValidation(wl, workflowParams, ctxData)
			if shouldSkip {
				// Component has complex validation that can't be handled, skip trait validation
				fmt.Printf("INFO: Skipping trait validation for component %q due to workflow-supplied parameters with complex validation\n", wl.Name)
				continue
			}
			wl.Params = augmented
		}

		pCtx, err := newValidationProcessContext(wl, ctxData)
		wl.Params = originalParams // Restore immediately

		if err != nil {
			return errors.WithMessagef(err, "cannot create the validation process context of app=%s in namespace=%s", a.Name, a.Namespace)
		}

		for _, tr := range wl.Traits {
			if tr.CapabilityCategory != types.CUECategory {
				continue
			}
			if tr.FullTemplate != nil &&
				tr.FullTemplate.TraitDefinition.Spec.Stage == v1beta1.PostDispatch {
				// PostDispatch type trait validation at this point might fail as they could have
				// references to fields that are populated/injected during runtime only
				continue
			}
			if err := tr.EvalContext(pCtx); err != nil {
				return errors.WithMessagef(err, "cannot evaluate trait %q", tr.Name)
			}
		}
	}
	return nil
}

// ValidateComponentParams performs CUE‑level validation for a Component’s
// parameters and emits helpful, context‑rich errors.
//
// Flow
//  1. Assemble a synthetic CUE document (template + params + app context).
//  2. Compile it; if compilation fails, return the compiler error.
//  3. When the EnableCueValidation gate is on, ensure *all* non‑optional,
//     non‑defaulted parameters are provided—either in the Component.Params
//     block or as workflow‑step inputs.
//  4. Run cue.Value.Validate to enforce user‑supplied values against
//     template constraints.
func (p *Parser) ValidateComponentParams(ctxData velaprocess.ContextData, wl *Component, app *Appfile) error {
	// ---------------------------------------------------------------------
	// 1. Build synthetic CUE source
	// ---------------------------------------------------------------------
	ctx := velaprocess.NewContext(ctxData)
	baseCtx, err := ctx.BaseContextFile()
	if err != nil {
		return errors.WithStack(err)
	}

	paramSnippet, err := cueParamBlock(wl.Params)
	if err != nil {
		return errors.WithMessagef(err, "component %q: invalid params", wl.Name)
	}

	cueSrc := strings.Join([]string{
		renderTemplate(wl.FullTemplate.TemplateStr),
		paramSnippet,
		baseCtx,
	}, "\n")

	val, err := cuex.DefaultCompiler.Get().CompileString(ctx.GetCtx(), cueSrc)
	if err != nil {
		return errors.WithMessagef(err, "component %q: CUE compile error", wl.Name)
	}

	// ---------------------------------------------------------------------
	// 2. Strict required‑field enforcement (feature‑gated)
	// ---------------------------------------------------------------------
	if err := enforceRequiredParams(val, wl.Params, app); err != nil {
		return errors.WithMessagef(err, "component %q", wl.Name)
	}

	// ---------------------------------------------------------------------
	// 3. Validate concrete values
	// ---------------------------------------------------------------------
	paramVal := val.LookupPath(value.FieldPath(velaprocess.ParameterFieldName))
	if err := paramVal.Validate(cue.Concrete(false)); err != nil {
		return errors.WithMessagef(err, "component %q: parameter constraint violation", wl.Name)
	}

	// ---------------------------------------------------------------------
	// 4. Reject undeclared parameters (feature-gated)
	// ---------------------------------------------------------------------
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.ValidateUndeclaredParameters) {
		// Compile the template WITHOUT user params to get the pure schema.
		schemaSrc := strings.Join([]string{
			renderTemplate(wl.FullTemplate.TemplateStr),
			baseCtx,
		}, "\n")
		schemaRoot, schemaErr := cuex.DefaultCompiler.Get().CompileString(ctx.GetCtx(), schemaSrc)
		if schemaErr != nil {
			klog.V(4).Infof("component %q: skipping undeclared parameter check: schema compilation failed: %v", wl.Name, schemaErr)
		} else {
			paramSchema := schemaRoot.LookupPath(value.FieldPath(velaprocess.ParameterFieldName))
			undeclared := findUndeclaredFields(paramSchema, wl.Params, "")

			// Second pass: resolve conditional parameter declarations by
			// re-compiling with only the base-declared params so CUE
			// conditionals (e.g. if mode == "x" { field?: T }) evaluate.
			if len(undeclared) > 0 {
				baseDeclared := getDeclaredFieldNames(paramSchema)
				if len(baseDeclared) > 0 {
					filteredParams := make(map[string]any)
					for k, v := range wl.Params {
						if baseDeclared[k] {
							filteredParams[k] = v
						}
					}
					condSnippet, fErr := cueParamBlock(filteredParams)
					if fErr == nil {
						condSrc := strings.Join([]string{
							renderTemplate(wl.FullTemplate.TemplateStr),
							condSnippet,
							baseCtx,
						}, "\n")
						condRoot, condErr := cuex.DefaultCompiler.Get().CompileString(ctx.GetCtx(), condSrc)
						if condErr == nil {
							condSchema := condRoot.LookupPath(value.FieldPath(velaprocess.ParameterFieldName))
							undeclared = findUndeclaredFields(condSchema, wl.Params, "")
						}
					}
				}
			}

			if len(undeclared) > 0 {
				sort.Strings(undeclared)
				return errors.WithMessagef(
					fmt.Errorf("undeclared parameters: %s", strings.Join(undeclared, ",")),
					"component %q", wl.Name)
			}
		}
	}

	return nil
}

// checkUndeclaredParams verifies that every key in params is declared in the
// CUE parameter schema. Returns an error listing any undeclared field paths.
func checkUndeclaredParams(schema cue.Value, params map[string]any) error {
	if len(params) == 0 {
		return nil
	}
	undeclared := findUndeclaredFields(schema, params, "")
	if len(undeclared) > 0 {
		sort.Strings(undeclared)
		return fmt.Errorf("undeclared parameters: %s", strings.Join(undeclared, ","))
	}
	return nil
}

// findUndeclaredFields recursively walks the user-provided params and collects
// field paths that are not declared in the CUE schema (including optional fields).
func findUndeclaredFields(schema cue.Value, params map[string]any, prefix string) []string {
	// If the schema has a pattern constraint ([string]: T), all string-keyed
	// fields are valid at this level (e.g., labels?: [string]: string).
	if schema.LookupPath(cue.MakePath(cue.AnyString)).Exists() {
		return nil
	}

	// Collect declared field names from the schema (required + optional).
	declared := make(map[string]cue.Value)
	it, err := schema.Fields(cue.Optional(true), cue.Definitions(false), cue.Hidden(false))
	if err != nil {
		// Cannot enumerate schema fields; skip undeclared check at this level
		// to avoid false positives when field iteration fails.
		return nil
	}
	for it.Next() {
		declared[cueutils.GetSelectorLabel(it.Selector())] = it.Value()
	}

	var undeclared []string
	for key, val := range params {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		fieldSchema, ok := declared[key]
		if !ok {
			undeclared = append(undeclared, path)
			continue
		}
		// Recurse into nested structs.
		if nested, isMap := val.(map[string]any); isMap && fieldSchema.IncompleteKind() == cue.StructKind {
			undeclared = append(undeclared, findUndeclaredFields(fieldSchema, nested, path)...)
		}
		// Recurse into list elements that contain structs.
		if items, isList := val.([]any); isList && fieldSchema.IncompleteKind() == cue.ListKind {
			elemSchema := fieldSchema.LookupPath(cue.MakePath(cue.AnyIndex))
			if elemSchema.Exists() && elemSchema.IncompleteKind() == cue.StructKind {
				for i, item := range items {
					if nested, isMap := item.(map[string]any); isMap {
						elemPath := fmt.Sprintf("%s[%d]", path, i)
						undeclared = append(undeclared, findUndeclaredFields(elemSchema, nested, elemPath)...)
					}
				}
			}
		}
	}
	return undeclared
}

// getDeclaredFieldNames returns the set of top-level field names declared in
// the CUE schema (including optional fields).
func getDeclaredFieldNames(schema cue.Value) map[string]bool {
	declared := make(map[string]bool)
	it, err := schema.Fields(cue.Optional(true), cue.Definitions(false), cue.Hidden(false))
	if err != nil {
		return declared
	}
	for it.Next() {
		declared[cueutils.GetSelectorLabel(it.Selector())] = true
	}
	return declared
}

// cueParamBlock marshals the Params map into a `parameter:` block suitable
// for inclusion in a CUE document.
func cueParamBlock(params map[string]any) (string, error) {
	if len(params) == 0 {
		return velaprocess.ParameterFieldName + ": {}", nil
	}
	b, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: %s", velaprocess.ParameterFieldName, string(b)), nil
}

// enforceRequiredParams checks that every required field declared in the
// template’s `parameter:` stanza is satisfied either directly (Params) or
// indirectly (workflow‑step inputs). It returns an error describing any
// missing keys.
func enforceRequiredParams(root cue.Value, params map[string]any, app *Appfile) error {
	requiredParams, err := requiredFields(root.LookupPath(value.FieldPath(velaprocess.ParameterFieldName)))
	if err != nil {
		return err
	}

	// filter out params that are initialized directly
	requiredParams, err = filterMissing(requiredParams, params)
	if err != nil {
		return err
	}

	// if there are still required params not initialized
	if len(requiredParams) > 0 {
		// collect params that are initialized in workflow steps
		wfInitParams := make(map[string]bool)
		for _, step := range app.WorkflowSteps {
			for _, in := range step.Inputs {
				wfInitParams[in.ParameterKey] = true
			}
		}

		for _, p := range app.Policies {
			if p.Type != "override" {
				continue
			}

			var spec overrideSpec
			if err := json.Unmarshal(p.Properties.Raw, &spec); err != nil {
				return fmt.Errorf("override policy %q: parse properties: %w", p.Name, err)
			}

			for _, c := range spec.Components {
				if len(c.Properties) == 0 {
					continue
				}

				flat, err := flatten.Flatten(c.Properties, "", flatten.DotStyle)
				if err != nil {
					return fmt.Errorf("override policy %q: flatten properties: %w", p.Name, err)
				}

				for k := range flat {
					wfInitParams[k] = true // idempotent set-style insert
				}
			}
		}

		// collect required params that were not initialized even in workflow steps
		var missingParams []string
		for _, key := range requiredParams {
			if !wfInitParams[key] {
				missingParams = append(missingParams, key)
			}
		}

		if len(missingParams) > 0 {
			return fmt.Errorf("missing parameters: %v", strings.Join(missingParams, ","))
		}
	}
	return nil
}

type overrideSpec struct {
	Components []struct {
		Properties map[string]any `json:"properties"`
	} `json:"components"`
}

// requiredFields returns the list of "parameter" fields that must be supplied
// by the caller.  Nested struct leaves are returned as dot-separated paths.
//
// Rules:
//   - A field with a trailing '?' is optional -> ignore
//   - A field that has a default (*value | …) is optional -> ignore
//   - Everything else is required.
//   - Traverses arbitrarily deep into structs.
func requiredFields(v cue.Value) ([]string, error) {
	var out []string
	err := collect("", v, &out)
	return out, err
}

func collect(prefix string, v cue.Value, out *[]string) error {
	// Only structs can contain nested required fields.
	if v.Kind() != cue.StructKind {
		return nil
	}
	it, err := v.Fields(
		cue.Optional(false),
		cue.Definitions(false),
		cue.Hidden(false),
	)
	if err != nil {
		return err
	}

	for it.Next() {
		// Skip fields that provide a default (*").
		if _, hasDef := it.Value().Default(); hasDef {
			continue
		}

		label := it.Selector().Unquoted()
		path := label
		if prefix != "" {
			path = prefix + "." + label
		}

		// Recurse if the value itself is a struct; otherwise record the leaf.
		if it.Value().Kind() == cue.StructKind {
			if err := collect(path, it.Value(), out); err != nil {
				return err
			}
		} else {
			*out = append(*out, path)
		}
	}
	return nil
}

// filterMissing removes every key that is already present in the provided map.
//
// It re‑uses the original slice’s backing array to avoid allocations.
func filterMissing(keys []string, provided map[string]any) ([]string, error) {
	flattenProvided, err := flatten.Flatten(provided, "", flatten.DotStyle)
	if err != nil {
		return nil, err
	}
	out := keys[:0]
	for _, k := range keys {
		if _, ok := flattenProvided[k]; !ok {
			out = append(out, k)
		}
	}
	return out, nil
}

// renderTemplate appends the placeholders expected by KubeVela’s template
// compiler so that the generated snippet is always syntactically complete.
func renderTemplate(tmpl string) string {
	return tmpl + `
context: _
parameter: _
`
}

func newValidationProcessContext(c *Component, ctxData velaprocess.ContextData) (process.Context, error) {
	baseHooks := []process.BaseHook{
		// add more hook funcs here to validate CUE base
	}
	auxiliaryHooks := []process.AuxiliaryHook{
		// add more hook funcs here to validate CUE auxiliaries
		validateAuxiliaryNameUnique(),
	}

	ctxData.BaseHooks = baseHooks
	ctxData.AuxiliaryHooks = auxiliaryHooks
	pCtx := velaprocess.NewContext(ctxData)
	if err := c.EvalContext(pCtx); err != nil {
		return nil, errors.Wrapf(err, "evaluate base template app=%s in namespace=%s", ctxData.AppName, ctxData.Namespace)
	}
	return pCtx, nil
}

// validateAuxiliaryNameUnique validates the name of each outputs item which is
// called auxiliary in vela CUE-based DSL.
// Each capability definition can have arbitrary number of outputs and each
// outputs can have more than one auxiliaries.
// Auxiliaries can be referenced by other cap's template to pass data
// within a workload, so their names must be unique.
func validateAuxiliaryNameUnique() process.AuxiliaryHook {
	return process.AuxiliaryHookFn(func(c process.Context, a []process.Auxiliary) error {
		_, existingAuxs := c.Output()
		for _, newAux := range a {
			for _, existingAux := range existingAuxs {
				if existingAux.Name == newAux.Name {
					return errors.Wrap(fmt.Errorf("auxiliary %q already exits", newAux.Name),
						"outputs item name must be unique")
				}
			}
		}
		return nil
	})
}

// getWorkflowAndPolicySuppliedParams returns a set of parameter keys that will be
// supplied by workflow steps or override policies at runtime.
func getWorkflowAndPolicySuppliedParams(app *Appfile) map[string]bool {
	result := make(map[string]bool)

	// Collect from workflow step inputs
	for _, step := range app.WorkflowSteps {
		for _, in := range step.Inputs {
			result[in.ParameterKey] = true
		}
	}

	// Collect from override policies
	for _, p := range app.Policies {
		if p.Type != "override" {
			continue
		}

		var spec overrideSpec
		if err := json.Unmarshal(p.Properties.Raw, &spec); err != nil {
			continue // Skip if we can't parse
		}

		for _, c := range spec.Components {
			if len(c.Properties) == 0 {
				continue
			}

			flat, err := flatten.Flatten(c.Properties, "", flatten.DotStyle)
			if err != nil {
				continue // Skip if we can't flatten
			}

			for k := range flat {
				result[k] = true
			}
		}
	}

	return result
}

// getDefaultForMissingParameter checks if a parameter can be defaulted for validation
// and returns an appropriate placeholder value.
func getDefaultForMissingParameter(v cue.Value) (bool, any) {
	if v.IsConcrete() {
		return true, nil
	}

	if defaultVal, hasDefault := v.Default(); hasDefault {
		return true, defaultVal
	}

	// Use Expr() to inspect the operation tree for complex validation
	op, args := v.Expr()

	switch op {
	case cue.NoOp, cue.SelectorOp:
		// No operation or field selector - simple type
		// Use IncompleteKind for non-concrete values to get the correct type
		return true, getTypeDefault(v.IncompleteKind())

	case cue.AndOp:
		// Conjunction (e.g., int & >0 & <100)
		if len(args) > 1 {
			// Check if any arg is NOT just a basic kind (indicates complex validation)
			for _, arg := range args {
				if arg.Kind() == cue.BottomKind {
					return false, nil
				}
			}
		}
		return true, getTypeDefault(v.IncompleteKind())

	case cue.OrOp:
		// Disjunction (e.g., "value1" | "value2" | "value3") - likely an enum
		if len(args) > 0 {
			firstVal := args[0]
			if firstVal.IsConcrete() {
				var result any
				if err := firstVal.Decode(&result); err == nil {
					return true, result
				}
			}
		}
		return false, nil

	default:
		return false, nil
	}
}

// getTypeDefault returns a simple default value based on the CUE Kind.
func getTypeDefault(kind cue.Kind) any {
	switch kind {
	case cue.StringKind:
		return "__workflow_supplied__"
	case cue.FloatKind:
		return 0.0
	case cue.IntKind, cue.NumberKind:
		return 0
	case cue.BoolKind:
		return false
	case cue.ListKind:
		return []any{}
	case cue.StructKind:
		return map[string]any{}
	default:
		return "__workflow_supplied__"
	}
}

// augmentComponentParamsForValidation checks if workflow-supplied parameters
// need to be augmented for trait validation. Returns (shouldSkip, augmentedParams).
// If shouldSkip=true, the component has complex validation and should skip trait validation.
// If shouldSkip=false, augmentedParams contains the original params plus simple defaults.
func (p *Parser) augmentComponentParamsForValidation(wl *Component, workflowParams map[string]bool, ctxData velaprocess.ContextData) (bool, map[string]any) {
	// Build CUE value to inspect the component's parameter schema
	ctx := velaprocess.NewContext(ctxData)
	baseCtx, err := ctx.BaseContextFile()
	if err != nil {
		return false, wl.Params // Can't inspect, proceed normally
	}

	paramSnippet, err := cueParamBlock(wl.Params)
	if err != nil {
		return false, wl.Params
	}

	cueSrc := strings.Join([]string{
		renderTemplate(wl.FullTemplate.TemplateStr),
		paramSnippet,
		baseCtx,
	}, "\n")

	val, err := cuex.DefaultCompiler.Get().CompileString(ctx.GetCtx(), cueSrc)
	if err != nil {
		return false, wl.Params // Can't compile, proceed normally
	}

	// Get the parameter schema
	paramVal := val.LookupPath(value.FieldPath(velaprocess.ParameterFieldName))

	// Collect default values for workflow-supplied params that are missing
	workflowParamDefaults := make(map[string]any)

	for paramKey := range workflowParams {
		// Skip if already provided
		if _, exists := wl.Params[paramKey]; exists {
			continue
		}

		// Check the field in the schema
		fieldVal := paramVal.LookupPath(cue.ParsePath(paramKey))
		if !fieldVal.Exists() {
			continue // Not a parameter field
		}

		canDefault, defaultVal := getDefaultForMissingParameter(fieldVal)
		if !canDefault {
			// complex validation - skip
			return true, nil
		}

		if defaultVal != nil {
			workflowParamDefaults[paramKey] = defaultVal
		}
	}

	if len(workflowParamDefaults) == 0 {
		return false, wl.Params
	}

	// Create augmented params map
	augmented := make(map[string]any)
	for k, v := range wl.Params {
		augmented[k] = v
	}
	for k, v := range workflowParamDefaults {
		augmented[k] = v
	}

	fmt.Printf("INFO: Augmented component %q with workflow-supplied defaults for trait validation: %v\n",
		wl.Name, getMapKeys(workflowParamDefaults))

	return false, augmented
}

// getMapKeys returns the keys from a map as a slice
func getMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
