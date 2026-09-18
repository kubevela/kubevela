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
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"github.com/google/cel-go/cel"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/sources"
)

// validateExpressions checks the $(...) expressions in an Application's
// properties before it is admitted.
//
// Only what is decidable without values: the expression parses, uses no
// construct whose result type would depend on data admission has not seen, and
// reaches no identifier this surface does not offer. A mistake then surfaces
// against the property that contains it, rather than as a CUE error during a
// render that has already been accepted.
//
// Roots vary by surface. An Application-scoped policy renders before the appfile
// exists, so it has no sources to read - permitting context there and refusing
// source is what lets the surface carry expressions at all.
// appScoped reports whether a policy type is an Application-scoped
// PolicyDefinition. Supplied by the caller because that needs a client lookup,
// and this pass is otherwise a pure function of the Application.
func validateExpressions(app *v1beta1.Application, appScoped func(string) bool) field.ErrorList {
	var errs field.ErrorList

	check := func(raw *runtime.RawExtension, path *field.Path, roots ...string) {
		if raw == nil || len(raw.Raw) == 0 {
			return
		}
		var decoded interface{}
		if err := json.Unmarshal(raw.Raw, &decoded); err != nil {
			// Malformed properties are reported by the consumer's own parsing.
			return
		}
		if !propexpr.HasExpression(decoded) {
			return
		}
		if err := validateExpressionTree(decoded, roots...); err != nil {
			errs = append(errs, field.Invalid(path, string(raw.Raw), err.Error()))
		}
	}

	both := []string{propexpr.SourceIdent, propexpr.ContextIdent}
	contextOnly := []string{propexpr.ContextIdent}

	for i, comp := range app.Spec.Components {
		p := field.NewPath("spec", "components").Index(i)
		check(comp.Properties, p.Child("properties"), both...)
		for j, tr := range comp.Traits {
			check(tr.Properties, p.Child("traits").Index(j).Child("properties"), both...)
		}
	}
	for i, src := range app.Spec.Sources {
		check(src.Properties, field.NewPath("spec", "sources").Index(i).Child("properties"), both...)
	}
	for i, policy := range app.Spec.Policies {
		// A policy with a CUE template renders through the same engine a component
		// does, so a source resolves there; a built-in one has no render at all.
		roots := contextOnly
		if sources.SurfaceReadsSource(appfile.PolicySurface(policy.Type, appScoped(policy.Type))) {
			roots = both
		}
		check(policy.Properties, field.NewPath("spec", "policies").Index(i).Child("properties"), roots...)
	}
	if app.Spec.Workflow != nil {
		for i, step := range app.Spec.Workflow.Steps {
			p := field.NewPath("spec", "workflow", "steps").Index(i)
			check(step.Properties, p.Child("properties"), both...)
			for j, sub := range step.SubSteps {
				check(sub.Properties, p.Child("subSteps").Index(j).Child("properties"), both...)
			}
		}
	}

	return errs
}

