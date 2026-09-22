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

package celexpr

import (
	"encoding/json"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/google/cel-go/cel"

	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// The CEL environment is built from the shared context registry, so a surface
// offers the same fields to both engines. That is the property that makes a swap
// tractable at all: one declaration, two type systems reading it.
func TestSurfaceParity(t *testing.T) {
	cc := cuecontext.New()
	v := cc.CompileString("s: {host: string}").LookupPath(cue.ParsePath("s"))
	sources := map[string]cue.Value{"cfg": v}

	for _, surface := range []string{"component", "trait", "workflowstep", "policy-rendered"} {
		env, err := EnvForSurface(sources, surface)
		if err != nil {
			t.Fatalf("%s: building the env: %v", surface, err)
		}
		schema := propexpr.ContextFor(surface)

		// Every field the registry says this surface offers must type-check.
		for _, f := range schema.ReadableFields() {
			if f == "custom" {
				continue // `_` by construction; typed as Any, nothing to compare
			}
			if _, err := OutputType(env, "context."+f); err != nil {
				t.Errorf("%s offers context.%s but CEL rejected it: %v", surface, f, err)
			}
		}

		// And a field it does not offer must be rejected, not silently dyn.
		if _, err := OutputType(env, "context.nosuchfield"); err == nil {
			t.Errorf("%s: an undeclared context field was accepted", surface)
		}
		t.Logf("%-16s %d context fields, all typed", surface, len(schema.ReadableFields()))
	}
}

// A surface-specific field must be absent where the surface does not offer it -
// the same restriction the CUE path enforces, arrived at from the same registry.
func TestSurfaceRestrictionParity(t *testing.T) {
	cc := cuecontext.New()
	v := cc.CompileString("s: {host: string}").LookupPath(cue.ParsePath("s"))
	sources := map[string]cue.Value{"cfg": v}

	for _, tc := range []struct {
		field   string
		offered []string
		denied  []string
	}{
		{"componentName", []string{"component", "trait"}, []string{"workflowstep", "policy-rendered"}},
		{"traitType", []string{"trait"}, []string{"component", "workflowstep"}},
		{"stepName", []string{"workflowstep"}, []string{"component", "trait"}},
		{"policyName", []string{"policy-rendered"}, []string{"component", "trait", "workflowstep"}},
	} {
		for _, s := range tc.offered {
			env, err := EnvForSurface(sources, s)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := OutputType(env, "context."+tc.field); err != nil {
				t.Errorf("%s should offer context.%s: %v", s, tc.field, err)
			}
		}
		for _, s := range tc.denied {
			env, err := EnvForSurface(sources, s)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := OutputType(env, "context."+tc.field); err == nil {
				t.Errorf("%s must not offer context.%s", s, tc.field)
			}
		}
		t.Logf("context.%-14s offered on %v, denied on %v", tc.field, tc.offered, tc.denied)
	}
}

// The default form. `*read | fallback` has no CEL equivalent; has() plus a ternary
// is the replacement, so the capability survives even though the spelling changes.
func TestOptionalDefaults(t *testing.T) {
	cc := cuecontext.New()
	v := cc.CompileString(`s: {host: string, note?: string, data: [string]: string}`).
		LookupPath(cue.ParsePath("s"))
	env, err := EnvForSurface(map[string]cue.Value{"cfg": v}, "component")
	if err != nil {
		t.Fatal(err)
	}

	in := map[string]interface{}{
		"source":  map[string]interface{}{"cfg": map[string]interface{}{"host": "h", "data": map[string]interface{}{}}},
		"context": map[string]interface{}{},
	}

	for _, tc := range []struct {
		expr string
		want interface{}
	}{
		{`has(source.cfg.note) ? source.cfg.note : "none"`, "none"},
		{`source.cfg.data["absent"]`, nil}, // reading an absent map key errors
	} {
		got, err := Eval(env, tc.expr, in)
		if tc.want == nil {
			if err == nil {
				t.Errorf("%-42s should have failed, got %#v", tc.expr, got)
			} else {
				t.Logf("%-42s errors as expected: absent map key", tc.expr)
			}
			continue
		}
		if err != nil {
			t.Errorf("%-42s ERROR %v", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%-42s got %#v, want %#v", tc.expr, got, tc.want)
			continue
		}
		t.Logf("%-42s -> %#v", tc.expr, got)
	}
}

// The undefended-read rule, which CEL's checker does not give us.
//
// An unguarded read of an optional field compiles cleanly as its declared type
// and then fails at render with "no such key", so the rule the CUE path enforces
// has to be enforced here too - from the AST rather than from the type.
func TestUndefendedReads(t *testing.T) {
	cc := cuecontext.New()
	v := cc.CompileString(`s: {host: string, note?: string, data: [string]: string}`).
		LookupPath(cue.ParsePath("s"))
	env, err := EnvForSurface(map[string]cue.Value{"cfg": v}, "component")
	if err != nil {
		t.Fatal(err)
	}
	optional := func(r propexpr.Reference) bool {
		p := strings.Join(r.Path, ".")
		return p == "cfg.note" || strings.HasPrefix(p, "cfg.data.")
	}

	for _, tc := range []struct {
		expr string
		want bool // true = must be flagged
	}{
		{`source.cfg.host`, false},
		{`source.cfg.note`, true},
		{`source.cfg.data["image"]`, true},
		{`source.cfg.host + source.cfg.note`, true},
		{`has(source.cfg.note) ? source.cfg.note : "none"`, false},
		{`has(source.cfg.data.image) ? source.cfg.data.image : "nginx"`, false},
		// has() is a macro expanding to a test-only select, so the guard is on the
		// read itself rather than on an enclosing call.
		{`has(source.cfg.note)`, false},
		// A ternary *condition* is always evaluated, so a read there is not
		// guarded. Getting this wrong is a false negative in the safety check.
		{`source.cfg.note == "x" ? "a" : "b"`, true},
	} {
		bad, err := UndefendedReads(env, tc.expr, optional)
		if err != nil {
			t.Errorf("%-52s ERROR %v", tc.expr, err)
			continue
		}
		if got := len(bad) > 0; got != tc.want {
			t.Errorf("%-52s flagged=%v, want %v (%v)", tc.expr, got, tc.want, bad)
			continue
		}
		t.Logf("%-52s flagged=%v", tc.expr, len(bad) > 0)
	}
}

// A value with no statically known type must not silently satisfy a concrete
// parameter. Reading below an untyped region yields dyn, and dyn is assignable to
// anything - weaker than the CUE path, where an assertion is mandatory.
func TestTargetGuards(t *testing.T) {
	cc := cuecontext.New()
	v := cc.CompileString(`s: {host: string, port: int, blob: _}`).LookupPath(cue.ParsePath("s"))
	env, err := EnvForSurface(map[string]cue.Value{"cfg": v}, "component")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		expr    string
		target  *cel.Type
		wantErr string
	}{
		{`source.cfg.port`, cel.IntType, ""},
		{`source.cfg.host`, cel.IntType, "type mismatch"},
		{`source.cfg.blob.deep`, cel.IntType, "no statically known type"},
		// An explicit conversion is the way through, as `& int` is today.
		{`int(source.cfg.blob.deep)`, cel.IntType, ""},
		// Nothing to contradict when the target is itself open.
		{`source.cfg.blob.deep`, cel.DynType, ""},
	} {
		err := CheckTarget(env, tc.expr, tc.target)
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("%-34s -> %-6v unexpected: %v", tc.expr, tc.target, err)
		case tc.wantErr != "" && err == nil:
			t.Errorf("%-34s -> %-6v expected %q, got nil", tc.expr, tc.target, tc.wantErr)
		case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
			t.Errorf("%-34s -> %-6v want %q, got %v", tc.expr, tc.target, tc.wantErr, err)
		default:
			t.Logf("%-34s -> %-6v ok", tc.expr, tc.target)
		}
	}
}

