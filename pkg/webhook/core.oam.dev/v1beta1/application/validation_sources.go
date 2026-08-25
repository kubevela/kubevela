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
	"strconv"
	"strings"
	"sync"

	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/sources"

	"cuelang.org/go/cue"
	"github.com/google/cel-go/cel"
	"k8s.io/apimachinery/pkg/util/validation/field"

	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"

	"github.com/oam-dev/kubevela/pkg/definition/cachekey"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/sourcedefinition"
)

// ValidateSources validates source bindings and the source reads expressions make.
func (h *ValidatingHandler) ValidateSources(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	var errs field.ErrorList

	// Nothing here runs unless expressions are enabled for this Application. The
	// same decision the render makes, from the same function, because an
	// Application admitted under one answer and rendered under the other is the
	// one outcome worse than either.
	if !sources.ExpressionsEnabledFor(app.GetAnnotations()) {
		// Declaring sources without them enabled is refused rather than ignored.
		// Ignoring would render $(source.x.y) into the workload as text, which
		// reaches the cluster looking like a value and fails much further away.
		if len(app.Spec.Sources) > 0 {
			errs = append(errs, field.Forbidden(field.NewPath("spec", "sources"),
				sourcesDisabledMessage()))
		}
		return errs
	}

	// Expression syntax and sandbox first: it needs no definition lookups, so a
	// typo is reported even when the rest of validation cannot run.
	appScoped := h.policyScopeLookup(ctx, app)
	errs = append(errs, validateExpressions(app, appScoped)...)

	sourceNameToType := map[string]string{}
	sourceNameToIndex := map[string]int{}
	for i, src := range app.Spec.Sources {
		p := field.NewPath("spec", "sources").Index(i)
		if src.Name == "" {
			errs = append(errs, field.Required(p.Child("name"), "source name is required"))
			continue
		}
		if prev, ok := sourceNameToIndex[src.Name]; ok {
			errs = append(errs, field.Invalid(p.Child("name"), src.Name, fmt.Sprintf("duplicated source name, already defined at index %d", prev)))
			continue
		}
		sourceNameToIndex[src.Name] = i
		sourceNameToType[src.Name] = src.Type
	}

	var refs []sourceReference
	for i, comp := range app.Spec.Components {
		compRefs, refErrs := collectSourceRefs(comp.Properties, field.NewPath("spec", "components").Index(i).Child("properties"), -1)
		errs = append(errs, refErrs...)
		refs = append(refs, withSurface(compRefs, sources.SurfaceComponent)...)
		for j, tr := range comp.Traits {
			trRefs, trErrs := collectSourceRefs(tr.Properties, field.NewPath("spec", "components").Index(i).Child("traits").Index(j).Child("properties"), -1)
			errs = append(errs, trErrs...)
			refs = append(refs, withSurface(trRefs, sources.SurfaceTrait)...)
		}
	}
	for i, policy := range app.Spec.Policies {
		policyRefs, policyErrs := collectSourceRefs(policy.Properties, field.NewPath("spec", "policies").Index(i).Child("properties"), -1)
		errs = append(errs, policyErrs...)
		refs = append(refs, withSurface(policyRefs, appfile.PolicySurface(policy.Type, appScoped(policy.Type)))...)
	}
	if app.Spec.Workflow != nil {
		for i, step := range app.Spec.Workflow.Steps {
			stepRefs, stepErrs := collectSourceRefs(step.Properties, field.NewPath("spec", "workflow", "steps").Index(i).Child("properties"), -1)
			errs = append(errs, stepErrs...)
			refs = append(refs, withSurface(stepRefs, sources.SurfaceWorkflowStep)...)
			for j, sub := range step.SubSteps {
				subRefs, subErrs := collectSourceRefs(sub.Properties, field.NewPath("spec", "workflow", "steps").Index(i).Child("subSteps").Index(j).Child("properties"), -1)
				errs = append(errs, subErrs...)
				refs = append(refs, withSurface(subRefs, sources.SurfaceWorkflowStep)...)
			}
		}
	}
	for i, src := range app.Spec.Sources {
		srcRefs, srcErrs := collectSourceRefs(src.Properties, field.NewPath("spec", "sources").Index(i).Child("properties"), i)
		errs = append(errs, srcErrs...)
		refs = append(refs, withSurface(srcRefs, sources.SurfaceSource)...)
	}

	schemaValidators := map[string]*sourceSchemaValidator{}
	consumableFromCache := map[string][]string{}
	requiredContextCache := map[string][]string{}
	// Which surfaces each binding actually resolves on, chains followed.
	bindingAt := map[int]string{}
	for name, idx := range sourceNameToIndex {
		bindingAt[idx] = name
	}
	effective := effectiveSurfaces(refs, bindingAt)
	// Field paths this pass has already faulted. The type pass below reaches the
	// same properties by a different route and would otherwise restate an
	// undeclared source or an unknown schema path in its own words.
	reported := map[string]bool{}
	fault := func(ref sourceReference, value interface{}, msg string) {
		errs = append(errs, field.Invalid(ref.FieldPath, value, msg))
		reported[ref.FieldPath.String()] = true
	}
	for _, ref := range refs {
		sourceType, ok := sourceNameToType[ref.SourceName]
		if !ok {
			fault(ref, ref.SourceName, "source is not declared in spec.sources")
			continue
		}
		if ref.SourceIndex >= 0 {
			depIdx, exists := sourceNameToIndex[ref.SourceName]
			if !exists {
				fault(ref, ref.SourceName, "source is not declared in spec.sources")
				continue
			}
			if depIdx >= ref.SourceIndex {
				fault(ref, ref.SourceName,
					fmt.Sprintf("source at index %d can only depend on prior sources, but %q is at index %d", ref.SourceIndex, ref.SourceName, depIdx))
				continue
			}
		}
		// No surface check here: validateExpressions above already restricts
		// which roots each surface offers, and reading `source` where it cannot
		// resolve is exactly what that refuses. Checking it again produced two
		// errors for one mistake.
		if sourceType == "" {
			continue
		}
		// A SourceDefinition may restrict where it can be consumed from. That
		// restriction holds wherever the value is consumed, not only in a
		// component or a trait: a workflow step or a rendered policy reading a
		// component-only source is exactly what consumableFrom refuses.
		//
		// A chained read is consumed wherever the outer binding is, so it is
		// judged against those surfaces rather than against "source".
		consumedAt := []string{ref.Surface}
		if ref.SourceIndex >= 0 {
			consumedAt = effective[ref.SourceName]
		}
		if refused, surfaces, err := h.refusedSurface(ctx, app, sourceType, consumableFromCache, consumedAt); err != nil {
			errs = append(errs, field.Invalid(ref.FieldPath, ref.Path,
				fmt.Sprintf("failed to load SourceDefinition %q: %v", sourceType, err)))
			continue
		} else if refused != "" {
			errs = append(errs, field.Invalid(ref.FieldPath, ref.Path,
				fmt.Sprintf("SourceDefinition %q declares consumableFrom %v and cannot be consumed from a %s", sourceType, surfaces, refused)))
			continue
		}

		// A source resolves in its call site's context, so it can only be consumed
		// where every field its template reads exists.
		//
		// Which surfaces that means depends on how this reference reaches the
		// source. A direct read resolves right here, at this one call site, so
		// only this surface has to satisfy it - a per-component source consumed by
		// a component is fine even when the same binding is also read from a
		// workflow step, and it is that second read alone that is wrong. A chained
		// read resolves inside whichever render triggered the outer binding, so it
		// must satisfy the outer binding's consumers - which is what
		// effectiveSurfaces works out.
		required, rerr := h.requiredContext(ctx, app.Namespace, sourceType, app.GetAnnotations(), requiredContextCache)
		if rerr == nil && len(required) > 0 {
			mustSatisfy := []string{ref.Surface}
			if ref.SourceIndex >= 0 {
				mustSatisfy = effective[ref.SourceName]
			}
			for _, surface := range mustSatisfy {
				if cerr := cachekey.CheckSurface(required, surface); cerr != nil {
					fault(ref, ref.Path, fmt.Sprintf("SourceDefinition %q %v", sourceType, cerr))
					break
				}
			}
		}
		validator, exists := schemaValidators[sourceType]
		if !exists {
			var err error
			validator, err = h.loadSourceSchemaValidator(ctx, app.Namespace, sourceType, app.GetAnnotations())
			if err != nil {
				errs = append(errs, field.Invalid(ref.FieldPath, ref.Path, fmt.Sprintf("failed to load SourceDefinition %q schema: %v", sourceType, err)))
				continue
			}
			schemaValidators[sourceType] = validator
		}
		if validator == nil {
			continue
		}
		// An empty path is a read of the binding entire, which is in contract by
		// definition: there is no field to look up, only the whole output.
		if ref.Path != "" && !ref.OpaquePath && !validator.HasPath(ref.Path) {
			fault(ref, ref.Path,
				fmt.Sprintf("path %q is not declared in schema of SourceDefinition %q", ref.Path, sourceType))
			continue
		}
		// The "optional source field consumed without a default" check is
		// target-aware (KEP: a default is required only when the optional field
		// feeds a REQUIRED target parameter). It is enforced in the target-aware
		// passes below (validateSourceInputs for source-property targets,
		// validateExpressionTargetTypes for component/trait targets), which know
		// the target parameter's optional/required marker.
	}

	// A source's properties are evaluated in the *consumer's* context, so a
	// context read there must exist on every surface that consumes the binding.
	errs = append(errs, validateSourceContextReads(app, effective)...)

	// Input contract: validate each source's properties against that
	// SourceDefinition's parameter: block (unknown fields + type compatibility).
	errs = append(errs, h.validateSourceInputs(ctx, app, sourceNameToType, schemaValidators)...)

	// Target contract: each expression's result type must be compatible with the
	// consuming component/trait parameter it is substituted into.
	errs = append(errs, h.validateExpressionTargetTypes(ctx, app, sourceNameToType, schemaValidators, reported)...)

	return errs
}