// validateExpressionTargetTypes type-checks each property containing a $(...)
// expression against the parameter it feeds.
//
// This is the point of typing expressions at all: the result type is derived
// from the source schemas at admission, so a mismatch is refused before the
// Application exists rather than surfacing as a CUE error mid-render.
//
// Best-effort per target: where the target parameter type cannot be determined
// the check is skipped rather than guessed.
//
// Every surface is typed, not just components and traits. Leaving the others out
// meant a mismatch in a workflow step or a policy was caught only incidentally -
// by JSON unmarshalling, or by the dry-run render - with a message naming a Go
// field or a CUE path rather than the property the author wrote.
func (h *ValidatingHandler) validateExpressionTargetTypes(ctx context.Context, app *v1beta1.Application,
	sourceNameToType map[string]string, schemaValidators map[string]*sourceSchemaValidator,
	reported map[string]bool) field.ErrorList {
	var errs field.ErrorList
	targetParams := map[string]*cueStruct{}

	loadTarget := func(kind, defType string) *cueStruct {
		key := kind + "/" + defType
		if pv, ok := targetParams[key]; ok {
			return pv
		}
		pv := h.loadTargetParameter(ctx, app.Namespace, kind, defType, app.GetAnnotations())
		targetParams[key] = pv
		return pv
	}

	schemasFor := h.sourceSchemaTexts(ctx, app.GetAnnotations(), app.Namespace, sourceNameToType, schemaValidators)

	check := func(leaves []inputLeaf, param *cueStruct, targetDesc string,
		ctxSchema propexpr.ContextSchema) {
		for _, lf := range leaves {
			raw, ok := lf.literal.(string)
			if !ok || lf.path == "" {
				continue
			}
			parsed, err := propexpr.Parse(raw)
			if err != nil || !parsed.HasExpr() {
				continue
			}
			// The reference pass already faulted this property - an undeclared
			// source, or a path the schema does not have. Typing it would only
			// restate that in different words.
			if reported[lf.fieldPath.String()] {
				continue
			}

			srcKind, srcType, err := expressionValueType(raw, schemasFor, ctxSchema)
			if err != nil {
				errs = append(errs, field.Invalid(lf.fieldPath, raw, err.Error()))
				continue
			}
			if param == nil {
				continue
			}
			dstKind, declared := param.kindAt(lf.segments)
			if !declared {
				// The consuming template may accept it via an open struct; do not
				// over-report, exactly as the directive's check does not.
				continue
			}
			if !kindsCompatible(srcKind, dstKind) {
				errs = append(errs, field.Invalid(lf.fieldPath, lf.path,
					fmt.Sprintf("type mismatch: expression %s is %s but %s expects %s",
						raw, kindName(srcKind), targetDesc, kindName(dstKind))))
				continue
			}
			// The kinds agree, which for a collection means only "both lists".
			if dv, ok := param.valueAt(lf.segments); ok {
				if agree, want, got := celexpr.ElementsCompatible(srcType, dv); !agree {
					errs = append(errs, field.Invalid(lf.fieldPath, lf.path,
						fmt.Sprintf("type mismatch: expression %s is %s but %s expects %s",
							raw, got, targetDesc, want)))
					continue
				}
			}

			// The same rule the directive follows: a default is required only
			// when a value that may be absent feeds a required parameter.
			undefended, uerr := undefendedReads(raw, schemasFor)
			if uerr != nil || len(undefended) == 0 {
				continue
			}
			if param.requiredAt(lf.segments) {
				errs = append(errs, field.Invalid(lf.fieldPath, lf.path,
					fmt.Sprintf("%s may be absent and feeds required %s; %s",
						undefended[0], targetDesc, defaultHint(undefended[0].String()))))
			}
		}
	}

	for i, comp := range app.Spec.Components {
		if comp.Properties != nil && len(comp.Properties.Raw) > 0 {
			base := field.NewPath("spec", "components").Index(i).Child("properties")
			check(flattenLeafPaths(comp.Properties.Raw, base), loadTarget("component", comp.Type),
				fmt.Sprintf("component %q parameter", comp.Type), propexpr.ComponentContext)
		}
		for j, tr := range comp.Traits {
			if tr.Properties == nil || len(tr.Properties.Raw) == 0 {
				continue
			}
			base := field.NewPath("spec", "components").Index(i).Child("traits").Index(j).Child("properties")
			check(flattenLeafPaths(tr.Properties.Raw, base), loadTarget("trait", tr.Type),
				fmt.Sprintf("trait %q parameter", tr.Type), propexpr.TraitContext)
		}
	}

	// Policies read `context` only, so they are typed against that alone - typing
	// against a component's roots would accept a `source` this surface never
	// receives, which is the whole reason the roots vary by surface.
	for i, policy := range app.Spec.Policies {
		if policy.Properties == nil || len(policy.Properties.Raw) == 0 {
			continue
		}
		base := field.NewPath("spec", "policies").Index(i).Child("properties")
		// The policy's own context, not the component's: a policy is evaluated
		// against what its path supplies, and typing it against anything else is
		// how the two came to disagree. Which path depends on the kind - a
		// built-in policy is consumed off the appfile, a rendered one goes through
		// the engine and sees a render's context.
		scoped := h.policyIsAppScoped(ctx, app, policy.Type)
		// Whether this policy may read a source at all is settled by the
		// reference pass; this one only types what it read.
		check(flattenLeafPaths(policy.Properties.Raw, base), loadTarget("policy", policy.Type),
			fmt.Sprintf("policy %q parameter", policy.Type),
			appfile.PolicyContextSchema(policy.Type, scoped))
	}

	if app.Spec.Workflow != nil {
		for i, step := range app.Spec.Workflow.Steps {
			p := field.NewPath("spec", "workflow", "steps").Index(i)
			if step.Properties != nil && len(step.Properties.Raw) > 0 {
				check(flattenLeafPaths(step.Properties.Raw, p.Child("properties")),
					loadTarget("workflowstep", step.Type),
					fmt.Sprintf("workflow step %q parameter", step.Type),
					propexpr.WorkflowStepContext)
			}
			for j, sub := range step.SubSteps {
				if sub.Properties == nil || len(sub.Properties.Raw) == 0 {
					continue
				}
				check(flattenLeafPaths(sub.Properties.Raw, p.Child("subSteps").Index(j).Child("properties")),
					loadTarget("workflowstep", sub.Type),
					fmt.Sprintf("workflow step %q parameter", sub.Type),
					propexpr.WorkflowStepContext)
			}
		}
	}
	return errs
}

