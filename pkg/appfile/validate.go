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
	"strings"

	"cuelang.org/go/cue"
	"github.com/jeremywohl/flatten/v2"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

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

		pCtx, err := newValidationProcessContext(wl, ctxData)
		if err != nil {
			return errors.WithMessagef(err, "cannot create the validation process context of app=%s in namespace=%s", a.Name, a.Namespace)
		}

		for _, tr := range wl.Traits {
			if tr.CapabilityCategory != types.CUECategory {
				continue
			}
			if tr.FullTemplate != nil &&
				tr.FullTemplate.TraitDefinition.Spec.Stage == v1beta1.PostDispatch {
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

	return nil
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
