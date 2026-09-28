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

package cuetest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadAndRun(t *testing.T) {
	r := require.New(t)
	suites, err := Load("testdata/defs")
	r.NoError(err)

	got := map[string][]string{}
	for _, s := range suites {
		for _, c := range s.Cases {
			if !c.Pending {
				got[filepath.Base(s.File)+"/"+c.Name] = c.Run()
			}
		}
	}
	statusErr := got["web_test.cue/status errors before any status exists"]
	r.Len(statusErr, 1)
	r.Contains(statusErr[0], "error: unexpected error: ")
	r.Contains(statusErr[0], "readyReplicas")
	delete(got, "web_test.cue/status errors before any status exists")

	r.Equal(map[string][]string{
		"scaler_test.cue/patches replicas":               nil,
		"scaler_test.cue/healthy once scaled":            nil,
		"scaler_test.cue/unhealthy below target":         nil,
		"web_test.cue/defaults to one replica":           nil,
		"web_test.cue/exposes a service":                 nil,
		"web_test.cue/rejects a numeric image":           nil,
		"web_test.cue/healthy once ready":                nil,
		"web_test.cue/unhealthy while rolling out":       nil,
		"web_test.cue/fails on purpose":                  {"output.spec.replicas: expected 2, got 1"},
		"web_test.cue/closed on purpose":                 {"output.spec.template: unexpected field, got {\n\tspec: {\n\t\tcontainers: [{\n\t\t\timage: \"nginx\"\n\t\t}]\n\t}\n}"},
		"web_test.cue/expects an error that never comes": {`error: expected an error matching =~"image", got none`},
	}, got)
}

func TestLoadSingleFile(t *testing.T) {
	suites, err := Load("testdata/defs/scaler_test.cue")
	require.NoError(t, err)
	require.Len(t, suites, 1)
	c := suites[0].Cases[0]
	require.Equal(t, "patches replicas", c.Name)
	require.Equal(t, TestTraitRender, c.Test)
	require.Equal(t, "scaler", c.Subject.Name)
	require.Equal(t, "testdata/defs/scaler_test.cue", c.File)
	require.Equal(t, 5, c.Line)
	require.Len(t, suites[0].Cases, 4, "hidden fields are not cases")
}

func TestLoadDefinitionPaths(t *testing.T) {
	const def = "d: type: \"component\"\ntemplate: {output: {kind: \"X\"}, parameter: {}}"
	cases := map[string]struct {
		testFile   string
		definition string
	}{
		"bare name beside the test":   {"a_test.cue", "d"},
		"explicit .cue":               {"a_test.cue", "d.cue"},
		"relative path, .cue omitted": {"tests/a_test.cue", "../d"},
		"relative path with .cue":     {"tests/a_test.cue", "../d.cue"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "d.cue", def)
			write(t, dir, "broken.cue", "this is not { valid cue")
			write(t, dir, tc.testFile, "import \"vela/test\"\nx: test.#ComponentRender & {definition: \""+tc.definition+"\", expect: output: kind: \"X\"}")
			suites, err := Load(filepath.Join(dir, tc.testFile))
			require.NoError(t, err, "a broken sibling .cue does not matter")
			c := suites[0].Cases[0]
			require.Equal(t, "d", c.Subject.Name, "the name comes from the file")
			require.Empty(t, c.Run())
		})
	}
}

func TestLoadErrors(t *testing.T) {
	const component = "d: type: \"component\"\ntemplate: {output: {}, parameter: {}}"
	cases := map[string]struct {
		files map[string]string
		err   string
	}{
		"missing definition file": {
			files: map[string]string{"a_test.cue": "import \"vela/test\"\nx: test.#ComponentRender & {definition: \"nope\", expect: {}}"},
			err:   `definition "nope": nope.cue does not exist`,
		},
		"misspelt field": {
			files: map[string]string{"d.cue": component, "a_test.cue": "import \"vela/test\"\nx: test.#ComponentRender & {definition: \"d\", expectt: {}}"},
			err:   "expectt: field not allowed",
		},
		"wrong function for the definition kind": {
			files: map[string]string{"d.cue": component, "a_test.cue": "import \"vela/test\"\nx: test.#TraitRender & {definition: \"d\", workload: {}, expect: {}}"},
			err:   `"d" is a component definition, so #TraitRender cannot test it`,
		},
		"non-concrete parameter": {
			files: map[string]string{"d.cue": component, "a_test.cue": "import \"vela/test\"\nx: test.#ComponentRender & {definition: \"d\", parameter: a: int, expect: {}}"},
			err:   "parameter must be concrete",
		},
		"no cases": {
			files: map[string]string{"a_test.cue": `tests: {definition: "d", cases: {}}`},
			err:   `no cases: declare each with a function from "vela/test"`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for f, src := range tc.files {
				write(t, dir, f, src)
			}
			suites, err := Load(dir)
			require.NoError(t, err, "a file that fails to load is reported on its suite")
			require.Len(t, suites, 1)
			require.Empty(t, suites[0].Cases)
			require.ErrorContains(t, suites[0].Err, tc.err)
		})
	}
}

func TestLoadKeepsGoingPastABrokenFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "d.cue", "d: type: \"component\"\ntemplate: {output: {}, parameter: {}}")
	write(t, dir, "a_test.cue", "import \"vela/test\"\nx: test.#ComponentRender & {definition: \"d\", expect: {}}")
	write(t, dir, "b_test.cue", "tests: {}")
	suites, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, suites, 2)
	require.NoError(t, suites[0].Err)
	require.Len(t, suites[0].Cases, 1)
	require.ErrorContains(t, suites[1].Err, "no cases")
}

func TestLoadPathErrors(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope"))
	require.ErrorContains(t, err, "no such file")
	dir := t.TempDir()
	write(t, dir, "d.cue", "d: type: \"component\"")
	_, err = Load(dir)
	require.ErrorContains(t, err, "no *_test.cue files")
}

func write(t *testing.T, dir, file, src string) {
	t.Helper()
	path := filepath.Join(dir, file)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))
}

func TestLabels(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "d.cue", "d: type: \"component\"\ntemplate: {output: {}, parameter: {}}")
	write(t, dir, "a_test.cue", `import "vela/test"

@label(file-wide)

_base: {
	@label(from-base)
	definition: "d"
}

"field and decl": test.#ComponentStatus & {
	@label(env:prod)
	definition: "d"
	expect: {}
} @label(slow, smoke)

"inherited": test.#ComponentRender & _base & {expect: {}}

"automatic only": test.#ComponentRender & {definition: "d", expect: {}} @other(ignored)
`)
	suites, err := Load(dir)
	require.NoError(t, err)
	require.NoError(t, suites[0].Err)
	got := map[string][]string{}
	for _, c := range suites[0].Cases {
		got[c.Name] = c.Labels
	}
	require.Equal(t, map[string][]string{
		"field and decl": {"component", "env:prod", "file-wide", "slow", "smoke", "status"},
		"inherited":      {"component", "file-wide", "from-base", "render"},
		"automatic only": {"component", "file-wide", "render"},
	}, got)
}

func TestLabelsInvalid(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "d.cue", "d: type: \"component\"\ntemplate: {output: {}, parameter: {}}")
	write(t, dir, "a_test.cue", "import \"vela/test\"\nx: test.#ComponentRender & {definition: \"d\", expect: {}} @label(a&b)")
	suites, err := Load(dir)
	require.NoError(t, err)
	require.ErrorContains(t, suites[0].Err, `case "x": invalid label "a&b"`)
}

func TestPending(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "d.cue", "d: type: \"component\"\ntemplate: {output: {}, parameter: {}}")
	write(t, dir, "a_test.cue", `import "vela/test"

_parked: {
	@pending(parked in base)
	definition: "d"
}

"after":   test.#ComponentRender & {definition: "d", expect: {}} @pending(gateway reads loadbalancer)
"inside":  test.#ComponentRender & {@pending(), definition: "d", expect: {}}
"via base": test.#ComponentRender & _parked & {expect: {}}
"runs":    test.#ComponentRender & {definition: "d", expect: {}}
`)
	write(t, dir, "b_test.cue", "import \"vela/test\"\n@pending(whole file)\nx: test.#ComponentRender & {definition: \"d\", expect: {}}")
	suites, err := Load(dir)
	require.NoError(t, err)
	type p struct {
		pending bool
		reason  string
	}
	got := map[string]p{}
	for _, s := range suites {
		require.NoError(t, s.Err)
		for _, c := range s.Cases {
			got[c.Name] = p{c.Pending, c.PendingReason}
		}
	}
	require.Equal(t, map[string]p{
		"after":    {true, "gateway reads loadbalancer"},
		"inside":   {true, ""},
		"via base": {true, "parked in base"},
		"runs":     {false, ""},
		"x":        {true, "whole file"},
	}, got)
}

