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

// Package celexpr evaluates the CEL expressions that appear inside `$( )` in an
// Application's properties.
//
// CEL carries the three things property expressions need. It has a real type
// checker, so an expression's result type is known before any value exists and
// can be checked against the parameter it feeds. It is sandboxed by
// construction - no I/O, no imports, bounded evaluation. And it exposes a
// walkable AST, which is where dependency ordering and +sensitive tracking come
// from.
//
// Conditionals come for free: CEL's ternary requires both arms to unify, which
// is the soundness rule the expression language needs anyway.
package celexpr

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/google/cel-go/ext"
	apiservercel "k8s.io/apiserver/pkg/cel"

	"k8s.io/utils/lru"

	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// declTypeNamed is DeclTypeFor with an explicit type name.
//
// The name has to be unique per binding. Every source schema is compiled the same
// way, so deriving it from the CUE path gave every one of them the same name and
// they collided in the type provider: whichever registered first won, and a read
// against any other reported its fields as undefined. Only a test with two
// sources catches that - with one binding there is nothing to collide with.
func declTypeNamed(v cue.Value, name string) *apiservercel.DeclType {
	switch v.IncompleteKind() {
	case cue.StringKind:
		return apiservercel.StringType
	case cue.IntKind:
		return apiservercel.IntType
	case cue.FloatKind, cue.NumberKind:
		return apiservercel.DoubleType
	case cue.BoolKind:
		return apiservercel.BoolType
	case cue.ListKind:
		if elem := v.LookupPath(cue.MakePath(cue.AnyIndex)); elem.Exists() {
			return apiservercel.NewListType(declTypeNamed(elem, name+".item"), -1)
		}
		return apiservercel.NewListType(apiservercel.AnyType, -1)
	case cue.StructKind:
		// An open map - `[string]: string` - is a CEL map. A closed struct with
		// named fields is a CEL object. The distinction matters: a map read may
		// be absent (has() applies), an object field is declared.
		if elem := v.LookupPath(cue.MakePath(cue.AnyString)); elem.Exists() {
			return apiservercel.NewMapType(apiservercel.StringType, declTypeNamed(elem, name+".value"), -1)
		}
		fields := map[string]*apiservercel.DeclField{}
		iter, err := v.Fields(cue.Optional(true))
		if err != nil {
			return apiservercel.AnyType
		}
		for iter.Next() {
			f := iter.Selector().Unquoted()
			fields[f] = apiservercel.NewDeclField(
				f, declTypeNamed(iter.Value(), name+"."+f), !iter.IsOptional(), nil, nil)
		}
		return apiservercel.NewObjectType(name, fields)
	default:
		return apiservercel.AnyType
	}
}

// Env builds a CEL environment where `source` and `context` are typed from the
// schemas given, exactly as a surface would offer them.
//
// sources maps a binding name to its schema; ctx maps a context field to its
// type. Both become declared variables, so an undeclared read is a compile error
// rather than a runtime surprise - the same guarantee propexpr gets from its
// grammar walk, but from the type checker instead.
func Env(sources map[string]cue.Value, ctx map[string]*apiservercel.DeclType) (*cel.Env, error) {
	return env(sources, ctx)
}

// libraries is the function set every environment offers, declared once so the
// permissive and typed environments cannot disagree about what compiles.
//
// Only pure, total libraries are enabled. Strings gives the text handling that
// reshaping a value needs; Lists gives slice. Neither performs I/O nor reaches
// outside its arguments, so the environment still declares exactly `source` and
// `context` and an undeclared identifier still cannot compile.
//
// Deliberately absent: Encoders and Sets have no established use here, and
// Bindings introduces `cel.bind`, which would let an expression name intermediate
// values and grow into a small program.
func libraries() []cel.EnvOption {
	return []cel.EnvOption{
		ext.Strings(),
		ext.Lists(),
	}
}