// hasSourceExpression reports whether a property value carries a $(...)
// expression.
func hasSourceExpression(raw string) bool {
	parsed, err := propexpr.Parse(raw)
	return err == nil && parsed.HasExpr()
}

// sourceSchemaTexts maps binding names to their SourceDefinition schema text,
// which is what sentinel typing needs.
func (h *ValidatingHandler) sourceSchemaTexts(ctx context.Context, annotations map[string]string, appNamespace string,
	sourceNameToType map[string]string, schemaValidators map[string]*sourceSchemaValidator) map[string]string {
	out := map[string]string{}
	for name, sourceType := range sourceNameToType {
		if sourceType == "" {
			continue
		}
		sv, ok := schemaValidators[sourceType]
		if !ok {
			var err error
			sv, err = h.loadSourceSchemaValidator(ctx, appNamespace, sourceType, annotations)
			if err != nil {
				continue
			}
			schemaValidators[sourceType] = sv
		}
		if sv == nil || sv.schemaExpr == "" {
			continue
		}
		out[name] = sv.schemaExpr
	}
	return out
}

// expressionKind types a property value that carries expressions.
func (h *ValidatingHandler) expressionKind(ctx context.Context, annotations map[string]string, appNamespace, raw string,
	sourceNameToType map[string]string, schemaValidators map[string]*sourceSchemaValidator) (cue.Kind, *cel.Type, error) {
	return expressionValueType(raw, h.sourceSchemaTexts(ctx, annotations, appNamespace, sourceNameToType, schemaValidators),
		propexpr.ComponentContext)
}

// undefendedExpressionReads returns the reads in a property value that could be
// absent at render and carry no default.
func (h *ValidatingHandler) undefendedExpressionReads(ctx context.Context, annotations map[string]string, appNamespace, raw string,
	sourceNameToType map[string]string, schemaValidators map[string]*sourceSchemaValidator) []propexpr.Reference {
	refs, err := undefendedReads(raw, h.sourceSchemaTexts(ctx, annotations, appNamespace, sourceNameToType, schemaValidators))
	if err != nil {
		return nil
	}
	return refs
}

// policyIsAppScoped reports whether a policy type is an Application-scoped
// PolicyDefinition, which decides both the context it is typed against and
// whether it may read a source at all.
//
// A built-in type never has a definition, so it is answered without a lookup. A
// missing or unreadable definition answers false - the same fail-open every other
// surface check uses, and the definition's own absence is reported elsewhere.
func (h *ValidatingHandler) policyIsAppScoped(ctx context.Context, app *v1beta1.Application, policyType string) bool {
	if appfile.IsBuiltinPolicyType(policyType) {
		return false
	}
	def := &v1beta1.PolicyDefinition{}
	if err := oamutil.GetCapabilityDefinition(ctx, h.Client, def, policyType, app.Annotations); err != nil {
		return false
	}
	return def.Spec.Scope != v1beta1.DefaultScope
}

// policyScopeLookup returns a memoised classifier, so one Application does not
// fetch the same PolicyDefinition once per policy.
func (h *ValidatingHandler) policyScopeLookup(ctx context.Context, app *v1beta1.Application) func(string) bool {
	seen := map[string]bool{}
	return func(policyType string) bool {
		if v, ok := seen[policyType]; ok {
			return v
		}
		v := h.policyIsAppScoped(ctx, app, policyType)
		seen[policyType] = v
		return v
	}
}

// validateExpressionTree runs the grammar check through whichever engine is
// selected.
//
// A spike switch. With CEL the grammar walk has no equivalent - the language is
// sandboxed by construction, and an undeclared identifier is a compile error - so
// the check narrows to "does every expression compile, and does it only read the
// roots this surface permits". The permissive env is enough for that: admission
// types the result separately.
func validateExpressionTree(v interface{}, roots ...string) error {
	return celexpr.ValidateTree(v, roots...)
}

