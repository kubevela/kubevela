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
	"errors"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"github.com/google/cel-go/cel"
	celengine "github.com/kubevela/pkg/cel"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/definition/cachekey"
	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/sources"
)

// everyRoot permits every root, for a Check that applies one root's rules and
// leaves which roots a surface offers to the roots pass.
var everyRoot = []string{propexpr.SourceIdent, propexpr.ContextIdent, propexpr.ComponentIdent}

// blobPlan is one properties blob of an Application, planned once: where it
// sits, the surface that evaluates it, what it feeds, and its expressions.
type blobPlan struct {
	leafPlan
	base *field.Path
	raw  []byte
	// surface evaluates the blob; ctxSchema is what its context offers.
	surface   string
	ctxSchema propexpr.ContextSchema
	// sourceIndex is i for spec.sources[i], whose binding is binding; -1
	// for every other blob.
	sourceIndex int
	binding     string
	// targetKind and targetType name the definition the blob feeds, and
	// targetDesc is how a refusal names it.
	targetKind, targetType, targetDesc string
}

// planApplication plans every properties blob of an Application, in the order
// admission reports them.
func planApplication(app *v1beta1.Application, appScoped func(string) bool) []blobPlan {
	var out []blobPlan
	add := func(raw *runtime.RawExtension, bp blobPlan) {
		if raw == nil || len(raw.Raw) == 0 {
			return
		}
		bp.leafPlan = planLeaves(raw.Raw, bp.base)
		bp.raw = raw.Raw
		out = append(out, bp)
	}
	for i, comp := range app.Spec.Components {
		p := field.NewPath("spec", "components").Index(i)
		add(comp.Properties, blobPlan{base: p.Child("properties"), surface: sources.SurfaceComponent,
			ctxSchema: propexpr.ComponentContext, sourceIndex: -1, targetKind: "component", targetType: comp.Type,
			targetDesc: fmt.Sprintf("component %q parameter", comp.Type)})
		for j, tr := range comp.Traits {
			add(tr.Properties, blobPlan{base: p.Child("traits").Index(j).Child("properties"), surface: sources.SurfaceTrait,
				ctxSchema: propexpr.TraitContext, sourceIndex: -1, targetKind: "trait", targetType: tr.Type,
				targetDesc: fmt.Sprintf("trait %q parameter", tr.Type)})
		}
	}
	for i, policy := range app.Spec.Policies {
		// A policy is typed against the context its own path supplies: a
		// built-in one is consumed off the appfile, a rendered one sees a
		// render's context.
		scoped := appScoped(policy.Type)
		add(policy.Properties, blobPlan{base: field.NewPath("spec", "policies").Index(i).Child("properties"),
			surface: appfile.PolicySurface(policy.Type, scoped), ctxSchema: appfile.PolicyContextSchema(policy.Type, scoped),
			sourceIndex: -1, targetKind: "policy", targetType: policy.Type,
			targetDesc: fmt.Sprintf("policy %q parameter", policy.Type)})
	}
	if app.Spec.Workflow != nil {
		for i, step := range app.Spec.Workflow.Steps {
			p := field.NewPath("spec", "workflow", "steps").Index(i)
			add(step.Properties, blobPlan{base: p.Child("properties"), surface: sources.SurfaceWorkflowStep,
				ctxSchema: propexpr.WorkflowStepContext, sourceIndex: -1, targetKind: "workflowstep", targetType: step.Type,
				targetDesc: fmt.Sprintf("workflow step %q parameter", step.Type)})
			for j, sub := range step.SubSteps {
				add(sub.Properties, blobPlan{base: p.Child("subSteps").Index(j).Child("properties"),
					surface: sources.SurfaceWorkflowStep, ctxSchema: propexpr.WorkflowStepContext, sourceIndex: -1,
					targetKind: "workflowstep", targetType: sub.Type,
					targetDesc: fmt.Sprintf("workflow step %q parameter", sub.Type)})
			}
		}
	}
	for i, src := range app.Spec.Sources {
		add(src.Properties, blobPlan{base: field.NewPath("spec", "sources").Index(i).Child("properties"),
			surface: sources.SurfaceSource, ctxSchema: propexpr.ComponentContext, sourceIndex: i, binding: src.Name,
			targetKind: "source", targetType: src.Type})
	}
	return out
}

