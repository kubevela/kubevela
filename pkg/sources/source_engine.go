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

package sources

import (
	"context"
	"fmt"

	"github.com/google/cel-go/cel"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// SourceEngineOptions describes one caller's world: which bindings exist, what
// backs them, and what context they resolve against.
//
// Stated explicitly rather than read off a process.Context. That is what keeps
// resolution testable without a cluster and keeps the render context's protocol
// in one place - ResolveSourceExpressions - rather than spread through here.
type SourceEngineOptions struct {
	// Surface names the call site, and decides which context fields a source may
	// read. One of propexpr.SurfaceNames().
	Surface string
	// Context is the value for each field the surface offers, as far as the caller
	// has them. Fields may legitimately be absent rather than empty - appRevision
	// before the first revision exists, publishVersion unless set - so this is not
	// required to be complete, and an expression reading an absent field is
	// refused at admission by the surface's own schema rather than here.
	Context map[string]interface{}

	// Bindings is binding name -> that binding's own properties, which may
	// themselves contain expressions reading an earlier binding.
	Bindings map[string]map[string]interface{}
	// Types is binding name -> SourceDefinition type, carrying a pinned revision
	// where one was requested.
	Types map[string]string
	// Templates is definition type -> its CUE template.
	Templates map[string]string
	// Sensitive is definition type -> paths its schema marks +sensitive.
	//
	// Optional: any type not listed has its paths derived from its template. Supply
	// this only to add paths a template does not declare. Leaving it nil is the
	// safe default, since a caller that supplied templates and forgot the paths
	// would otherwise get silent under-redaction.
	Sensitive map[string][]string

	// Store persists resolved values between reconciles. Nil resolves correctly
	// and simply re-fetches every time.
	Store velaprocess.SourceCacheStore
	// Compiler evaluates templates. Nil uses the workload compiler.
	Compiler SourceCompiler

	// Validate runs Check before resolving, and refuses rather than resolving an
	// expression that will not type.
	//
	// Off by default, and deliberately not the only way to get typing. The
	// Application checks at admission, where a failure has a field path and a user
	// to show it to; the same failure at render is a component that will not
	// reconcile. A caller with no admission step of its own sets this and gets the
	// check without having to know to call Check separately.
	Validate bool
}

// appendMissing adds entries not already present, so a caller adding a path a
// template does not declare does not also have to restate the ones it does.
func appendMissing(have, extra []string) []string {
	seen := map[string]struct{}{}
	for _, h := range have {
		seen[h] = struct{}{}
	}
	for _, e := range extra {
		if _, dup := seen[e]; dup {
			continue
		}
		seen[e] = struct{}{}
		have = append(have, e)
	}
	return have
}

// SourceEngine resolves source expressions in a properties blob.
//
// One engine is one caller's view for one render: it caches what it resolves so
// a binding read by several properties is fetched once, and it accumulates what
// resolved so the caller can report it. Build a new one per render rather than
// sharing.
type SourceEngine struct {
	opts SourceEngineOptions
}

// SourceResult is a resolution: the substituted properties, and what it took to
// produce them.
type SourceResult struct {
	// Properties is the input with every $( ) expression replaced.
	Properties interface{}
	// Statuses is what each binding resolved to, keyed by binding name - the
	// phase, the storage key, the expiry, and the values consumed. Enough to
	// build status from without the caller knowing how resolution works.
	Statuses map[string]SourceResolutionStatus
}

// NewSourceEngine validates the options and returns an engine.
//
// The surface must be one the registry declares. That is checked here because
// the alternative is silent: an unrecognised surface fails open to "offers
// everything", so a typo would widen what sources may read instead of erroring.
//
// The context deliberately is not checked for completeness. A render populates
// only the fields that have values, so requiring all of them would reject the
// Application's own calls.
func NewSourceEngine(opts SourceEngineOptions) (*SourceEngine, error) {
	if opts.Surface == "" {
		return nil, fmt.Errorf("a surface is required; one of %v", propexpr.SurfaceNames())
	}
	if !propexpr.SurfaceDeclared(opts.Surface) {
		return nil, fmt.Errorf("unknown surface %q; declared surfaces are %v",
			opts.Surface, propexpr.SurfaceNames())
	}
	// Derived rather than required, so redaction cannot be lost by omission.
	sensitive := map[string][]string{}
	for sourceType, template := range opts.Templates {
		sensitive[sourceType] = ExtractSensitiveOutputPaths(template)
	}
	for sourceType, extra := range opts.Sensitive {
		sensitive[sourceType] = appendMissing(sensitive[sourceType], extra)
	}
	opts.Sensitive = sensitive
	return &SourceEngine{opts: opts}, nil
}

// Resolve substitutes every $( ) expression in properties.
//
// properties is any JSON-shaped value - a map, a list, a scalar - and is walked
// to any depth, so an expression inside a list entry or a nested object resolves
// like any other.
func (e *SourceEngine) Resolve(ctx context.Context, properties interface{}) (SourceResult, error) {
	if properties == nil {
		return SourceResult{}, nil
	}
	if e.opts.Validate {
		if bad := e.Check(properties); len(bad) > 0 {
			// The first is enough to act on, and carries the rest's shape. A caller
			// wanting all of them calls Check itself.
			return SourceResult{}, fmt.Errorf("source expressions did not type check: %w", bad[0])
		}
	}
	r := newSourceResolver(ctx, e.opts.Context, e.opts.Surface, sourceInputs{
		Bindings:  e.opts.Bindings,
		Types:     e.opts.Types,
		Templates: e.opts.Templates,
		Sensitive: e.opts.Sensitive,
		Store:     e.opts.Store,
		Compiler:  e.opts.Compiler,
	})
	// Overlap the round trips before walking. Behaviour-neutral: it only
	// populates the memo the walk already consults, and anything it fails to
	// resolve is resolved again, sequentially, by the walk itself.
	r.prefetch(properties)

	out, err := resolveSourceNode(properties, r)
	if err != nil {
		return SourceResult{Statuses: r.statuses}, err
	}
	return SourceResult{Properties: out, Statuses: r.statuses}, nil
}

// CheckError is one expression that will not compile, and where it was found.
type CheckError struct {
	// Property is the path within the properties blob, e.g. "env[0].value".
	Property string
	// Expr is the expression source, without the surrounding $( ).
	Expr string
	Err  error
}

// Unwrap exposes the underlying compile error, so a caller can match on it.
func (c CheckError) Unwrap() error { return c.Err }

func (c CheckError) Error() string {
	if c.Property == "" {
		return fmt.Sprintf("%s: %v", c.Expr, c.Err)
	}
	return fmt.Sprintf("%s: %s: %v", c.Property, c.Expr, c.Err)
}

// typedEnv is the environment in which every source read carries the shape its
// definition declares, and every context read the shape its surface declares.
//
// The permissive environment types both as dyn, which is enough to evaluate but
// not to catch a string flowing into an int. Building this from the templates the
// engine already holds is what lets Check mean something.
func (e *SourceEngine) typedEnv() (*cel.Env, error) {
	// Keyed by binding, not by definition type: an expression reads
	// source.<binding>, and two bindings of one type may be read differently.
	schemas := map[string]string{}
	for binding, sourceType := range e.opts.Types {
		template, ok := e.opts.Templates[sourceType]
		if !ok {
			continue
		}
		expr, err := extractSourceSchemaExpr(template)
		if err != nil || expr == "" {
			// A definition whose schema will not parse types as absent, so the read
			// is reported as undeclared rather than as a fault in the definition -
			// which the definition's own validation reports, with the real cause.
			continue
		}
		schemas[binding] = expr
	}
	return celexpr.EnvForContext(schemas, propexpr.ContextFor(e.opts.Surface))
}

// Check reports every expression in properties that will not compile against the
// bindings and surface this engine was built for.
//
// No I/O and no resolution: this is a parse and a type-check, so it is safe on a
// value that has not been admitted and cheap enough to run before deciding
// whether to resolve at all.
//
// It answers "is this expression valid here" - an undeclared binding, a path the
// schema does not declare, a type error within the expression. It does not
// answer "does the result fit where it is going", because the engine does not
// know the destination; use TypeOf and compare against your own target.
//
// Every problem is reported rather than the first, since a caller validating a
// blob wants all of them in one pass.
func (e *SourceEngine) Check(properties interface{}) []CheckError {
	env, err := e.typedEnv()
	if err != nil {
		return []CheckError{{Err: err}}
	}
	var out []CheckError
	_ = propexpr.Walk(properties, "", func(path, raw string) error {
		parsed, perr := propexpr.Parse(raw)
		if perr != nil {
			out = append(out, CheckError{Property: path, Expr: raw, Err: perr})
			//nolint:nilerr // collected, not raised: the walk reports every bad expression, not the first
			return nil
		}
		if !parsed.HasExpr() {
			return nil
		}
		for _, fragment := range parsed.Fragments {
			if !fragment.IsExpr() {
				continue
			}
			if _, cerr := celexpr.OutputType(env, fragment.Expr); cerr != nil {
				out = append(out, CheckError{Property: path, Expr: fragment.Expr, Err: cerr})
			}
		}
		return nil
	})
	return out
}