// validateSourceInputs checks that every source binding's properties conform to
// the referenced SourceDefinition's parameter: block: no undeclared fields, and
// each provided value's type is compatible with the declared parameter type.
// Values fed by an expression take their type from the referenced source's schema:
// output field, so a chained value's type is checked without resolving it.
func (h *ValidatingHandler) validateSourceInputs(ctx context.Context, app *v1beta1.Application, sourceNameToType map[string]string, schemaValidators map[string]*sourceSchemaValidator) field.ErrorList {
	var errs field.ErrorList
	paramValidators := map[string]*cueStruct{}
	for i, src := range app.Spec.Sources {
		if src.Type == "" || src.Properties == nil || len(src.Properties.Raw) == 0 {
			continue
		}
		basePath := field.NewPath("spec", "sources").Index(i).Child("properties")
		pv, cached := paramValidators[src.Type]
		if !cached {
			var err error
			pv, err = h.loadSourceParameter(ctx, app.Namespace, src.Type, app.GetAnnotations())
			if err != nil {
				errs = append(errs, field.Invalid(basePath, src.Type, fmt.Sprintf("failed to load SourceDefinition %q parameter schema: %v", src.Type, err)))
				paramValidators[src.Type] = nil
				continue
			}
			paramValidators[src.Type] = pv
		}
		if pv == nil {
			// Definition declares no parameter block; any provided property is
			// undeclared. Only flag when properties are actually supplied.
			leaves := flattenLeafPaths(src.Properties.Raw, basePath)
			for _, lf := range leaves {
				errs = append(errs, field.Invalid(lf.fieldPath, lf.path,
					fmt.Sprintf("SourceDefinition %q declares no parameters, but property %q was supplied", src.Type, lf.path)))
			}
			continue
		}
		for _, lf := range flattenLeafPaths(src.Properties.Raw, basePath) {
			errs = append(errs, h.checkInputLeaf(lf, pv, src.Type, sourceNameToType, schemaValidators, ctx, app.Namespace, app.GetAnnotations())...)
		}
	}
	return errs
}