// refusal is an admission refusal carried through a CheckError, with the value
// field.Invalid shows beside it. A settled refusal is not restated by the type
// pass.
type refusal struct {
	value   interface{}
	msg     string
	settled bool
}

func (r *refusal) Error() string { return r.msg }

// fieldErrors turns a plan's faults into admission errors at their leaves,
// recording settled refusals in reported when it is given.
func (bp blobPlan) fieldErrors(faults []celengine.CheckError, reported map[string]bool) field.ErrorList {
	var errs field.ErrorList
	for _, f := range faults {
		fp := bp.fieldPath(f.Property, bp.base)
		var r *refusal
		if errors.As(f.Err, &r) {
			errs = append(errs, field.Invalid(fp, r.value, r.msg))
			if r.settled && reported != nil {
				reported[fp.String()] = true
			}
			continue
		}
		errs = append(errs, field.Invalid(fp, string(bp.raw), f.Err.Error()))
	}
	return errs
}

// syntaxAndRoots reports every expression that does not compile, and every read
// of a root the blob's surface does not offer. It needs no definition lookups,
// so a typo is reported even when the rest of admission cannot run.
func (bp blobPlan) syntaxAndRoots() field.ErrorList {
	if bp.malformed != nil {
		return field.ErrorList{field.Invalid(bp.base, string(bp.raw), fmt.Sprintf("invalid properties: %v", bp.malformed))}
	}
	errs := bp.fieldErrors(bp.faults, nil)
	if bp.plan != nil {
		errs = append(errs, bp.fieldErrors(bp.plan.Check(sources.RootsFor(bp.surface), nil), nil)...)
	}
	return errs
}

// refFor is a source read as the reference records effectiveSurfaces works on.
func (bp blobPlan) refFor(read celengine.Read) sourceReference {
	return sourceReference{
		SourceName: read.Path[0],
		Path:       strings.Join(read.Path[1:], "."),
		// Whether the dotted form round-trips has to be decided while the
		// segments are separate: labels["a.b/c"] joins to labels.a.b/c, and a
		// list index joins to a segment the schema has no field for.
		OpaquePath:  pathIsOpaque(read.Path[1:]),
		FieldPath:   bp.fieldPath(read.Property, bp.base),
		SourceIndex: bp.sourceIndex,
		Surface:     bp.surface,
	}
}

// sourceRefs is every source read in the Application. A whole-binding read,
// $(source.cfg), still has to be declared, come earlier in a chain, and allow
// the surface reading it.
func sourceRefs(blobs []blobPlan) []sourceReference {
	var refs []sourceReference
	for _, bp := range blobs {
		if bp.plan == nil {
			continue
		}
		for _, read := range bp.plan.Reads(propexpr.SourceIdent) {
			if len(read.Path) > 0 {
				refs = append(refs, bp.refFor(read))
			}
		}
	}
	return refs
}

// sourceRules is what judging a source read needs from the Application and its
// SourceDefinitions, memoised across every blob.
type sourceRules struct {
	h                *ValidatingHandler
	ctx              context.Context
	app              *v1beta1.Application
	nameToType       map[string]string
	nameToIndex      map[string]int
	effective        map[string][]string
	schemaValidators map[string]*sourceSchemaValidator
	consumableFrom   map[string][]string
	requiredContext  map[string][]string
}

