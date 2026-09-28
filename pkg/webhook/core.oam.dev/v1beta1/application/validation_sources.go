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

	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/sources"

	"cuelang.org/go/cue"
	celengine "github.com/kubevela/pkg/cel"
	"github.com/kubevela/pkg/cel/template"
	"k8s.io/apimachinery/pkg/util/validation/field"

	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"

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

	// Every properties blob is planned once, and every pass below reads that plan.
	appScoped := h.policyScopeLookup(ctx, app)
	blobs := planApplication(app, appScoped)

	// Expression syntax and roots first: they need no definition lookups, so a
	// typo is reported even when the rest of validation cannot run.
	compiled := true
	for _, bp := range blobs {
		errs = append(errs, bp.syntaxAndRoots()...)
		compiled = compiled && bp.malformed == nil && len(bp.faults) == 0
	}
	// The component-read rules read every expression again, so an expression
	// that does not compile would be reported twice.
	if compiled {
		if err := appfile.ValidateComponentReads(app.Spec); err != nil {
			errs = append(errs, field.Invalid(field.NewPath("spec", "components"), "", err.Error()))
		}
	}
	errs = append(errs, h.validatePostDispatchReads(ctx, app)...)
	errs = append(errs, h.validateReadClusters(ctx, app)...)

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

	// Which surfaces each binding actually resolves on, chains followed.
	bindingAt := map[int]string{}
	for name, idx := range sourceNameToIndex {
		bindingAt[idx] = name
	}
	effective := effectiveSurfaces(sourceRefs(blobs), bindingAt)

	schemaValidators := map[string]*sourceSchemaValidator{}
	rules := &sourceRules{h: h, ctx: ctx, app: app, nameToType: sourceNameToType, nameToIndex: sourceNameToIndex,
		effective: effective, schemaValidators: schemaValidators,
		consumableFrom: map[string][]string{}, requiredContext: map[string][]string{}}
	// Field paths the source rules have settled. The type pass reaches the same
	// properties and would otherwise restate an undeclared source or an unknown
	// schema path in its own words.
	reported := map[string]bool{}
	for _, bp := range blobs {
		if bp.plan == nil {
			continue
		}
		// Which roots a surface offers was judged above.
		faults := bp.plan.Check(everyRoot, map[string]celengine.Checker{propexpr.SourceIdent: rules.checker(bp)})
		errs = append(errs, bp.fieldErrors(faults, reported)...)
	}

	// A source's properties are evaluated in the *consumer's* context, so a
	// context read there must exist on every surface that consumes the binding.
	errs = append(errs, validateSourceContextReads(blobs, effective)...)

	// Input contract: each source's properties against that SourceDefinition's
	// parameter: block (unknown fields and type compatibility).
	t := newTyping(h.sourceSchemaTexts(ctx, app.GetAnnotations(), app.Namespace, sourceNameToType, schemaValidators))
	errs = append(errs, h.validateSourceInputs(ctx, app, blobs, t, effective, reported)...)

	// Target contract: each expression's result against the parameter it feeds.
	errs = append(errs, h.validateExpressionTargetTypes(ctx, app, blobs, t, reported)...)

	return errs
}

