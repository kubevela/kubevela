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

// Package celexpr binds KubeVela's expression roots to the engine in
// github.com/kubevela/pkg/cel: it declares source, context and component, and
// types source and context from binding schemas and the surface registry.
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
	celengine "github.com/kubevela/pkg/cel"
	apiservercel "k8s.io/apiserver/pkg/cel"

	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// Vela is the engine every KubeVela property expression is compiled and
// evaluated with: the source, context and component roots, and the placement
// calls a component read may make.
var Vela = mustEngine()

func mustEngine() *celengine.Engine {
	// Resolved in this order: component and context are answered from memory,
	// and a component read that must wait ends the render before any source is
	// fetched.
	e, err := celengine.NewEngine(
		celengine.Root{Name: propexpr.ComponentIdent, QualifiedAt: 1, Qualifiers: []celengine.Qualifier{
			{Name: propexpr.PlaceCluster},
			{Name: propexpr.PlaceNamespace},
		}},
		celengine.Root{Name: propexpr.ContextIdent},
		celengine.Root{Name: propexpr.SourceIdent},
	)
	if err != nil {
		panic(err)
	}
	return e
}

// rootDecls types source from its bindings' schemas and context from its fields.
// component stays dyn: its values are a live object's status, which has no
// schema at admission.
func rootDecls(sources map[string]cue.Value, ctx map[string]*apiservercel.DeclType) (map[string]*apiservercel.DeclType, error) {
	srcFields := map[string]*apiservercel.DeclField{}
	for name, schema := range sources {
		if err := ValidBindingName(name); err != nil {
			return nil, err
		}
		srcFields[name] = apiservercel.NewDeclField(
			name, celengine.DeclType(schema, "vela.source."+name), true, nil, nil)
	}
	ctxFields := map[string]*apiservercel.DeclField{}
	for name, t := range ctx {
		ctxFields[name] = apiservercel.NewDeclField(name, t, true, nil, nil)
	}
	return map[string]*apiservercel.DeclType{
		propexpr.SourceIdent:  apiservercel.NewObjectType("vela.source", srcFields),
		propexpr.ContextIdent: apiservercel.NewObjectType("vela.context", ctxFields),
	}, nil
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
	key, _ := typedEnvKey(schemaText, ctxSchema)
	return Vela.TypedEnv(key, func() (map[string]*apiservercel.DeclType, error) {
		return declsForContext(schemaText, ctxSchema)
	})
}

// typedEnvKey renders the inputs as a key, and reports whether they can be one.
// Typed environments cost ~100us each to build, and admission builds one per
// expression.
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

func declsForContext(schemaText map[string]string, ctxSchema propexpr.ContextSchema) (map[string]*apiservercel.DeclType, error) {
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

	return rootDecls(sources, contextDecls(ctxSchema))
}

// contextDecls types each field a surface's context offers.
func contextDecls(ctxSchema propexpr.ContextSchema) map[string]*apiservercel.DeclType {
	ctx := map[string]*apiservercel.DeclType{}
	for _, name := range ctxSchema.ReadableFields() {
		fv, ok := ctxSchema.FieldValue(name)
		if !ok {
			continue
		}
		ctx[name] = celengine.DeclType(fv, "vela.context."+name)
	}
	return ctx
}