// expressionValueType derives an expression's result kind through whichever
// engine is selected.
//
// With CEL the type comes from Compile()/OutputType() rather than from evaluating
// against sentinels, and it is derived against a *typed* env built from the
// consumed sources' schemas. That is what makes the check real: the permissive
// env types every source read as dyn, so a string feeding an int parameter would
// pass unnoticed.
// The CEL type is returned alongside the kind so the caller can compare element
// types, which a CUE kind cannot express - see celexpr.ElementsCompatible. It is
// nil whenever there is nothing precise to say: no expression, an interpolated
// string, or a compile failure.
// The readable roots are not a parameter: EnvForContext declares source and
// context and nothing else, so the sandbox is the environment rather than a list
// passed alongside it.
func expressionValueType(raw string, schemas map[string]string,
	ctxSchema propexpr.ContextSchema) (cue.Kind, *cel.Type, error) {
	parsed, err := propexpr.Parse(raw)
	if err != nil || !parsed.HasExpr() {
		return cue.BottomKind, nil, err
	}
	env, err := celexpr.EnvForContext(schemas, ctxSchema)
	if err != nil {
		return cue.BottomKind, nil, err
	}
	expr, whole := parsed.SoleExpr()
	if !whole {
		// Embedded in text, so the result is a string - but each fragment still
		// has to compile, and has to have a string form.
		//
		// CEL will not object to joining a struct to text the way CUE does,
		// because the join happens in the interpolation code rather than in the
		// expression: a map would simply be formatted, yielding something like
		// map[region:eu-west] in a property. Refusing it keeps the rule the CUE
		// engine enforced.
		for _, f := range parsed.Fragments {
			if !f.IsExpr() {
				continue
			}
			ft, cerr := celexpr.OutputType(env, f.Expr)
			if cerr != nil {
				return cue.BottomKind, nil, cerr
			}
			//nolint:exhaustive // only the kinds with no string form are refused; the rest interpolate
			switch celKind(ft) {
			case cue.StructKind, cue.ListKind:
				return cue.BottomKind, nil, fmt.Errorf(
					"expression $(%s) is %s and cannot be combined with text; read a field out of it",
					f.Expr, ft)
			}
		}
		return cue.StringKind, nil, nil
	}
	t, err := celexpr.OutputType(env, expr)
	if err != nil {
		return cue.BottomKind, nil, err
	}
	return celKind(t), t, nil
}

// celKind maps a CEL type onto the CUE kind the target check compares against.
//
// dyn and any become TopKind, which the caller treats as "cannot be compared".
// That is the honest answer for a read below an untyped region, and celexpr's
// CheckTarget is what refuses it against a concrete parameter.
func celKind(t *cel.Type) cue.Kind {
	switch t.String() {
	case "string":
		return cue.StringKind
	case "int", "uint":
		return cue.IntKind
	case "double":
		return cue.FloatKind
	case "bool":
		return cue.BoolKind
	case "dyn", "any":
		return cue.TopKind
	}
	// CEL spells a list type as list(T); without this it would collapse into
	// StructKind and mismatch every list-valued parameter.
	if strings.HasPrefix(t.String(), "list(") {
		return cue.ListKind
	}
	return cue.StructKind
}

// defaultHint tells the author how to defend a possibly-absent read, in the
// syntax the selected engine actually accepts.
//
// Getting this wrong is worse than saying nothing: the CUE disjunction is a parse
// error under CEL, so an author following the message would be told to write
// something the very next admission refuses.
func defaultHint(read string) string {
	return fmt.Sprintf("guard it with has(%s) ? %s : <fallback>", read, read)
}

// undefendedReads finds reads that may be absent and carry no guard.
//
// Two packages meet here and the split is deliberate: which paths an expression
// reads is a question for the expression language, while whether a path may be
// absent is schema analysis - and a source's schema is CUE regardless of what
// expressions are written in.
func undefendedReads(raw string, schemas map[string]string) ([]propexpr.Reference, error) {
	env, err := celexpr.DynEnv()
	if err != nil {
		return nil, err
	}
	parsed, err := propexpr.Parse(raw)
	if err != nil || !parsed.HasExpr() {
		return nil, err
	}
	var refs []propexpr.Reference
	for _, f := range parsed.Fragments {
		if !f.IsExpr() {
			continue
		}
		found, rerr := celexpr.References(env, f.Expr)
		if rerr != nil {
			return nil, rerr
		}
		refs = append(refs, found...)
	}
	return propexpr.UndefendedIn(refs, schemas)
}
