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

package ctxschema

import (
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

// custom compiles the view as a component template would see it, with the
// extra source standing in for the template.
func custom(t *testing.T, v *View, template string) cue.Value {
	t.Helper()
	imports, ctx := v.Source()
	src := imports + "\n" + template + "\n" + ctx
	val := cuecontext.New().CompileString(src)
	require.NoError(t, val.Err(), src)
	return val.LookupPath(cue.ParsePath("context.custom"))
}

// accepts reports whether context.custom accepts a value of that shape.
func accepts(t *testing.T, v *View, value string) bool {
	t.Helper()
	c := custom(t, v, "")
	u := c.Unify(c.Context().CompileString(value))
	return u.Validate() == nil
}

func policy(name, template string) Policy { return Policy{Name: name, Template: template} }

func TestBuildCompleteness(t *testing.T) {
	cases := map[string]struct {
		policies []Policy
		complete bool
	}{
		"no policies":                    {nil, true},
		"a policy without ctx":           {[]Policy{policy("p", `output: labels: a: "b"`)}, true},
		"a closed schema":                {[]Policy{policy("p", "schema: ctx: close({team: string})\noutput: ctx: team: \"a\"")}, true},
		"an open schema":                 {[]Policy{policy("p", "schema: ctx: team: string\noutput: ctx: team: \"a\"")}, false},
		"a closed schema with a pattern": {[]Policy{policy("p", "schema: ctx: close({[string]: string})\noutput: ctx: team: \"a\"")}, false},
		"a closed schema made open":      {[]Policy{policy("p", "schema: ctx: close({team: string, ...})\noutput: ctx: team: \"a\"")}, false},
		"ctx without a schema":           {[]Policy{policy("p", `output: ctx: team: "a"`)}, false},
		"a schema but no ctx":            {[]Policy{policy("p", "schema: ctx: close({team: string})\noutput: labels: a: \"b\"")}, true},
		"adds policies":                  {[]Policy{policy("p", "schema: ctx: close({team: string})\noutput: {ctx: team: \"a\", policies: []}")}, false},
		"an unknown policy":              {[]Policy{{Name: "p", Unknown: true}}, false},
		"one of two schema-less":         {[]Policy{policy("a", "schema: ctx: close({team: string})\noutput: ctx: team: \"a\""), policy("b", `output: ctx: zone: "z"`)}, false},
		"two closed schemas":             {[]Policy{policy("a", "schema: ctx: close({team: string})\noutput: ctx: team: \"a\""), policy("b", "schema: ctx: close({zone: string})\noutput: ctx: zone: \"z\"")}, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			v := Build(tc.policies)
			require.Equal(t, tc.complete, v.Complete)
			require.Empty(t, v.Warnings)
		})
	}
}

func TestBuildWarnsAndTreatsUnreadablePoliciesAsUnknown(t *testing.T) {
	cases := map[string]struct {
		template string
		warning  string
	}{
		"a template that does not parse": {`output: {`, `policy "broken": cannot read its template`},
		"a schema that does not compile": {"_t: string\nschema: ctx: team: _t\noutput: ctx: team: \"a\"", `policy "broken": cannot read schema.ctx`},
		"a schema that is not a struct":  {"schema: ctx: string\noutput: ctx: team: \"a\"", `policy "broken": cannot read schema.ctx`},
		"a schema that is a disjunction": {"schema: ctx: close({team: string}) | close({zone: string})\noutput: ctx: team: \"a\"", `policy "broken": cannot read schema.ctx`},
		"a schema given by reference":    {"_s: ctx: close({team: *\"p\" | string})\nschema: _s\noutput: labels: a: \"b\"", `policy "broken": declares a schema admission cannot read`},
		"ctx under a comprehension":      {"schema: {if true {ctx: close({team: *\"p\" | string})}}\noutput: labels: a: \"b\"", `policy "broken": declares a schema admission cannot read`},
		"ctx in an embedding":            {"_s: ctx: close({team: *\"p\" | string})\nschema: {_s}\noutput: labels: a: \"b\"", `policy "broken": declares a schema admission cannot read`},
		"ctx under a dynamic label":      {"_k: \"ctx\"\nschema: {(_k): close({team: *\"p\" | string})}\noutput: labels: a: \"b\"", `policy "broken": declares a schema admission cannot read`},
		"a schema under a dynamic label": {"_k: \"schema\"\n(_k): ctx: close({team: *\"p\" | string})\noutput: labels: a: \"b\"", `policy "broken": declares a schema admission cannot read`},
		"a schema under a comprehension": {"if true {schema: ctx: close({team: *\"p\" | string})}\noutput: labels: a: \"b\"", `policy "broken": declares a schema admission cannot read`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			v := Build([]Policy{policy("broken", tc.template)})
			require.False(t, v.Complete)
			require.Len(t, v.Warnings, 1)
			require.Contains(t, v.Warnings[0], tc.warning)
			require.False(t, v.Declares("team"))
		})
	}
}