// refusedSurface names the first surface a source is consumed from that its
// consumableFrom does not allow, or "" when every one of them is allowed.
func (h *ValidatingHandler) refusedSurface(ctx context.Context, app *v1beta1.Application, sourceType string,
	cache map[string][]string, consumedAt []string) (string, []string, error) {
	surfaces, err := h.loadConsumableFrom(ctx, app.Namespace, sourceType, cache, app.GetAnnotations())
	if err != nil {
		return "", nil, err
	}
	for _, surface := range consumedAt {
		if !sourcedefinition.SurfaceAllowed(surfaces, surface) {
			return surface, surfaces, nil
		}
	}
	return "", surfaces, nil
}

// inputLeaf is a single scalar value within a properties blob, addressed by its
// dotted path relative to the parameter block.
type inputLeaf struct {
	path      string      // dotted path into the parameter block, e.g. "region"
	segments  []string    // the same path unsplit, since a key may contain a dot
	fieldPath *field.Path // full field path for error reporting
	literal   interface{} // the value at this path
}

// flattenLeafPaths walks a properties JSON blob and returns one inputLeaf per
// scalar node, addressed by its path segments. Array elements are addressed by
// index. Returns nothing on unparseable input, which the collection pass reports.
//
// An empty object or list is a leaf too. It has nothing under it to recurse
// into, so emitting nothing would mean `{"nope": {}}` was never checked against
// the parameter block at all and an undeclared field passed.
func flattenLeafPaths(raw []byte, basePath *field.Path) []inputLeaf {
	var decoded interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	var out []inputLeaf
	emit := func(segs []string, fp *field.Path, node interface{}) {
		out = append(out, inputLeaf{
			path:      strings.Join(segs, "."),
			segments:  segs,
			fieldPath: fp,
			literal:   node,
		})
	}
	var walk func(node interface{}, segs []string, fp *field.Path)
	walk = func(node interface{}, segs []string, fp *field.Path) {
		switch v := node.(type) {
		case map[string]interface{}:
			if len(v) == 0 {
				emit(segs, fp, node)
				return
			}
			for k, child := range v {
				// A fresh slice per child: appending into segs would share the
				// backing array between siblings.
				walk(child, append(append([]string{}, segs...), k), fp.Child(k))
			}
		case []interface{}:
			if len(v) == 0 {
				emit(segs, fp, node)
				return
			}
			for idx, child := range v {
				walk(child, append(append([]string{}, segs...), strconv.Itoa(idx)), fp.Index(idx))
			}
		default:
			emit(segs, fp, node)
		}
	}
	walk(decoded, nil, basePath)
	return out
}