func env(sources map[string]cue.Value, ctx map[string]*apiservercel.DeclType, extra ...cel.EnvOption) (*cel.Env, error) {
	srcFields := map[string]*apiservercel.DeclField{}
	for name, schema := range sources {
		if err := ValidBindingName(name); err != nil {
			return nil, err
		}
		srcFields[name] = apiservercel.NewDeclField(
			name, declTypeNamed(schema, "vela.source."+name), true, nil, nil)
	}
	sourceType := apiservercel.NewObjectType("vela.source", srcFields)

	ctxFields := map[string]*apiservercel.DeclField{}
	for name, t := range ctx {
		ctxFields[name] = apiservercel.NewDeclField(name, t, true, nil, nil)
	}
	contextType := apiservercel.NewObjectType("vela.context", ctxFields)

	// DeclTypeProvider is what teaches the checker about the named object types
	// reachable from these roots. Registering them with cel.Types() is not enough:
	// the provider is what resolves `vela.source` when a field is selected off it.
	provider := apiservercel.NewDeclTypeProvider(sourceType, contextType)
	opts, err := provider.EnvOptions(types.NewEmptyRegistry())
	if err != nil {
		return nil, fmt.Errorf("declaring source and context types: %w", err)
	}
	opts = append(opts,
		cel.Variable("source", sourceType.CelType()),
		cel.Variable("context", contextType.CelType()),
	)
	opts = append(opts, libraries()...)
	return cel.NewEnv(append(opts, extra...)...)
}

// bindingName is the shape a spec.sources[] entry's name must have to be
// readable in an expression.
var bindingName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidBindingName reports whether a binding can be named in an expression.
//
// A binding is read as a field selection - source.cfg.host - so its name has to
// be an identifier. A hyphen is the one that bites, because `source.cluster-info`
// parses as subtraction rather than as a name, and unlike CUE there is no bracket
// form to fall back on: `source` is an object type, not a map, so it cannot be
// indexed.
//
// This constrains only spec.sources[].name, which is a local alias the
// Application author picks. SourceDefinition names are untouched - they appear in
// spec.sources[].type and never inside an expression, so git-file and http-get
// stay as they are.
func ValidBindingName(name string) error {
	if bindingName.MatchString(name) {
		return nil
	}
	suggestion := strings.NewReplacer("-", "", ".", "", "/", "").Replace(name)
	return fmt.Errorf(
		"source binding name %q cannot be read in an expression: a binding is read as "+
			"source.%s, so the name must be a letter or underscore followed by letters, "+
			"digits or underscores; try %q",
		name, name, suggestion)
}

// OutputType compiles an expression and reports the type it produces.
//
// This is the whole point of the spike. propexpr needs sentinel values and an
// evaluation to answer this; CEL answers it from the AST, before any data exists.
func OutputType(env *cel.Env, expr string) (*cel.Type, error) {
	ast, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, iss.Err()
	}
	return ast.OutputType(), nil
}

// Eval compiles and runs an expression against real data.
func Eval(env *cel.Env, expr string, in map[string]interface{}) (interface{}, error) {
	c, err := compiledFor(env, expr)
	if err != nil {
		return nil, err
	}
	prg := c.prg
	// Normalised here rather than in each caller: several build their own input
	// maps, and a fix applied to one would miss the others.
	out, _, err := prg.Eval(normaliseInput(in))
	if err != nil {
		return nil, err
	}
	return native(out), nil
}

// EvalProperty evaluates a whole property value, interpolation included.
//
// The `$( )` splitting is not part of the expression language - propexpr.Parse
// already separates a value into text and expression fragments, and knows nothing
// about CUE. Only the contents of each fragment change, so interpolation survives
// a swap to CEL unchanged:
//
//	'https://$(source.registry.host)/health'  ->  "https://" + host + "/health"
//
// A value that is a single expression keeps its type - an int stays an int - and
// one embedded in text yields a string, because concatenating with text is what
// that means. Same rule as today.
func EvalProperty(env *cel.Env, raw string, in map[string]interface{}) (interface{}, error) {
	parsed, err := propexpr.Parse(raw)
	if err != nil {
		return nil, err
	}
	// Nothing to evaluate, but `$$(` still has to collapse: a value mixing an
	// escape with an expression already did, through the joining path below, and
	// the two halves have to agree.
	if !parsed.HasExpr() {
		return parsed.Literal(), nil
	}
	// A lone expression substitutes as its own type.
	if expr, ok := parsed.SoleExpr(); ok {
		return Eval(env, expr, in)
	}
	// Otherwise every fragment is rendered and joined, so the result is a string.
	var b strings.Builder
	for _, f := range parsed.Fragments {
		if !f.IsExpr() {
			b.WriteString(f.Text)
			continue
		}
		v, err := Eval(env, f.Expr, in)
		if err != nil {
			return nil, fmt.Errorf("evaluating %q: %w", f.Expr, err)
		}
		b.WriteString(fmt.Sprintf("%v", v))
	}
	return b.String(), nil
}

