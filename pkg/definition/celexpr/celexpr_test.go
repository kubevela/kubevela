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
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/require"
	apiservercel "k8s.io/apiserver/pkg/cel"
)

// A schema shaped like a real SourceDefinition: typed scalars, a nested struct,
// an optional field, and the open `data` map the configmap source returns.
const schemaSrc = `{
	host:     string
	port:     int
	replicas: int
	secure:   bool
	tier:     string
	note?:    string
	meta:     {region: string, zone: string}
	data: [string]: string
}`

func testEnv(t *testing.T) *cel.Env {
	t.Helper()
	cc := cuecontext.New()
	v := cc.CompileString("s: " + schemaSrc).LookupPath(cue.ParsePath("s"))
	if v.Err() != nil {
		t.Fatalf("compiling the schema: %v", v.Err())
	}
	env, err := Env(
		map[string]cue.Value{"cfg": v},
		map[string]*apiservercel.DeclType{
			"appName":   apiservercel.StringType,
			"namespace": apiservercel.StringType,
			"cluster":   apiservercel.StringType,
		},
	)
	if err != nil {
		t.Fatalf("building the env: %v", err)
	}
	return env
}

// The spike's central question: does CEL give a static result type, before any
// value exists, for the shapes an Application actually writes?
func TestOutputTypes(t *testing.T) {
	env := testEnv(t)

	for _, tc := range []struct{ expr, want string }{
		{`source.cfg.host`, "string"},
		{`source.cfg.port`, "int"},
		{`source.cfg.secure`, "bool"},
		{`source.cfg.meta.region`, "string"},
		{`source.cfg.data["image"]`, "string"},
		{`source.cfg.host + ":" + string(source.cfg.port)`, "string"},
		{`source.cfg.replicas / 2`, "int"},
		{`context.appName + "." + context.namespace`, "string"},
		// The case the CUE grammar refuses outright. CEL's ternary requires both
		// arms to unify, so the result type is value-independent by construction.
		{`source.cfg.tier == "gold" ? 10 : 2`, "int"},
		{`has(source.cfg.note) ? source.cfg.note : "none"`, "string"},
	} {
		got, err := OutputType(env, tc.expr)
		if err != nil {
			t.Errorf("%-52s COMPILE ERROR %v", tc.expr, err)
			continue
		}
		if got.String() != tc.want {
			t.Errorf("%-52s got %s, want %s", tc.expr, got, tc.want)
			continue
		}
		t.Logf("%-52s -> %s", tc.expr, got)
	}
}

// Mistakes an author makes must be compile errors, not runtime surprises.
func TestRejections(t *testing.T) {
	env := testEnv(t)

	for _, expr := range []string{
		`source.cfg.nope`,                        // undeclared field
		`source.nosuchbinding.host`,              // undeclared binding
		`parameter.image`,                        // outside the sandbox
		`source.cfg.host + source.cfg.port`,      // string + int
		`source.cfg.tier == "gold" ? 10 : "two"`, // arms do not unify
	} {
		if _, err := OutputType(env, expr); err == nil {
			t.Errorf("%-46s was ACCEPTED; expected a compile error", expr)
		} else {
			t.Logf("%-46s rejected", expr)
		}
	}
}