// A substituted value has to end up in an Application's properties, so whatever
// an expression returns must be ordinary Go data that marshals.
//
// ref.Val.Value() is only shallow: a field selection returns what was put in, but
// anything CEL *constructs* is built from its own types, so `{"a": x}` yields
// map[ref.Val]ref.Val and `[x, y]` yields []ref.Val. Neither survives JSON.
func TestNativeValues(t *testing.T) {
	env := testEnv(t)
	in := map[string]interface{}{
		"source": map[string]interface{}{"cfg": map[string]interface{}{
			"host": "db", "port": 5432, "replicas": 6, "secure": true, "tier": "gold",
			"meta": map[string]interface{}{"region": "eu-west", "zone": "z"},
			"data": map[string]interface{}{"image": "nginx:1.25", "tag": "v1"},
		}},
		"context": map[string]interface{}{"appName": "a", "namespace": "n", "cluster": "c"},
	}

	for _, tc := range []struct{ expr, want string }{
		{`source.cfg.meta`, `{"region":"eu-west","zone":"z"}`},
		{`source.cfg.data`, `{"image":"nginx:1.25","tag":"v1"}`},
		{`{"a": source.cfg.host, "b": source.cfg.port}`, `{"a":"db","b":5432}`},
		{`[source.cfg.host, source.cfg.tier]`, `["db","gold"]`},
		{`{"nested": {"deep": [source.cfg.port]}}`, `{"nested":{"deep":[5432]}}`},
		// Iterating a map is covered by TestMapIterationOrderIsNotStable, which
		// does not assert an order - this one did, and flaked on it.
	} {
		v, err := Eval(env, tc.expr, in)
		if err != nil {
			t.Errorf("%-52s ERROR %v", tc.expr, err)
			continue
		}
		j, err := json.Marshal(v)
		if err != nil {
			t.Errorf("%-52s %T does not marshal: %v", tc.expr, v, err)
			continue
		}
		if string(j) != tc.want {
			t.Errorf("%-52s got %s, want %s", tc.expr, j, tc.want)
			continue
		}
		t.Logf("%-52s %s", tc.expr, j)
	}
}