// EvalPropertyTyped is EvalProperty for an activation whose values already carry
// the types their schema declares.
//
// The `source` subtree is only width-converted; everything else, `context`
// included, is read the way EvalProperty reads it. That split is the point: a
// resolved source has a schema to consult, so guessing an int from a float64
// with no fractional part would undo the answer, while a context value comes
// from Go and needs the widths lined up either way.
func EvalPropertyTyped(env *cel.Env, raw string, in map[string]interface{}) (interface{}, error) {
	prepared := make(map[string]interface{}, len(in))
	for k, v := range in {
		if k == "source" {
			prepared[k] = normaliseWidths(v)
			continue
		}
		prepared[k] = v
	}
	return EvalProperty(env, raw, typedActivation(prepared))
}

// typedActivation marks an activation as already typed, so Eval's own
// normalisation leaves the `source` subtree alone.
func typedActivation(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		if k == "source" {
			out[k] = alreadyTyped{value: v}
			continue
		}
		out[k] = v
	}
	return out
}

// EnvForSurface builds an environment from the shared context registry.
//
// The registry stays the single declaration of what each call site offers; this
// reads it and translates into CEL, exactly as the CUE path reads it and
// translates into a sentinel scope. Two type systems, one source of truth - which
// is the property that made the registry worth building.
//
// A default is written with has(), not the `*read | fallback` disjunction:
//
//	has(source.cfg.note) ? source.cfg.note : "none"
//
// cel.OptionalTypes() would give the tidier `source.cfg.?note.orValue("none")`,
// but it is incompatible with the DeclTypeProvider this env is built on -
// "custom types not supported by provider" - so the has() form is what is
// available without replacing the type provider wholesale.
func EnvForSurface(sources map[string]cue.Value, surface string) (*cel.Env, error) {
	schema := propexpr.ContextFor(surface)
	ctx := map[string]*apiservercel.DeclType{}
	for _, name := range schema.ReadableFields() {
		v, ok := schema.FieldValue(name)
		if !ok {
			continue
		}
		ctx[name] = declTypeNamed(v, "vela.context."+name)
	}
	return env(sources, ctx)
}

// CheckTarget reports whether an expression's result can feed a parameter of the
// given type.
//
// Two things it refuses that a naive type comparison would let through.
//
// A `dyn` result is rejected against a concrete target. Reading below an untyped
// region - a schema's `blob: _` - yields dyn, and dyn is assignable to anything,
// so an unknown value would silently satisfy an int parameter. The CUE path makes
// the author assert (`& int`); this makes them convert (`int(...)`), which is the
// same obligation spelled differently. Against a dyn target it is allowed, since
// there is nothing to contradict.
//
// A type error in the expression itself is returned as-is rather than being
// reported as a mismatch, so the author sees the real cause.
func CheckTarget(env *cel.Env, expr string, target *cel.Type) error {
	out, err := OutputType(env, expr)
	if err != nil {
		return err
	}
	if target == nil || target == cel.DynType {
		return nil
	}
	if out == cel.DynType || out == cel.AnyType {
		return fmt.Errorf(
			"expression %q has no statically known type (it reads below an untyped "+
				"field) but the target expects %s; convert it explicitly, for example %s(...)",
			expr, target, target)
	}
	if out.String() != target.String() {
		return fmt.Errorf("type mismatch: expression %q is %s but the target expects %s",
			expr, out, target)
	}
	return nil
}