// And it has to evaluate, with the types surviving.
func TestEval(t *testing.T) {
	env := testEnv(t)

	in := map[string]interface{}{
		"source": map[string]interface{}{
			"cfg": map[string]interface{}{
				"host": "db.internal", "port": 5432, "replicas": 6,
				"secure": true, "tier": "gold",
				"meta": map[string]interface{}{"region": "eu-west", "zone": "eu-west-1a"},
				"data": map[string]interface{}{"image": "nginx:1.25"},
			},
		},
		"context": map[string]interface{}{
			"appName": "checkout", "namespace": "team-a", "cluster": "local",
		},
	}

	for _, tc := range []struct {
		expr string
		want interface{}
	}{
		{`source.cfg.host`, "db.internal"},
		{`source.cfg.replicas / 2`, int64(3)},
		{`source.cfg.host + ":" + string(source.cfg.port)`, "db.internal:5432"},
		{`source.cfg.data["image"]`, "nginx:1.25"},
		{`source.cfg.tier == "gold" ? 10 : 2`, int64(10)},
		{`context.appName + "." + context.namespace`, "checkout.team-a"},
	} {
		got, err := Eval(env, tc.expr, in)
		if err != nil {
			t.Errorf("%-52s EVAL ERROR %v", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%-52s got %#v, want %#v", tc.expr, got, tc.want)
			continue
		}
		t.Logf("%-52s -> %#v", tc.expr, got)
	}
}

// A binding is read as a field selection, so its name has to be an identifier.
//
// This is the one constraint CEL imposes that CUE did not: `source.cluster-info`
// parses as subtraction, and there is no bracket form to fall back on because
// `source` is an object type rather than a map. Catching it when the binding is
// declared gives a better error than a compile failure inside every expression
// that reads it.
func TestBindingNames(t *testing.T) {
	for _, ok := range []string{"cfg", "clusterInfo", "app_config", "_x", "s3"} {
		if err := ValidBindingName(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"cluster-info", "app.config", "9lives", "has space", ""} {
		if err := ValidBindingName(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}

	// The message has to name the fix, not just the rule.
	err := ValidBindingName("cluster-info")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"cluster-info", "source.cluster-info", "clusterinfo"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message should mention %q; got %v", want, err)
		}
	}
	t.Logf("%v", err)
}

// And a bad name is refused when the environment is built, not left to surface as
// a confusing compile error inside each expression.
func TestEnvRejectsBadBinding(t *testing.T) {
	cc := cuecontext.New()
	v := cc.CompileString("s: {host: string}").LookupPath(cue.ParsePath("s"))
	if _, err := Env(map[string]cue.Value{"cluster-info": v}, nil); err == nil {
		t.Fatal("Env should refuse a hyphenated binding name")
	} else {
		t.Logf("%v", err)
	}
}

// Interpolation survives the swap to CEL, because the `$( )` splitting was never
// part of the expression language.
//
// propexpr.Parse breaks a value into text and expression fragments and contains
// no CUE; only the contents of each fragment change. A lone expression keeps its
// type, one embedded in text yields a string - the same rule as today.
func TestInterpolation(t *testing.T) {
	env := testEnv(t)
	in := map[string]interface{}{
		"source": map[string]interface{}{"cfg": map[string]interface{}{
			"host": "db.internal", "port": 5432, "replicas": 6, "secure": true,
			"tier": "gold",
			"meta": map[string]interface{}{"region": "eu-west", "zone": "z"},
			"data": map[string]interface{}{"image": "nginx:1.25"},
		}},
		"context": map[string]interface{}{
			"appName": "checkout", "namespace": "team-a", "cluster": "local"},
	}

	for _, tc := range []struct {
		raw  string
		want interface{}
	}{
		{`https://$(source.cfg.host)/health`, "https://db.internal/health"},
		{`$(source.cfg.host):$(source.cfg.port)`, "db.internal:5432"},
		{`tier-$(source.cfg.tier)-$(context.appName)`, "tier-gold-checkout"},
		// A lone expression is not stringified.
		{`$(source.cfg.replicas)`, int64(6)},
		{`$(source.cfg.secure)`, true},
		{`$(source.cfg.tier == "gold" ? 10 : 2)`, int64(10)},
		// No expression at all: returned exactly as it came in.
		{`plain literal, no expression`, "plain literal, no expression"},
	} {
		got, err := EvalProperty(env, tc.raw, in)
		if err != nil {
			t.Errorf("%-46s ERROR %v", tc.raw, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%-46s got %#v, want %#v", tc.raw, got, tc.want)
			continue
		}
		t.Logf("%-46s -> %#v", tc.raw, got)
	}
}

// The escape has to collapse whether or not the same value also carries an
// expression. It used to do so only in the mixed case, because the no-expression
// branch returned the raw string, which meant an author who escaped
// $(SERVICE_HOST) shipped $$(SERVICE_HOST) and Kubernetes rendered the literal
// text $(SERVICE_HOST) instead of expanding it.
func TestEvalPropertyCollapsesEscapesWithoutAnExpression(t *testing.T) {
	env, err := cel.NewEnv()
	require.NoError(t, err)

	for _, tc := range []struct{ name, raw, want string }{
		{"escape alone", "$$(SERVICE_HOST)", "$(SERVICE_HOST)"},
		{"two escapes", "$$(HOST):$$(PORT)", "$(HOST):$(PORT)"},
		{"in a command", "echo $$(hostname)", "echo $(hostname)"},
		{"plain value untouched", "nginx:1.25.0", "nginx:1.25.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EvalProperty(env, tc.raw, map[string]interface{}{})
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