// The guard detection, probed adversarially.
//
// It walks the AST, so formatting cannot fool it. Asking only "is this read
// inside a ternary arm" is wrong in the unsafe direction twice over, and these
// are the cases that say so.
func TestGuardResilience(t *testing.T) {
	cc := cuecontext.New()
	v := cc.CompileString(`s: {host: string, note?: string, other?: string}`).
		LookupPath(cue.ParsePath("s"))
	env, err := EnvForSurface(map[string]cue.Value{"cfg": v}, "component")
	if err != nil {
		t.Fatal(err)
	}
	optional := func(r propexpr.Reference) bool {
		p := strings.Join(r.Path, ".")
		return p == "cfg.note" || p == "cfg.other"
	}

	for _, tc := range []struct {
		expr    string
		flagged bool
		why     string
	}{
		{`source.cfg.host`, false, "not optional"},
		{`source.cfg.note`, true, "bare optional read"},
		{`has(source.cfg.note) ? source.cfg.note : "x"`, false, "guarded"},

		// Formatting is irrelevant - this is an AST walk, not string matching.
		{`has( source.cfg.note )?source.cfg.note:"x"`, false, "whitespace"},
		{"has(source.cfg.note)\n ? source.cfg.note\n : \"x\"", false, "newlines"},
		{`(has(source.cfg.note)) ? (source.cfg.note) : ("x")`, false, "parens"},

		// The guard has to test THIS path. Testing a sibling defends nothing, and
		// treating it as a guard let a read through that fails at render.
		{`has(source.cfg.other) ? source.cfg.note : "x"`, true, "guard tests another field"},

		// A read in a condition is always evaluated. Nesting must not launder it:
		// the inner condition sits inside the outer ternary's arm.
		{`true ? (source.cfg.note == "a" ? "x" : "y") : "z"`, true, "nested condition"},

		// Guarded once does not defend a second, bare read.
		{`(has(source.cfg.note) ? source.cfg.note : "x") + source.cfg.note`, true, "second read bare"},

		// CEL's logical operators absorb an error from one side when the other
		// settles the result, so this is a real guard.
		{`has(source.cfg.note) && source.cfg.note == "a"`, false, "&& short-circuit"},
	} {
		bad, err := UndefendedReads(env, tc.expr, optional)
		if err != nil {
			t.Errorf("%-58s ERROR %v", tc.expr, err)
			continue
		}
		if got := len(bad) > 0; got != tc.flagged {
			t.Errorf("%-58s flagged=%v, want %v (%s)",
				strings.ReplaceAll(tc.expr, "\n", "\\n"), got, tc.flagged, tc.why)
			continue
		}
		t.Logf("%-58s flagged=%-5v %s", strings.ReplaceAll(tc.expr, "\n", "\\n"), tc.flagged, tc.why)
	}
}