func TestSharedExpectations(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "d.cue", `d: type: "component"
template: {
	output: {kind: "X", metadata: {name: context.name, labels: a: "b"}, spec: {replicas: 1, paused: false}, items: [{p: 1}, {p: 2}]}
	parameter: {}
}`)
	write(t, dir, "a_test.cue", `import "vela/test"

_spec: expect: output: spec: replicas: 1
_exactSpec: expect: output: spec: {replicas: 1} @exact()
_notPaused: expect: output: spec: paused: true @not()
_hasItem: expect: output: items: [{p: 2}] @contains()

"shared subset":          test.#ComponentRender & _spec & {definition: "d"}
"shared plus own":        test.#ComponentRender & _spec & {definition: "d", expect: output: kind: "X"}
"exact written directly": test.#ComponentRender & {definition: "d", expect: output: spec: {replicas: 1} @exact()}
"exact inherited":        test.#ComponentRender & _exactSpec & {definition: "d"}
"exact satisfied":        test.#ComponentRender & {definition: "d", expect: output: spec: {replicas: 1, paused: false} @exact()}
"not inherited":          test.#ComponentRender & _notPaused & {definition: "d"}
"contains inherited":     test.#ComponentRender & _hasItem & {definition: "d"}
`)
	suites, err := Load(dir)
	require.NoError(t, err)
	require.NoError(t, suites[0].Err)
	got := map[string][]string{}
	for _, c := range suites[0].Cases {
		got[c.Name] = c.Run()
	}
	require.Equal(t, map[string][]string{
		"shared subset":          nil,
		"shared plus own":        nil,
		"exact written directly": {"output.spec.paused: unexpected field, got false"},
		"exact inherited":        {"output.spec.paused: unexpected field, got false"},
		"exact satisfied":        nil,
		"not inherited":          nil,
		"contains inherited":     nil,
	}, got)
}

func TestAttachedTraits(t *testing.T) {
	suites, err := Load("testdata/traits")
	require.NoError(t, err)
	require.NoError(t, suites[0].Err)
	got := map[string][]string{}
	for _, c := range suites[0].Cases {
		got[c.Name] = c.Run()
	}
	require.Equal(t, map[string][]string{
		"traits patch the workload and add outputs":                nil,
		"a trait overrides a parameter default":                    nil,
		"a trait conflicts with a value the user set":              nil,
		"the same trait twice, with clashing outputs":              nil,
		"component and trait both healthy":                         nil,
		"trait unhealthy, component fine":                          nil,
		"observed a trait that is not attached":                    nil,
		"two sources render the same object":                       nil,
		"an unreadable context only matters when asked about":      nil,
		"asking about an unreadable context fails":                 {"context: reading context: json: error calling MarshalJSON for type cue.Value: cue: marshal error: outputs: cannot convert incomplete value \"_\" to JSON"},
		"context: a later trait's output replaces the component's": nil,
		"context is what the templates saw":                        nil,
	}, got)
}

func TestAttachedTraitErrors(t *testing.T) {
	cases := map[string]struct {
		traits string
		err    string
	}{
		"missing trait":           {`[{definition: "nope"}]`, `traits[0].definition "nope": nope.cue does not exist`},
		"a component as a trait":  {`[{definition: "d"}]`, `traits[0]: "d" is a component definition, not a trait`},
		"non-concrete parameters": {`[{definition: "t", parameter: x: int}]`, "traits[0].parameter must be concrete"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "d.cue", "d: type: \"component\"\ntemplate: {output: {apiVersion: \"v1\", kind: \"X\"}, parameter: {}}")
			write(t, dir, "t.cue", "t: type: \"trait\"\ntemplate: {parameter: x: int}")
			write(t, dir, "a_test.cue", "import \"vela/test\"\nx: test.#ComponentRender & {definition: \"d\", traits: "+tc.traits+", expect: {}}")
			suites, err := Load(dir)
			require.NoError(t, err)
			require.ErrorContains(t, suites[0].Err, tc.err)
		})
	}
}
