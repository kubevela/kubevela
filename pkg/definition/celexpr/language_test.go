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
	"fmt"
	"reflect"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/google/cel-go/cel"
	apiservercel "k8s.io/apiserver/pkg/cel"
)

// The constructs in this file are the ones the CUE grammar deliberately excluded:
// conditionals, comparisons, function calls and iteration. The inherited test
// suite is therefore shaped like the *old* language, and none of this was covered
// by it - so a green suite said nothing about any of it.

const langSchema = `{
	host:    string
	port:    int
	tier:    string
	secret:  string
	tags:    [...string]
	ports:   [...int]
	members: [...{name: string, role: string, weight: int}]
	meta:    {region: string, zone: string}
	data:    [string]: string
	note?:   string
}`

func langEnv(t *testing.T) *cel.Env {
	t.Helper()
	cc := cuecontext.New()
	a := cc.CompileString("s: " + langSchema).LookupPath(cue.ParsePath("s"))
	if a.Err() != nil {
		t.Fatalf("compiling the schema: %v", a.Err())
	}
	b := cc.CompileString("s: {name: string}").LookupPath(cue.ParsePath("s"))
	env, err := Env(
		map[string]cue.Value{"cfg": a, "other": b},
		map[string]*apiservercel.DeclType{
			"appName":   apiservercel.StringType,
			"namespace": apiservercel.StringType,
		})
	if err != nil {
		t.Fatalf("building the env: %v", err)
	}
	return env
}

func langInput() map[string]interface{} {
	return map[string]interface{}{
		"source": map[string]interface{}{
			"cfg": map[string]interface{}{
				"host": "db.eu.internal", "port": 5432, "tier": "Gold", "secret": "s3cr3t",
				"tags":  []interface{}{"b", "a", "b"},
				"ports": []interface{}{80, 443},
				"members": []interface{}{
					map[string]interface{}{"name": "ana", "role": "admin", "weight": 3},
					map[string]interface{}{"name": "bo", "role": "user", "weight": 1},
				},
				"meta": map[string]interface{}{"region": "eu", "zone": "eu-1a"},
				"data": map[string]interface{}{"image": "nginx:1.25", "replicas": "3"},
			},
			"other": map[string]interface{}{"name": "ana"},
		},
		"context": map[string]interface{}{"appName": "checkout", "namespace": "team-a"},
	}
}

// Iteration, remapping and list handling: what an author reaches for when a source
// returns a collection and a parameter wants a different shape.
func TestListOperationsAndRemapping(t *testing.T) {
	env := langEnv(t)
	in := langInput()

	for _, tc := range []struct {
		expr     string
		wantType string
		want     interface{}
	}{
		// Remap a list of structs into a list of scalars.
		{`source.cfg.members.map(m, m.name)`, "list(string)",
			[]interface{}{"ana", "bo"}},
		// Filter, then remap.
		{`source.cfg.members.filter(m, m.role == "admin").map(m, m.name)`, "list(string)",
			[]interface{}{"ana"}},
		// Remap into a different struct shape.
		{`source.cfg.members.map(m, {"n": m.name, "w": m.weight})`, "list(map(string, dyn))",
			[]interface{}{
				map[string]interface{}{"n": "ana", "w": int64(3)},
				map[string]interface{}{"n": "bo", "w": int64(1)},
			}},
		// Build a map from scratch.
		{`{"region": source.cfg.meta.region, "tier": source.cfg.tier}`, "map(string, string)",
			map[string]interface{}{"region": "eu", "tier": "Gold"}},
		// Arithmetic across a list.
		{`source.cfg.ports.map(p, p * 2)`, "list(int)", []interface{}{int64(160), int64(886)}},
		// Predicates.
		{`source.cfg.members.exists(m, m.role == "admin")`, "bool", true},
		{`source.cfg.members.all(m, m.weight > 0)`, "bool", true},
		{`source.cfg.tags.size()`, "int", int64(3)},
		// Indexing.
		{`source.cfg.members[0].name`, "string", "ana"},
		{`source.cfg.ports[1]`, "int", int64(443)},
		// A read from one binding inside a comprehension over another.
		{`source.cfg.members.exists(m, m.name == source.other.name)`, "bool", true},
		// Concatenation.
		{`source.cfg.ports + [8080]`, "list(int)",
			[]interface{}{int64(80), int64(443), int64(8080)}},
	} {
		got, err := OutputType(env, tc.expr)
		if err != nil {
			t.Errorf("%-62s COMPILE ERROR %v", tc.expr, err)
			continue
		}
		if got.String() != tc.wantType {
			t.Errorf("%-62s typed %s, want %s", tc.expr, got, tc.wantType)
		}
		v, err := Eval(env, tc.expr, in)
		if err != nil {
			t.Errorf("%-62s EVAL ERROR %v", tc.expr, err)
			continue
		}
		if !reflect.DeepEqual(v, tc.want) {
			t.Errorf("%-62s got %#v, want %#v", tc.expr, v, tc.want)
		}
	}
}