// checker judges a blob's source reads: the binding is declared, a chained read
// names an earlier binding, the SourceDefinition allows the surfaces consuming
// it and can resolve in their context, and the path is in its schema.
func (s *sourceRules) checker(bp blobPlan) celengine.Checker {
	return celengine.CheckerFunc(func(reads []celengine.Read) []celengine.CheckError {
		var out []celengine.CheckError
		refuse := func(read celengine.Read, value interface{}, msg string, settled bool) {
			out = append(out, celengine.CheckError{Property: read.Property, Read: read,
				Err: &refusal{value: value, msg: msg, settled: settled}})
		}
		for _, read := range reads {
			if len(read.Path) == 0 {
				continue
			}
			ref := bp.refFor(read)
			sourceType, ok := s.nameToType[ref.SourceName]
			if !ok {
				refuse(read, ref.SourceName, "source is not declared in spec.sources", true)
				continue
			}
			if ref.SourceIndex >= 0 {
				if depIdx := s.nameToIndex[ref.SourceName]; depIdx >= ref.SourceIndex {
					refuse(read, ref.SourceName, fmt.Sprintf("source at index %d can only depend on prior sources, but %q is at index %d",
						ref.SourceIndex, ref.SourceName, depIdx), true)
					continue
				}
			}
			if sourceType == "" {
				continue
			}
			// A chained read is consumed wherever the outer binding is, so it is
			// judged against those surfaces rather than against "source".
			consumedAt := []string{ref.Surface}
			if ref.SourceIndex >= 0 {
				consumedAt = s.effective[ref.SourceName]
			}
			refused, surfaces, err := s.h.refusedSurface(s.ctx, s.app, sourceType, s.consumableFrom, consumedAt)
			if err != nil {
				refuse(read, ref.Path, fmt.Sprintf("failed to load SourceDefinition %q: %v", sourceType, err), false)
				continue
			}
			if refused != "" {
				refuse(read, ref.Path, fmt.Sprintf("SourceDefinition %q declares consumableFrom %v and cannot be consumed from a %s",
					sourceType, surfaces, refused), false)
				continue
			}
			// A direct read resolves at this call site; a chained one inside
			// whichever render triggered the outer binding, so it must satisfy
			// the outer binding's consumers.
			required, rerr := s.h.requiredContext(s.ctx, s.app.Namespace, sourceType, s.app.GetAnnotations(), s.requiredContext)
			if rerr == nil && len(required) > 0 {
				for _, surface := range consumedAt {
					if cerr := cachekey.CheckSurface(required, surface); cerr != nil {
						refuse(read, ref.Path, fmt.Sprintf("SourceDefinition %q %v", sourceType, cerr), true)
						break
					}
				}
			}
			validator, exists := s.schemaValidators[sourceType]
			if !exists {
				validator, err = s.h.loadSourceSchemaValidator(s.ctx, s.app.Namespace, sourceType, s.app.GetAnnotations())
				if err != nil {
					refuse(read, ref.Path, fmt.Sprintf("failed to load SourceDefinition %q schema: %v", sourceType, err), false)
					continue
				}
				s.schemaValidators[sourceType] = validator
			}
			// An empty path reads the binding entire, which is in contract by
			// definition.
			if validator != nil && ref.Path != "" && !ref.OpaquePath && !validator.HasPath(ref.Path) {
				refuse(read, ref.Path, fmt.Sprintf("path %q is not declared in schema of SourceDefinition %q", ref.Path, sourceType), true)
			}
		}
		return out
	})
}

// contextChecker judges the context read in a source's properties: those are
// evaluated in the consumer's context, so a field must exist on every surface
// that consumes the binding.
func contextChecker(bp blobPlan, effective map[string][]string) celengine.Checker {
	return celengine.CheckerFunc(func(reads []celengine.Read) []celengine.CheckError {
		var out []celengine.CheckError
		for _, read := range reads {
			if len(read.Path) == 0 {
				continue
			}
			for _, surface := range effective[bp.binding] {
				if propexpr.ContextFor(surface).Offers(read.Path[0]) {
					continue
				}
				out = append(out, celengine.CheckError{Property: read.Property, Read: read, Err: &refusal{
					value:   bp.leafText(read.Property),
					msg:     contextUnavailableMessage(read.Path[0], surface, bp.binding),
					settled: true,
				}})
			}
		}
		return out
	})
}

// target is a definition's parameter block as the engine's Target, found by the
// leaf the property names.
func (bp blobPlan) target(param *cueStruct) celengine.Target {
	return celengine.TargetFunc(func(property string) (cue.Value, bool, bool) {
		lf, ok := bp.leaf(property)
		if !ok {
			return cue.Value{}, false, false
		}
		v, ok := param.valueAt(lf.segments)
		if !ok {
			return cue.Value{}, false, false
		}
		return v, param.requiredAt(lf.segments), true
	})
}

// typing is what typing expressions needs, built once per admission: the
// binding schemas, the judges over them, and a typed environment per context.
type typing struct {
	schemas map[string]string
	opts    celengine.TypeOptions
	envs    map[string]*cel.Env
}

