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

package propexpr

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func srcRef(path ...string) Reference { return Reference{Root: SourceIdent, Path: path} }
func ctxRef(path ...string) Reference { return Reference{Root: ContextIdent, Path: path} }

// A schema is a contract: a declared, non-optional field is always there, and
// demanding a default for it would be noise on every expression. A default is
// required exactly where the schema stops promising the value.
func TestUndefendedInNeedsADefaultOnlyWhereTheSchemaStopsPromising(t *testing.T) {
	schemas := map[string]string{
		"cfg": `{
	host: string
	note?: string
	network?: {vpcId: string}
	labels: [string]: string
	open: _
}`,
	}

	for _, tc := range []struct {
		name       string
		ref        Reference
		undefended bool
		why        string
	}{
		{"a declared field", srcRef("cfg", "host"), false,
			"the schema promises it, so a fallback would be noise"},
		{"an optional field", srcRef("cfg", "note"), true,
			"declared optional means it may simply not be there"},
		{"a field under an optional struct", srcRef("cfg", "network", "vpcId"), true,
			"vpcId is absent whenever network is, however required it looks inside"},
		{"a key of an open map", srcRef("cfg", "labels", "team"), true,
			"the map is declared, never the key"},
		{"a field the schema does not declare", srcRef("cfg", "nope"), false,
			"reported by the type check, which names it properly; not twice"},
		{"anything under an open field", srcRef("cfg", "open", "whatever"), false,
			"unknowable here, so it fails open rather than demanding a default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := UndefendedIn([]Reference{tc.ref}, schemas)
			require.NoError(t, err)
			if tc.undefended {
				require.Len(t, got, 1, tc.why)
			} else {
				require.Empty(t, got, tc.why)
			}
		})
	}
}

// A read that carries its own fallback survives the value being absent, which is
// the whole point of writing one.
func TestUndefendedInSkipsDefaultedReads(t *testing.T) {
	ref := srcRef("cfg", "note")
	ref.Defaulted = true
	got, err := UndefendedIn([]Reference{ref}, map[string]string{"cfg": `{note?: string}`})
	require.NoError(t, err)
	require.Empty(t, got)
}

// context has no schema, so the rule is structural: a plain field is always
// supplied, an indexed read is a lookup into an open map and may find nothing.
//
// An unguarded read of an absent label fails the render with "no such key", so
// admission has to catch it for the same reason it catches an optional source
// field. Judging only source reads leaves this one to blow up at render.
func TestUndefendedInJudgesContextStructurally(t *testing.T) {
	got, err := UndefendedIn([]Reference{ctxRef("cluster")}, nil)
	require.NoError(t, err)
	require.Empty(t, got, "a plain context field is always supplied")

	got, err = UndefendedIn([]Reference{ctxRef("appLabels", "team")}, nil)
	require.NoError(t, err)
	require.Len(t, got, 1, "a label may simply not be set")

	guarded := ctxRef("appLabels", "team")
	guarded.Defaulted = true
	got, err = UndefendedIn([]Reference{guarded}, nil)
	require.NoError(t, err)
	require.Empty(t, got, "a guarded read survives the label being absent")
}

// An unknown binding is reported by the type check, which names it properly.
// Complaining here as well would tell the author the same thing twice, in worse
// words.
func TestUndefendedInStaysQuietAboutAnUnknownBinding(t *testing.T) {
	got, err := UndefendedIn([]Reference{srcRef("nosuch", "field")}, map[string]string{})
	require.NoError(t, err)
	require.Empty(t, got)
}

// A schema that will not compile is not a reason to fail the expression check:
// the definition's own validation reports it.
func TestUndefendedInIgnoresASchemaThatWillNotCompile(t *testing.T) {
	got, err := UndefendedIn([]Reference{srcRef("cfg", "host")},
		map[string]string{"cfg": `{this is not cue`})
	require.NoError(t, err)
	require.Empty(t, got)
}

// A list index is a real position in the schema, and reading past the end is a
// read that may find nothing.
func TestUndefendedInHandlesListIndices(t *testing.T) {
	schemas := map[string]string{"cfg": `{items: [{name: string}]}`}

	got, err := UndefendedIn([]Reference{srcRef("cfg", "items", "0", "name")}, schemas)
	require.NoError(t, err)
	require.Empty(t, got, "position 0 is pinned by the schema")

	got, err = UndefendedIn([]Reference{srcRef("cfg", "items", "5", "name")}, schemas)
	require.NoError(t, err)
	require.Len(t, got, 1, "position 5 is not promised by a one-element list")
}

func TestIsIndexSegment(t *testing.T) {
	require.True(t, isIndexSegment("0"))
	require.True(t, isIndexSegment("42"))
	require.False(t, isIndexSegment(""))
	require.False(t, isIndexSegment("name"))
	require.False(t, isIndexSegment("1a"), "a field cannot start with a digit, so this is a name")
	require.False(t, isIndexSegment("-1"))
}

func TestIsCUEIdent(t *testing.T) {
	require.True(t, isCUEIdent("name"))
	require.True(t, isCUEIdent("_name"))
	require.True(t, isCUEIdent("$name"))
	require.True(t, isCUEIdent("n4me"))
	require.False(t, isCUEIdent(""))
	require.False(t, isCUEIdent("4name"), "cannot start with a digit")
	require.False(t, isCUEIdent("my-name"), "a hyphen needs bracket syntax")
	require.False(t, isCUEIdent("a.b"))
}

// The rendered form has to be an expression the author can paste, because the
// errors using it say "supply a default with *<ref> | <fallback>".
func TestReferenceStringRendersSomethingThatParses(t *testing.T) {
	for _, tc := range []struct {
		ref  Reference
		want string
	}{
		{srcRef("cfg", "host"), `source.cfg.host`},
		{srcRef("cfg", "items", "0", "name"), `source.cfg.items[0].name`},
		{srcRef("my-source", "host"), `source["my-source"].host`},
		{ctxRef("appLabels", "a.b/c"), `context.appLabels["a.b/c"]`},
		{ctxRef("cluster"), `context.cluster`},
	} {
		require.Equal(t, tc.want, tc.ref.String())
	}
}

func TestReferenceIsSource(t *testing.T) {
	require.True(t, srcRef("cfg").IsSource())
	require.False(t, ctxRef("cluster").IsSource())
}

// Context reads were judged by path length alone: anything below the first
// segment was treated as a lookup into an open map and demanded a fallback. But
// context.clusterVersion is a struct with declared fields, so a read of
// .major is as certain as a read of context.namespace.
func TestUndefendedInReadsContextShapeNotPathLength(t *testing.T) {
	undefended := func(path ...string) bool {
		out, err := UndefendedIn([]Reference{{Root: "context", Path: path}}, nil)
		require.NoError(t, err)
		return len(out) > 0
	}

	require.False(t, undefended("namespace"), "a plain field is always supplied")
	require.False(t, undefended("clusterVersion", "major"),
		"clusterVersion declares its fields, so a read of one is not absent-prone")
	require.False(t, undefended("clusterVersion", "gitVersion"))

	require.True(t, undefended("appLabels", "team"),
		"a key of an open map may find nothing")
	require.True(t, undefended("appAnnotations", "owner"))
}