// jsonKind maps a decoded JSON scalar to the CUE kind it would satisfy.
func jsonKind(v interface{}) cue.Kind {
	switch n := v.(type) {
	case string:
		return cue.StringKind
	case bool:
		return cue.BoolKind
	case float64:
		// JSON numbers decode to float64; treat integral values as int-compatible.
		if n == float64(int64(n)) {
			return cue.IntKind
		}
		return cue.NumberKind
	case nil:
		return cue.NullKind
	case map[string]interface{}:
		return cue.StructKind
	case []interface{}:
		return cue.ListKind
	}
	return cue.BottomKind
}

// checkInputLeaf validates one properties leaf against the target parameter
// block: the field must be declared, and its type must be compatible with the
// declared parameter type. Expression-fed leaves take their type from the
// referenced source's schema output field.
func (h *ValidatingHandler) checkInputLeaf(lf inputLeaf, param *cueStruct, sourceType string, sourceNameToType map[string]string, schemaValidators map[string]*sourceSchemaValidator, ctx context.Context, appNamespace string, annotations map[string]string) field.ErrorList {
	var errs field.ErrorList
	if lf.path == "" {
		return errs
	}
	dstKind, declared := param.kindAt(lf.segments)
	if !declared {
		errs = append(errs, field.Invalid(lf.fieldPath, lf.path,
			fmt.Sprintf("property %q is not declared in the parameter schema of SourceDefinition %q", lf.path, sourceType)))
		return errs
	}
	// Determine the incoming value's type.
	var srcKind cue.Kind
	var srcType *cel.Type
	if raw, isString := lf.literal.(string); isString && hasSourceExpression(raw) {
		// A source's own properties may be fed by an expression - that is how
		// chaining is written without the directive. Typing it as the string it
		// literally is would reject every non-string target.
		k, kt, terr := h.expressionKind(ctx, annotations, appNamespace, raw, sourceNameToType, schemaValidators)
		if terr != nil {
			errs = append(errs, field.Invalid(lf.fieldPath, raw, terr.Error()))
			return errs
		}
		srcKind, srcType = k, kt

		// The same optional-feeds-required rule the directive follows.
		if undefended := h.undefendedExpressionReads(ctx, annotations, appNamespace, raw, sourceNameToType, schemaValidators); len(undefended) > 0 {
			if param.requiredAt(lf.segments) {
				errs = append(errs, field.Invalid(lf.fieldPath, lf.path,
					fmt.Sprintf("%s may be absent and feeds required parameter %q of SourceDefinition %q; guard it with has(%s) ? %s : <fallback>",
						undefended[0], lf.path, sourceType, undefended[0], undefended[0])))
			}
		}
	} else {
		srcKind = jsonKind(lf.literal)
	}
	if !kindsCompatible(srcKind, dstKind) {
		errs = append(errs, field.Invalid(lf.fieldPath, lf.path,
			fmt.Sprintf("type mismatch for parameter %q of SourceDefinition %q: expected %s, got %s",
				lf.path, sourceType, kindName(dstKind), kindName(srcKind))))
		return errs
	}
	// The kinds agree, which for a collection means only "both lists".
	if dv, ok := param.valueAt(lf.segments); ok {
		if agree, want, got := celexpr.ElementsCompatible(srcType, dv); !agree {
			errs = append(errs, field.Invalid(lf.fieldPath, lf.path,
				fmt.Sprintf("type mismatch for parameter %q of SourceDefinition %q: expected %s, got %s",
					lf.path, sourceType, want, got)))
		}
	}
	return errs
}