func newTyping(schemas map[string]string) *typing {
	compiled := propexpr.CompileSchemas(schemas)
	return &typing{schemas: schemas, envs: map[string]*cel.Env{}, opts: celengine.TypeOptions{
		// The has()-default rule: a read that may be missing, unguarded, is
		// refused where it feeds a required parameter.
		Optional: func(r celengine.Read) bool {
			may, err := compiled.CanBeAbsent(r.Reference)
			return err == nil && may
		},
		// A schema's number may hold an integer, which CEL's double loses.
		DeclaredKind: func(r celengine.Read) (cue.Kind, bool) { return compiled.Kind(r.Reference) },
	}}
}

func (t *typing) env(ctxSchema propexpr.ContextSchema) (*cel.Env, error) {
	if env, ok := t.envs[ctxSchema.Surface]; ok {
		return env, nil
	}
	env, err := celexpr.EnvForContext(t.schemas, ctxSchema)
	if err == nil && ctxSchema.Surface != "" {
		t.envs[ctxSchema.Surface] = env
	}
	return env, err
}

// faults types a blob's expressions, read in ctxSchema, against param, keyed by
// the leaf property each fault is at.
func (t *typing) faults(bp blobPlan, ctxSchema propexpr.ContextSchema, param *cueStruct) (map[string]celengine.CheckError, error) {
	if bp.plan == nil {
		return nil, nil
	}
	env, err := t.env(ctxSchema)
	if err != nil {
		return nil, err
	}
	var target celengine.Target
	if param != nil {
		target = bp.target(param)
	}
	out := map[string]celengine.CheckError{}
	for _, f := range bp.plan.CheckTypes(env, target, t.opts) {
		out[f.Property] = f
	}
	return out, nil
}

// wording is how a refusal names the parameter a value feeds.
type wording struct {
	mismatch func(lf inputLeaf, m *celengine.TypeMismatch) string
	absent   func(lf inputLeaf, read string) string
}

// fieldError words a type fault at its leaf.
func (w wording) fieldError(lf inputLeaf, f celengine.CheckError) *field.Error {
	var mismatch *celengine.TypeMismatch
	var absent *celengine.MayBeAbsent
	switch {
	case errors.As(f.Err, &mismatch):
		return field.Invalid(lf.fieldPath, lf.path, w.mismatch(lf, mismatch))
	case errors.As(f.Err, &absent):
		return field.Invalid(lf.fieldPath, lf.path, w.absent(lf, absent.Read.String()))
	default:
		return field.Invalid(lf.fieldPath, lf.literal, f.Err.Error())
	}
}

// parameterWording names a component, trait, policy or workflow step parameter.
func parameterWording(desc string) wording {
	return wording{
		mismatch: func(lf inputLeaf, m *celengine.TypeMismatch) string {
			return fmt.Sprintf("type mismatch: expression %v is %s but %s expects %s", lf.literal, m.Got, desc, m.Want)
		},
		absent: func(_ inputLeaf, read string) string {
			return fmt.Sprintf("%s may be absent and feeds required %s; %s", read, desc, defaultHint(read))
		},
	}
}

// sourceParameterWording names a SourceDefinition's parameter.
func sourceParameterWording(sourceType string) wording {
	return wording{
		mismatch: func(lf inputLeaf, m *celengine.TypeMismatch) string {
			return fmt.Sprintf("type mismatch for parameter %q of SourceDefinition %q: expected %s, got %s",
				lf.path, sourceType, m.Want, m.Got)
		},
		absent: func(lf inputLeaf, read string) string {
			return fmt.Sprintf("%s may be absent and feeds required parameter %q of SourceDefinition %q; %s",
				read, lf.path, sourceType, defaultHint(read))
		},
	}
}

// consumerContexts is the context of each surface consuming a binding: a
// source's properties are evaluated in its consumer's context. A binding nothing
// consumes resolves nowhere, and is typed as a component would read it.
func consumerContexts(surfaces []string) []propexpr.ContextSchema {
	if len(surfaces) == 0 {
		return []propexpr.ContextSchema{propexpr.ComponentContext}
	}
	out := make([]propexpr.ContextSchema, 0, len(surfaces))
	for _, s := range surfaces {
		out = append(out, propexpr.ContextFor(s))
	}
	return out
}
