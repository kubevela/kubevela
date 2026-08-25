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
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

func schemaFor(t *testing.T, src string) *cueStruct {
	t.Helper()
	v := cuecontext.New().CompileString(src)
	require.NoError(t, v.Err())
	return &cueStruct{root: v}
}

// A parameter path is resolved the same way whether it lands on a plain field, an
// optional one, a list position or a key of an open map. Missing any of these
// makes a legitimate read look undeclared.
func TestCueStructLookupResolvesEveryShape(t *testing.T) {
	s := schemaFor(t, `{
	host: string
	note?: string
	headers?: [string]: string
	items: [{name: string}]
	nested: {deep: {value: int}}
}`)

	for _, tc := range []struct {
		path  string
		found bool
		why   string
	}{
		{"host", true, "a plain field"},
		{"note", true, "an optional field is declared, just not promised"},
		{"headers.anything", true, "an open map declares a value type at every key"},
		{"items.0.name", true, "a list position the schema pins"},
		{"nested.deep.value", true, "nested structs"},
		{"nope", false, "a field the schema does not declare"},
		{"host.deeper", false, "a scalar has nothing beneath it"},
		{"items.5.name", false, "a position a one-element list does not have"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			_, ok := s.valueAt(segs(tc.path))
			require.Equal(t, tc.found, ok, tc.why)
		})
	}
}

func TestCueStructKindAt(t *testing.T) {
	s := schemaFor(t, `{
	host: string
	port: int
	ratio: float
	secure: bool
	tags: [...string]
	meta: {region: string}
}`)
	for path, want := range map[string]cue.Kind{
		"host": cue.StringKind, "port": cue.IntKind, "ratio": cue.FloatKind,
		"secure": cue.BoolKind, "tags": cue.ListKind, "meta": cue.StructKind,
	} {
		got, ok := s.kindAt(segs(path))
		require.True(t, ok, path)
		require.Equal(t, want, got, path)
	}
	_, ok := s.kindAt(segs("absent"))
	require.False(t, ok)
}

// Required means present, not optional, and with no default to fall back on.
// Each of the three is a separate reason a value need not be supplied.
func TestCueStructRequiredAt(t *testing.T) {
	s := schemaFor(t, `{
	host: string
	note?: string
	port: *8080 | int
	nested: {deep: string, opt?: string}
	items: [{name: string}]
}`)

	require.True(t, s.requiredAt(segs("host")))
	require.True(t, s.requiredAt(segs("nested.deep")))

	require.False(t, s.requiredAt(segs("note")), "optional")
	require.False(t, s.requiredAt(segs("port")), "defaulted")
	require.False(t, s.requiredAt(segs("nested.opt")), "optional, nested")
	require.False(t, s.requiredAt(segs("absent")), "not declared at all")
	require.False(t, s.requiredAt(segs("")), "an empty path names nothing")
	require.False(t, s.requiredAt(segs("nested.")), "a trailing separator names nothing")
	require.False(t, s.requiredAt(segs("items.0")), "a list index is a position, not a named field")
	require.False(t, s.requiredAt(segs("nope.deep")), "a path whose parent is absent")
}

// kindName is what an author reads in a type-mismatch message, so every kind
// they can write has to render as the word they wrote.
func TestKindNameRendersWhatAnAuthorWrote(t *testing.T) {
	for kind, want := range map[cue.Kind]string{
		cue.StringKind: "string",
		cue.IntKind:    "int",
		cue.NumberKind: "number",
		cue.FloatKind:  "number",
		cue.BoolKind:   "bool",
		cue.StructKind: "object",
		cue.ListKind:   "list",
		cue.NullKind:   "null",
	} {
		require.Equal(t, want, kindName(kind))
	}
	require.NotEmpty(t, kindName(cue.BytesKind),
		"a kind with no friendly name still renders as something")
}

// segs addresses a path the way the walker does, so a test can keep writing the
// readable dotted form.
func segs(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, ".")
}