// Conditionals. Excluded by the CUE grammar on soundness grounds; CEL types them
// statically, which is what made them available.
func TestConditionals(t *testing.T) {
	env := langEnv(t)
	in := langInput()

	for _, tc := range []struct {
		expr string
		want interface{}
	}{
		{`source.cfg.tier == "Gold" ? 10 : 2`, int64(10)},
		{`source.cfg.port > 1024 ? "high" : "low"`, "high"},
		{`source.cfg.host.startsWith("db") ? source.cfg.host : "fallback"`, "db.eu.internal"},
		// Nested, and the arms still have to unify.
		{`source.cfg.tier == "Gold" ? 10 : (source.cfg.tier == "Silver" ? 5 : 1)`, int64(10)},
		// A guarded read of an optional field.
		{`has(source.cfg.note) ? source.cfg.note : "none"`, "none"},
		// A guarded read of an open-map key, which has() cannot spell.
		{`"image" in source.cfg.data ? source.cfg.data["image"] : "nginx"`, "nginx:1.25"},
		{`"nope" in source.cfg.data ? source.cfg.data["nope"] : "nginx"`, "nginx"},
	} {
		got, err := Eval(env, tc.expr, in)
		if err != nil {
			t.Errorf("%-70s EVAL ERROR %v", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%-70s got %#v, want %#v", tc.expr, got, tc.want)
		}
	}

	// Arms that do not unify are a compile error, which is the whole soundness
	// argument: the result type cannot depend on which arm runs.
	if _, err := OutputType(env, `source.cfg.tier == "Gold" ? 10 : "two"`); err == nil {
		t.Error("a ternary whose arms disagree should not compile")
	}
}

// The Strings and Lists extension libraries. Without these an author can reshape
// structure but not text, which is most of what remapping means in practice.
func TestExtensionLibraries(t *testing.T) {
	env := langEnv(t)
	in := langInput()

	for _, tc := range []struct {
		expr string
		want interface{}
	}{
		{`source.cfg.host.split(".")[0]`, "db"},
		{`source.cfg.members.map(m, m.name).join(",")`, "ana,bo"},
		{`source.cfg.host.replace("internal", "local")`, "db.eu.local"},
		{`source.cfg.tier.lowerAscii()`, "gold"},
		{`source.cfg.tier.upperAscii()`, "GOLD"},
		{`source.cfg.host.substring(0, 2)`, "db"},
		{`("  x  ").trim()`, "x"},
		{`source.cfg.host.indexOf("eu")`, int64(3)},
		{`source.cfg.tier.reverse()`, "dloG"},
		{`source.cfg.tags.slice(0, 2)`, []interface{}{"b", "a"}},
		// The combination the libraries exist for: text out of a remapped list.
		{`source.cfg.members.filter(m, m.role == "admin").map(m, m.name.upperAscii()).join("|")`, "ANA"},
	} {
		got, err := Eval(env, tc.expr, in)
		if err != nil {
			t.Errorf("%-74s EVAL ERROR %v", tc.expr, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%-74s got %#v, want %#v", tc.expr, got, tc.want)
		}
	}
}

// The permissive environment and the typed one must offer the same functions. If
// they drift, an expression is accepted by one pass and refused by the other, and
// which pass reports it depends on whether a schema happened to load.
func TestBothEnvironmentsOfferTheSameFunctions(t *testing.T) {
	typed := langEnv(t)
	dyn, err := DynEnv()
	if err != nil {
		t.Fatal(err)
	}
	for _, expr := range []string{
		`source.cfg.host.split(".")[0]`,
		`source.cfg.tags.slice(0, 1)`,
		`source.cfg.tier.reverse()`,
		`source.cfg.members.map(m, m.name).join(",")`,
		`source.cfg.tier.lowerAscii()`,
		`source.cfg.members.filter(m, m.role == "admin")`,
		`source.cfg.tier == "Gold" ? 10 : 2`,
	} {
		_, terr := OutputType(typed, expr)
		_, derr := OutputType(dyn, expr)
		if (terr == nil) != (derr == nil) {
			t.Errorf("%-52s typed-env err=%v, permissive-env err=%v", expr, terr, derr)
		}
	}
}

// References must see reads inside a comprehension. Three things run off this set
// and all three fail quietly if it under-approximates: which sources a render must
// resolve and in what order, whether a chain forms a cycle, and which resolved
// values are sensitive and must be redacted from status.
func TestReferencesReachInsideComprehensions(t *testing.T) {
	env := langEnv(t)
	for _, tc := range []struct {
		expr string
		want []string
	}{
		{`source.cfg.tags.map(t, t + "-x")`, []string{"source.cfg.tags"}},
		{`source.cfg.members.filter(m, m.role == "admin").size()`, []string{"source.cfg.members"}},
		{`source.cfg.tags.all(t, t != source.cfg.secret)`,
			[]string{"source.cfg.secret", "source.cfg.tags"}},
		{`source.cfg.members.exists(m, m.name == source.other.name)`,
			[]string{"source.cfg.members", "source.other.name"}},
		// A sensitive value reachable only from inside a macro body.
		{`source.cfg.members.map(m, source.cfg.secret)`,
			[]string{"source.cfg.members", "source.cfg.secret"}},
		// Collection literals.
		{`[source.cfg.secret, source.cfg.host]`,
			[]string{"source.cfg.host", "source.cfg.secret"}},
		{`{"k": source.cfg.secret}`, []string{"source.cfg.secret"}},
		// An index whose key comes from the iteration variable: the read is of the
		// map, since the key is not knowable statically.
		{`source.cfg.members.map(m, source.cfg.data[m.name])`,
			[]string{"source.cfg.data", "source.cfg.members"}},
	} {
		refs, err := References(env, tc.expr)
		if err != nil {
			t.Errorf("%-62s COMPILE ERROR %v", tc.expr, err)
			continue
		}
		var got []string
		for _, r := range refs {
			got = append(got, r.String())
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%-62s refs %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// The iteration variable is not a read of anything, and must not leak out as one -
// nor be mistaken for an identifier escaping the sandbox.
func TestIterationVariableIsNotAReference(t *testing.T) {
	env := langEnv(t)
	refs, err := References(env, `source.cfg.members.map(m, m.name + m.role)`)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range refs {
		if r.Root != "source" && r.Root != "context" {
			t.Errorf("iteration variable leaked as a reference: %v", r)
		}
	}
	if err := ValidateTree(map[string]interface{}{
		"p": `$(source.cfg.members.map(m, m.name + m.role))`,
	}, "source", "context"); err != nil {
		t.Errorf("a comprehension should pass the sandbox check: %v", err)
	}
}

// The sandbox has to hold inside a macro body too, which is the one place an
// identifier could plausibly be introduced.
func TestSandboxHoldsInsideMacros(t *testing.T) {
	for _, expr := range []string{
		`parameter.image`,
		`source.cfg.members.map(m, parameter.x)`,
		`source.cfg.members.filter(m, parameter.role == m.role)`,
		`[parameter.a]`,
		`{"k": parameter.a}`,
	} {
		err := ValidateTree(map[string]interface{}{"p": "$(" + expr + ")"}, "source", "context")
		if err == nil {
			t.Errorf("%-56s escaped the sandbox", expr)
		}
	}
}

// Collections must survive substitution as real JSON, not as formatted text.
func TestCollectionsSubstituteAsStructuredValues(t *testing.T) {
	env := langEnv(t)
	in := langInput()

	got, err := EvalProperty(env, `$(source.cfg.members.map(m, m.name))`, in)
	if err != nil {
		t.Fatalf("evaluating: %v", err)
	}
	if !reflect.DeepEqual(got, []interface{}{"ana", "bo"}) {
		t.Errorf("a lone list expression should stay a list; got %#v", got)
	}

	got, err = EvalProperty(env, `$({"a": source.cfg.tier})`, in)
	if err != nil {
		t.Fatalf("evaluating: %v", err)
	}
	if !reflect.DeepEqual(got, map[string]interface{}{"a": "Gold"}) {
		t.Errorf("a lone map expression should stay a map; got %#v", got)
	}
}

// Element types. A CUE kind says only "list", so list(string) feeding a [...int]
// parameter used to pass admission and fail at render.
func TestElementsCompatible(t *testing.T) {
	env := langEnv(t)
	cc := cuecontext.New()
	target := func(decl string) cue.Value {
		return cc.CompileString("t: " + decl).LookupPath(cue.ParsePath("t"))
	}

	for _, tc := range []struct {
		expr  string
		decl  string
		agree bool
	}{
		{`source.cfg.members.map(m, m.name)`, `[...string]`, true},
		{`source.cfg.members.map(m, m.name)`, `[...int]`, false},
		{`source.cfg.members.map(m, m.weight)`, `[...int]`, true},
		{`source.cfg.members.map(m, m.weight)`, `[...string]`, false},
		{`source.cfg.tags`, `[...string]`, true},
		{`source.cfg.ports`, `[...string]`, false},
		{`{"a": source.cfg.tier}`, `[string]: string`, true},
		{`{"a": source.cfg.port}`, `[string]: string`, false},
		// An int feeding a number is a widening, not a mismatch.
		{`source.cfg.ports`, `[...number]`, true},
		// Fail open where the target says nothing precise.
		{`source.cfg.members.map(m, m.name)`, `[...]`, true},
		{`source.cfg.members.map(m, m.name)`, `_`, true},
		// Structs are not judged: open and optional fields make it unsafe.
		{`source.cfg.members`, `[...{name: string}]`, true},
	} {
		st, err := OutputType(env, tc.expr)
		if err != nil {
			t.Errorf("%-40s COMPILE ERROR %v", tc.expr, err)
			continue
		}
		agree, want, got := ElementsCompatible(st, target(tc.decl))
		if agree != tc.agree {
			t.Errorf("%-40s -> %-22s vs %-18s agree=%v, want %v (want=%s got=%s)",
				tc.expr, st, tc.decl, agree, tc.agree, want, got)
		}
	}
}

// A nil type must never be treated as a mismatch: it means "nothing precise to
// say", which is the case for every interpolated string.
func TestElementsCompatibleFailsOpenOnNil(t *testing.T) {
	cc := cuecontext.New()
	v := cc.CompileString("t: [...int]").LookupPath(cue.ParsePath("t"))
	if agree, _, _ := ElementsCompatible(nil, v); !agree {
		t.Error("a nil expression type must fail open")
	}
	if agree, _, _ := ElementsCompatible(cel.StringType, cue.Value{}); !agree {
		t.Error("an absent target must fail open")
	}
}

// Iterating a map has no defined order, and the result is a list - so a remapped
// map produces a list whose order varies between evaluations.
//
// This matters beyond the test that flaked on it. A property rendered from such an
// expression differs between reconciles even when nothing changed, which shows up
// as a resource rewritten on every pass. There is no sort() at this cel-go version
// to impose an order, so the answer is to document it: iterate a list, or read
// keys individually, when the output order is part of the resource.
func TestMapIterationOrderIsNotStable(t *testing.T) {
	env := langEnv(t)
	in := langInput()
	expr := `source.cfg.data.map(k, k + "=" + source.cfg.data[k])`

	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		v, err := Eval(env, expr, in)
		if err != nil {
			t.Fatalf("evaluating: %v", err)
		}
		list, ok := v.([]interface{})
		if !ok {
			t.Fatalf("expected a list, got %T", v)
		}
		if len(list) != 2 {
			t.Fatalf("expected both keys, got %#v", list)
		}
		seen[fmt.Sprint(list)] = true
	}
	// Both orderings are legitimate; the point is that neither can be relied on.
	// Only assert the contents, never the order.
	t.Logf("observed %d distinct orderings over 50 evaluations", len(seen))

	// A list keeps its order, which is the alternative to recommend.
	v, err := Eval(env, `source.cfg.members.map(m, m.name)`, in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v, []interface{}{"ana", "bo"}) {
		t.Errorf("iterating a list must preserve order; got %#v", v)
	}
}

// Resolved values reach evaluation having been through JSON, where an int and a
// float are the same thing. CEL is strict about mixed arithmetic, so without
// normalisation `port + 1000` fails at render on a value that type-checked at
// admission.
func TestJSONNumbersSurviveAsIntegers(t *testing.T) {
	var vals map[string]interface{}
	if err := json.Unmarshal([]byte(
		`{"port":8080,"replicas":3,"ratio":1.5,"ports":[80,443],"nested":{"n":7}}`), &vals); err != nil {
		t.Fatal(err)
	}
	if _, isFloat := vals["port"].(float64); !isFloat {
		t.Fatalf("precondition: JSON should decode a number as float64, got %T", vals["port"])
	}

	for _, tc := range []struct {
		expr string
		want interface{}
	}{
		{`source.cfg.port + 1000`, int64(9080)},
		{`source.cfg.port / 2`, int64(4040)},
		{`source.cfg.replicas * 3`, int64(9)},
		{`source.cfg.ports[0] + 1`, int64(81)},
		{`source.cfg.nested.n - 1`, int64(6)},
		{`source.cfg.ports.map(p, p * 2)`, []interface{}{int64(160), int64(886)}},
		// A genuine float keeps its type, so float arithmetic still works.
		{`source.cfg.ratio * 2.0`, float64(3)},
	} {
		got, err := EvalTree(map[string]interface{}{"v": "$(" + tc.expr + ")"},
			map[string]map[string]interface{}{"cfg": vals}, map[string]interface{}{})
		if err != nil {
			t.Errorf("%-34s ERROR %v", tc.expr, err)
			continue
		}
		v := got.(map[string]interface{})["v"]
		if !reflect.DeepEqual(v, tc.want) {
			t.Errorf("%-34s got %#v (%T), want %#v", tc.expr, v, v, tc.want)
		}
	}
}