func TestBuildTypesKeys(t *testing.T) {
	closed := Build([]Policy{policy("identity", `
import "strings"
schema: ctx: close({
	team:  =~"^[a-z]+$"
	name:  strings.MinRunes(3)
	mesh?: string
	org: close({unit: string, zone?: int})
})
output: ctx: {team: "a", name: "abc", org: unit: "u"}
`)})
	require.True(t, closed.Complete)

	for value, ok := range map[string]bool{
		`{team: "abc"}`:                    true,
		`{team: "ABC"}`:                    false,
		`{name: "ab"}`:                     false,
		`{name: "abc"}`:                    true,
		`{mesh: "m"}`:                      true,
		`{mesh: 1}`:                        false,
		`{other: 1}`:                       false,
		`{org: {unit: "u", zone: 1}}`:      true,
		`{org: {unit: "u", zone: "eu"}}`:   false,
		`{org: {unit: "u", region: "eu"}}`: false,
	} {
		require.Equal(t, ok, accepts(t, closed, value), value)
	}

	require.True(t, closed.Declares("team"))
	require.True(t, closed.Declares("mesh"))
	require.False(t, closed.Declares("other"))
	_, optional := closed.Optional("team")
	require.False(t, optional)
	by, optional := closed.Optional("mesh")
	require.True(t, optional)
	require.Equal(t, "identity", by)
}

func TestBuildOpenViewAcceptsUndeclaredKeys(t *testing.T) {
	v := Build([]Policy{
		policy("typed", "schema: ctx: close({team: string})\noutput: ctx: team: \"a\""),
		policy("untyped", `output: ctx: zone: "z"`),
	})
	require.False(t, v.Complete)
	require.True(t, accepts(t, v, `{team: "a", zone: "z"}`))
	require.False(t, accepts(t, v, `{team: 1}`), "a declared key is still typed")
}

func TestBuildConditionalPoliciesMakeKeysOptional(t *testing.T) {
	v := Build([]Policy{policy("sometimes", `
config: enabled: context.namespace == "prod"
schema: ctx: close({team: string})
output: ctx: team: "a"
`)})
	require.True(t, v.Complete)
	by, optional := v.Optional("team")
	require.True(t, optional)
	require.Equal(t, "sometimes", by)
}