func (h *ValidatingHandler) validateSourceInputs(ctx context.Context, app *v1beta1.Application, blobs []blobPlan,
	t *typing, effective map[string][]string, reported map[string]bool) field.ErrorList {
	var errs field.ErrorList
	paramValidators := map[string]*cueStruct{}
	for _, bp := range blobs {
		if bp.sourceIndex < 0 || bp.targetType == "" {
			continue
		}
		pv, cached := paramValidators[bp.targetType]
		if !cached {
			var err error
			pv, err = h.loadSourceParameter(ctx, app.Namespace, bp.targetType, app.GetAnnotations())
			if err != nil {
				errs = append(errs, field.Invalid(bp.base, bp.targetType, fmt.Sprintf("failed to load SourceDefinition %q parameter schema: %v", bp.targetType, err)))
				paramValidators[bp.targetType] = nil
				continue
			}
			paramValidators[bp.targetType] = pv
		}
		if pv == nil {
			// Definition declares no parameter block; any provided property is
			// undeclared. Only flag when properties are actually supplied.
			for _, lf := range bp.leaves {
				errs = append(errs, field.Invalid(lf.fieldPath, lf.path,
					fmt.Sprintf("SourceDefinition %q declares no parameters, but property %q was supplied", bp.targetType, lf.path)))
			}
			continue
		}
		// A chained value's type comes from the source it reads, without
		// resolving it, and in the context of every surface consuming the
		// binding.
		faults := map[string]celengine.CheckError{}
		var envErr error
		for _, ctxSchema := range consumerContexts(effective[bp.binding]) {
			fs, err := t.faults(bp, ctxSchema, pv)
			if err != nil {
				envErr = err
				break
			}
			for prop, f := range fs {
				if _, seen := faults[prop]; !seen {
					faults[prop] = f
				}
			}
		}
		w := sourceParameterWording(bp.targetType)
		for _, lf := range bp.leaves {
			errs = append(errs, checkInputLeaf(lf, pv, bp.targetType, faults, envErr, w, reported)...)
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
	// property is the path as the expression engine names it, "env[0].value",
	// so a leaf can be found in a plan of the same blob.
	property string
}

// flattenLeafPaths walks a properties JSON blob and returns one inputLeaf per
// scalar node, addressed by its path segments. Array elements are addressed by
// index. Returns nothing on unparseable input, which syntaxAndRoots reports.
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
	emit := func(segs []string, fp *field.Path, prop string, node interface{}) {
		out = append(out, inputLeaf{
			path:      strings.Join(segs, "."),
			segments:  segs,
			fieldPath: fp,
			literal:   node,
			property:  prop,
		})
	}
	var walk func(node interface{}, segs []string, fp *field.Path, prop string)
	walk = func(node interface{}, segs []string, fp *field.Path, prop string) {
		switch v := node.(type) {
		case map[string]interface{}:
			if len(v) == 0 {
				emit(segs, fp, prop, node)
				return
			}
			for k, child := range v {
				// A fresh slice per child: appending into segs would share the
				// backing array between siblings.
				walk(child, append(append([]string{}, segs...), k), fp.Child(k), template.JoinPath(prop, k))
			}
		case []interface{}:
			if len(v) == 0 {
				emit(segs, fp, prop, node)
				return
			}
			for idx, child := range v {
				walk(child, append(append([]string{}, segs...), strconv.Itoa(idx)), fp.Index(idx), template.IndexPath(prop, idx))
			}
		default:
			emit(segs, fp, prop, node)
		}
	}
	walk(decoded, nil, basePath, "")
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
		// JSON numbers decode to float64: an integral one is an int, and one
		// with a fraction a float.
		if n == float64(int64(n)) {
			return cue.IntKind
		}
		return cue.FloatKind
	case nil:
		return cue.NullKind
	case map[string]interface{}:
		return cue.StructKind
	case []interface{}:
		return cue.ListKind
	}
	return cue.BottomKind
}

func checkInputLeaf(lf inputLeaf, param *cueStruct, sourceType string, faults map[string]celengine.CheckError,
	envErr error, w wording, reported map[string]bool) field.ErrorList {
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
	if raw, isString := lf.literal.(string); isString && holdsExpression(raw) {
		// A source's own properties may be fed by an expression, which is how
		// chaining is written; it was typed against the parameter already. A
		// leaf the source rules refused is not restated, and one that does not
		// parse is reported by syntaxAndRoots.
		switch f, faulted := faults[lf.property]; {
		case reported[lf.fieldPath.String()]:
		case envErr != nil:
			errs = append(errs, field.Invalid(lf.fieldPath, raw, envErr.Error()))
		case faulted:
			errs = append(errs, w.fieldError(lf, f))
		}
		return errs
	}
	if srcKind := jsonKind(lf.literal); !celengine.KindFits(srcKind, dstKind) {
		errs = append(errs, field.Invalid(lf.fieldPath, lf.path,
			fmt.Sprintf("type mismatch for parameter %q of SourceDefinition %q: expected %s, got %s",
				lf.path, sourceType, kindName(dstKind), kindName(srcKind))))
	}
	return errs
}

// holdsExpression reports a property value that holds an expression, or tries
// to: one that does not parse is an expression gone wrong, not a literal.
func holdsExpression(raw string) bool {
	parsed, err := template.Parse(raw)
	return err != nil || parsed.HasExpr()
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