// native converts a CEL value into ordinary Go data.
//
// ref.Val.Value() is only shallow. A field selection returns whatever was put in,
// so `source.cfg.meta` comes back as a plain map - but anything CEL *constructs*
// is built from its own types, so `{"a": x}` yields map[ref.Val]ref.Val and
// `[x, y]` yields []ref.Val. Those cannot be marshalled into an Application's
// properties, which is where a substituted value has to end up.
//
// Map keys are rendered as strings because that is what a properties document
// requires; a non-string key would not survive JSON anyway.
func native(v ref.Val) interface{} {
	switch t := v.(type) {
	case traits.Lister:
		n, ok := t.Size().Value().(int64)
		if !ok {
			return v.Value()
		}
		out := make([]interface{}, 0, n)
		for i := int64(0); i < n; i++ {
			out = append(out, native(t.Get(types.Int(i))))
		}
		return out
	case traits.Mapper:
		out := map[string]interface{}{}
		for it := t.Iterator(); it.HasNext() == types.True; {
			k := it.Next()
			val := t.Get(k)
			out[fmt.Sprintf("%v", native(k))] = native(val)
		}
		return out
	default:
		return v.Value()
	}
}

// DynEnv is an environment for evaluation, where `source` and `context` are open
// maps rather than typed objects.
//
// Static typing is an admission-time concern: by the time a render happens the
// values exist, so nothing is gained by declaring their shapes again, and a typed
// env would need every consumed source's schema threaded through the resolver.
// Admission has already refused anything that would not type.
//
// CEL selects a map key with `.`, so `source.cfg.host` reads the same here as
// against a declared object - the expressions an author writes do not change.
//
// Built once and shared. It is immutable - nothing in this package extends it -
// and constructing it costs ~20us, which the render path was paying on every
// expression of every property of every component on every reconcile.
func DynEnv() (*cel.Env, error) {
	dynEnvOnce.Do(func() {
		dynEnvVal, dynEnvErr = cel.NewEnv(append([]cel.EnvOption{
			cel.Variable("source", cel.MapType(cel.StringType, cel.DynType)),
			cel.Variable("context", cel.MapType(cel.StringType, cel.DynType)),
		}, libraries()...)...)
	})
	return dynEnvVal, dynEnvErr
}

// EnvForContext builds a typed environment from source schemas given as CUE text
// and a surface's context schema.
//
// This is what makes the target check real. The permissive env types every source
// read as dyn, so a string flowing into an int parameter passes unnoticed; here
// each binding carries its declared shape, and the mismatch is a compile error.
//
// A schema that fails to compile is skipped rather than fatal: the binding then
// types as absent, which surfaces as "undeclared" on the read rather than as an
// error about the definition, and the definition's own validation reports the
// real cause.
func EnvForContext(schemaText map[string]string, ctxSchema propexpr.ContextSchema) (*cel.Env, error) {
	if key, ok := typedEnvKey(schemaText, ctxSchema); ok {
		if hit, found := typedEnvCache.Get(key); found {
			if cached, ok := hit.(*cel.Env); ok {
				return cached, nil
			}
		}
		built, err := buildEnvForContext(schemaText, ctxSchema)
		if err != nil {
			return nil, err
		}
		typedEnvCache.Add(key, built)
		return built, nil
	}
	return buildEnvForContext(schemaText, ctxSchema)
}

// typedEnvCacheSize bounds the distinct (surface, schema set) combinations kept.
const typedEnvCacheSize = 256

// typedEnvCache holds typed environments, which admission builds once per
// expression and which cost ~100us each to construct.
//
// A cel.Env is immutable and safe to share - nothing here extends one - and its
// contents are decided entirely by the schemas and the surface. lru.Cache locks
// internally.
var typedEnvCache = lru.New(typedEnvCacheSize)