// parameterBlockSources memoises the reduction below, keyed on the template.
//
// Every admission re-parsed each definition a validated Application references,
// walked its declarations and re-formatted the result, to recover text fixed for
// the life of the definition. Measured on an ordinary component template, the
// whole of parameterBlockOnly is 104us, of which the reduction is 43us.
//
// Only the text is kept, never the compiled value. cue documents that "values
// created from the same Context are not safe for concurrent use", and admission
// requests are concurrent, so a shared cue.Value would be a data race rather
// than a saving. The compile therefore still happens per call - 56us of the
// 104us that cannot be recovered without a change in that guarantee.
//
// Keyed on the template text, so a definition that changes gets a new entry and
// there is no invalidation to get wrong.
var parameterBlockSources sync.Map // template -> parameterBlockExtract

// sourcesDisabledMessage says which of the two switches is off, because "not
// enabled" sends an author to the wrong one half the time.
func sourcesDisabledMessage() string {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableCelExpressions) {
		return "source expressions are not enabled on this cluster; " +
			"an operator enables them with the EnableCelExpressions feature gate"
	}
	return fmt.Sprintf("this Application has not opted in to source expressions; "+
		"set the %s annotation to \"true\", and check its properties for $(VAR) "+
		"environment-variable syntax, which must be written $$(VAR) once expressions are read",
		oam.AnnotationCelExpressions)
}