// The controller sets each top-level key from the last policy that sets it, so
// the view types a key by the last policy certain to set it, and by any later
// one that may.
func TestBuildKeysSeveralPoliciesDeclare(t *testing.T) {
	cases := map[string]struct {
		policies []Policy
		accepts  map[string]bool
		optional bool
	}{
		"the last required declaration wins": {
			policies: []Policy{
				policy("a", "schema: ctx: close({team: int})\noutput: ctx: team: 1"),
				policy("b", "schema: ctx: close({team: string})\noutput: ctx: team: \"x\""),
			},
			accepts: map[string]bool{`{team: "x"}`: true, `{team: 1}`: false},
		},
		"a later optional one adds its type": {
			policies: []Policy{
				policy("a", "schema: ctx: close({team: int})\noutput: ctx: team: 1"),
				policy("b", "schema: ctx: close({team?: string})\noutput: ctx: {}"),
			},
			accepts: map[string]bool{`{team: "x"}`: true, `{team: 1}`: true, `{team: true}`: false},
		},
		"a later conditional one adds its type": {
			policies: []Policy{
				policy("a", "schema: ctx: close({team: int})\noutput: ctx: team: 1"),
				policy("b", "config: enabled: parameter.on\nschema: ctx: close({team: string})\noutput: ctx: team: \"x\""),
			},
			accepts: map[string]bool{`{team: "x"}`: true, `{team: 1}`: true},
		},
		"an earlier one is overridden by a later required one even when optional": {
			policies: []Policy{
				policy("a", "schema: ctx: close({team?: int})\noutput: ctx: {}"),
				policy("b", "schema: ctx: close({team: string})\noutput: ctx: team: \"x\""),
			},
			accepts: map[string]bool{`{team: "x"}`: true, `{team: 1}`: false},
		},
		"no declaration certain to set it leaves it optional": {
			policies: []Policy{
				policy("a", "schema: ctx: close({team?: int})\noutput: ctx: {}"),
				policy("b", "config: enabled: false\nschema: ctx: close({team: string})\noutput: ctx: team: \"x\""),
			},
			accepts:  map[string]bool{`{team: "x"}`: true, `{team: 1}`: true},
			optional: true,
		},
		"keys of two closed schemas are both allowed": {
			policies: []Policy{
				policy("a", "schema: ctx: close({team: string})\noutput: ctx: team: \"x\""),
				policy("b", "schema: ctx: close({zone: string})\noutput: ctx: zone: \"z\""),
			},
			accepts: map[string]bool{`{team: "x", zone: "z"}`: true, `{other: 1}`: false},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			v := Build(tc.policies)
			for value, ok := range tc.accepts {
				require.Equal(t, ok, accepts(t, v, value), value)
			}
			_, optional := v.Optional("team")
			require.Equal(t, tc.optional, optional)
		})
	}
}

func TestOptionalNamesTheLastPolicyDeclaringTheKey(t *testing.T) {
	v := Build([]Policy{
		policy("first", "schema: ctx: close({mesh?: string})\noutput: ctx: {}"),
		policy("second", "schema: ctx: close({mesh?: string})\noutput: ctx: {}"),
	})
	by, optional := v.Optional("mesh")
	require.True(t, optional)
	require.Equal(t, "second", by)
	_, optional = v.Optional("undeclared")
	require.False(t, optional)
}

func TestBuildSourceStandsBesideATemplate(t *testing.T) {
	v := Build([]Policy{
		policy("a", "import \"strings\"\nschema: ctx: close({team: strings.MinRunes(3)})\noutput: ctx: team: \"abc\""),
		policy("b", "import s \"strings\"\nschema: ctx: close({zone: s.HasPrefix(\"eu-\")})\noutput: ctx: zone: \"eu-west\""),
		policy("c", "import \"encoding/json\"\nschema: ctx: close({raw: json.Valid})\noutput: ctx: raw: \"{}\""),
	})
	// The template has its own strings import.
	c := custom(t, v, "import \"strings\"\n_up: strings.ToUpper(\"a\")")
	for value, ok := range map[string]bool{
		`{team: "abc", zone: "eu-west", raw: "{}"}`: true,
		`{team: "ab"}`: false,
		`{zone: "us"}`: false,
		`{raw: "{"}`:   false,
	} {
		u := c.Unify(c.Context().CompileString(value))
		require.Equal(t, ok, u.Validate() == nil, value)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	policies := []Policy{
		policy("a", "schema: ctx: close({zeta: string, alpha: int, mid?: bool})\noutput: ctx: {zeta: \"z\", alpha: 1}"),
		policy("b", "import \"strings\"\nschema: ctx: close({beta: strings.MinRunes(1)})\noutput: ctx: beta: \"b\""),
	}
	i1, c1 := Build(policies).Source()
	for range 20 {
		i2, c2 := Build(policies).Source()
		require.Equal(t, i1, i2)
		require.Equal(t, c1, c2)
	}
	require.Less(t, strings.Index(c1, "alpha"), strings.Index(c1, "zeta"))
}

func TestEmptyViewIsClosed(t *testing.T) {
	v := Build(nil)
	require.False(t, accepts(t, v, `{team: "a"}`))
	require.True(t, accepts(t, v, `{}`))
}

func TestPathOptional(t *testing.T) {
	v := Build([]Policy{
		policy("identity", "schema: ctx: close({team: string, mesh?: string, org: close({unit: string, zone?: string, deep?: {x: string}})})\noutput: ctx: {team: \"a\", org: unit: \"u\"}"),
		policy("later", "config: enabled: parameter.on\nschema: ctx: close({tier: string})\noutput: ctx: tier: \"t\""),
	})
	cases := map[string]struct {
		path     []string
		policy   string
		optional bool
	}{
		"a required key":                 {[]string{"team"}, "identity", false},
		"an optional key":                {[]string{"mesh"}, "identity", true},
		"a required nested key":          {[]string{"org", "unit"}, "identity", false},
		"an optional nested key":         {[]string{"org", "zone"}, "identity", true},
		"beneath an optional nested key": {[]string{"org", "deep", "x"}, "identity", true},
		"a conditional policy's key":     {[]string{"tier"}, "later", true},
		"an undeclared key":              {[]string{"region"}, "", false},
		"an undeclared nested key":       {[]string{"org", "region"}, "", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			by, optional := v.PathOptional(tc.path)
			require.Equal(t, tc.optional, optional)
			require.Equal(t, tc.policy, by)
		})
	}
}