// typedEnvKey renders the inputs as a key, and reports whether they can be one.
//
// Sorted, and with each schema's text included: two Applications naming the same
// bindings against different definitions must not share an environment, since
// that is exactly the mix-up a typed check exists to catch.
func typedEnvKey(schemaText map[string]string, ctxSchema propexpr.ContextSchema) (string, bool) {
	if ctxSchema.Surface == "" {
		return "", false
	}
	names := make([]string, 0, len(schemaText))
	for name := range schemaText {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(ctxSchema.Surface)
	for _, name := range names {
		b.WriteByte(0)
		b.WriteString(name)
		b.WriteByte(0)
		b.WriteString(schemaText[name])
	}
	return b.String(), true
}

func buildEnvForContext(schemaText map[string]string, ctxSchema propexpr.ContextSchema) (*cel.Env, error) {
	cc := cuecontext.New()
	sources := map[string]cue.Value{}
	for name, text := range schemaText {
		v := cc.CompileString("s: " + text)
		if v.Err() != nil {
			continue
		}
		s := v.LookupPath(cue.ParsePath("s"))
		if !s.Exists() {
			continue
		}
		sources[name] = s
	}

	ctx := map[string]*apiservercel.DeclType{}
	for _, name := range ctxSchema.ReadableFields() {
		fv, ok := ctxSchema.FieldValue(name)
		if !ok {
			continue
		}
		ctx[name] = declTypeNamed(fv, "vela.context."+name)
	}
	return env(sources, ctx)
}

// ComponentCtx is a convenience for tests and callers that want the component
// surface's context schema without importing the registry package directly.
func ComponentCtx() propexpr.ContextSchema { return propexpr.ComponentContext }

// ElementsCompatible reports whether a collection-valued expression can feed a
// collection-valued parameter.
//
// The target check compares CUE kinds, and a kind says only "list" - so
// list(string) feeding a `[...int]` parameter passed admission and failed at
// render, which is precisely the failure this feature exists to prevent. CEL
// carries the element type, so the comparison can be made honestly.
//
// It judges collections only, and only where both sides are concrete. A struct,
// an untyped region and a `dyn` all fail open, because that is where a legitimate
// widening is most likely and a false rejection is unfixable from the Application.
// The outer kind check has already run; this only narrows within a matching kind.
//
// Returns the target and expression types when they cannot agree, for the message.
func ElementsCompatible(src *cel.Type, dst cue.Value) (bool, string, string) {
	if src == nil || !dst.Exists() {
		return true, "", ""
	}
	want := declTypeNamed(dst, "vela.target").CelType()
	if elementsAgree(src, want) {
		return true, "", ""
	}
	return false, want.String(), src.String()
}

func elementsAgree(src, dst *cel.Type) bool {
	if src == nil || dst == nil || failsOpen(src) || failsOpen(dst) {
		return true
	}
	sp, dp := src.Parameters(), dst.Parameters()
	switch {
	case src.Kind() == types.ListKind && dst.Kind() == types.ListKind:
		if len(sp) == 1 && len(dp) == 1 {
			return elementsAgree(sp[0], dp[0]) && scalarsAgree(sp[0], dp[0])
		}
	case src.Kind() == types.MapKind && dst.Kind() == types.MapKind:
		// The key is a string on both sides by construction; only the value can
		// disagree.
		if len(sp) == 2 && len(dp) == 2 {
			return elementsAgree(sp[1], dp[1]) && scalarsAgree(sp[1], dp[1])
		}
	}
	return true
}

// scalarsAgree compares two element types once collections have been unwrapped.
func scalarsAgree(src, dst *cel.Type) bool {
	if failsOpen(src) || failsOpen(dst) {
		return true
	}
	if src.Kind() == dst.Kind() {
		return true
	}
	// CUE's number covers both, and an int is a valid double.
	num := map[types.Kind]bool{types.IntKind: true, types.UintKind: true, types.DoubleKind: true}
	return num[src.Kind()] && num[dst.Kind()]
}

// failsOpen reports the types this check refuses to judge.
func failsOpen(t *cel.Type) bool {
	//nolint:exhaustive // an allowlist of the kinds too loose to judge, not a mapping of every kind
	switch t.Kind() {
	case types.DynKind, types.AnyKind, types.StructKind, types.OpaqueKind, types.TypeParamKind:
		return true
	}
	return false
}