func TestSourceWithoutImports(t *testing.T) {
	v := Build([]Policy{policy("p", "import \"strings\"\nschema: ctx: close({team: strings.MinRunes(3), zone: =~\"^eu-\"})\noutput: ctx: {team: \"abc\", zone: \"eu-1\"}")})
	ctx := v.SourceWithoutImports()
	c := cuecontext.New().CompileString(ctx).LookupPath(cue.ParsePath("context.custom"))
	require.NoError(t, c.Err(), ctx)
	check := func(value string) bool { return c.Unify(c.Context().CompileString(value)).Validate() == nil }
	require.True(t, check(`{team: "a", zone: "eu-1"}`), "a key typed with an import is left unchecked")
	require.False(t, check(`{zone: "us"}`), "a key typed without one is still checked")
	require.False(t, check(`{other: 1}`), "the view stays closed")
}

func TestLoosen(t *testing.T) {
	v := Build([]Policy{policy("p", "schema: ctx: close({team: =~\"^[a-z]+$\", zone: string})\noutput: ctx: {team: \"a\", zone: \"z\"}")})
	loose := v.Loosen("team")
	require.True(t, accepts(t, loose, `{team: 1, zone: "z"}`))
	require.False(t, accepts(t, loose, `{zone: 1}`))
	require.False(t, accepts(t, loose, `{other: 1}`))
	require.False(t, accepts(t, v, `{team: 1}`), "the original view is unchanged")
}

// A loosened key uses no import; a source naming it anyway would not
// compile.
func TestLoosenDropsImportsNoLongerUsed(t *testing.T) {
	v := Build([]Policy{policy("p", "import \"strings\"\nschema: ctx: close({team: strings.MinRunes(3), zone: string})\noutput: ctx: {team: \"abc\", zone: \"z\"}")})
	imports, _ := v.Loosen("team").Source()
	require.Empty(t, imports)
	require.True(t, accepts(t, v.Loosen("team"), `{team: "a", zone: "z"}`))
}

// A key marked required with ! is as certain as an unmarked one.
func TestBuildRequiredMarker(t *testing.T) {
	v := Build([]Policy{policy("p", "schema: ctx: close({team!: string, org: close({unit!: string})})\noutput: ctx: {team: \"a\", org: unit: \"u\"}")})
	_, optional := v.Optional("team")
	require.False(t, optional)
	_, optional = v.PathOptional([]string{"org", "unit"})
	require.False(t, optional)
	require.True(t, accepts(t, v, `{team: "a"}`))
	require.False(t, accepts(t, v, `{team: 1}`))
}
