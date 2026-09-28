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
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cel/template"
	"github.com/stretchr/testify/require"
)

func srcRef(path ...string) template.Reference {
	return template.Reference{Root: SourceIdent, Path: path}
}
func ctxRef(path ...string) template.Reference {
	return template.Reference{Root: ContextIdent, Path: path}
}

// A schema is a contract: a declared, non-optional field is always there, and
// demanding a default for it would be noise on every expression. A default is
// required exactly where the schema stops promising the value.
func TestUndefendedNeedsADefaultOnlyWhereTheSchemaStopsPromising(t *testing.T) {
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
		ref        template.Reference
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
			got, err := undefendedIn([]template.Reference{tc.ref}, schemas)
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
func TestUndefendedSkipsDefaultedReads(t *testing.T) {
	ref := srcRef("cfg", "note")
	ref.Defaulted = true
	got, err := undefendedIn([]template.Reference{ref}, map[string]string{"cfg": `{note?: string}`})
	require.NoError(t, err)
	require.Empty(t, got)
}

// context has no schema, so the rule is structural: a plain field is always
// supplied, an indexed read is a lookup into an open map and may find nothing.
//
// An unguarded read of an absent label fails the render with "no such key", so
// admission has to catch it for the same reason it catches an optional source
// field. Judging only source reads leaves this one to blow up at render.
func TestUndefendedJudgesContextStructurally(t *testing.T) {
	got, err := undefendedIn([]template.Reference{ctxRef("cluster")}, nil)
	require.NoError(t, err)
	require.Empty(t, got, "a plain context field is always supplied")

	got, err = undefendedIn([]template.Reference{ctxRef("appLabels", "team")}, nil)
	require.NoError(t, err)
	require.Len(t, got, 1, "a label may simply not be set")

	guarded := ctxRef("appLabels", "team")
	guarded.Defaulted = true
	got, err = undefendedIn([]template.Reference{guarded}, nil)
	require.NoError(t, err)
	require.Empty(t, got, "a guarded read survives the label being absent")
}

// An unknown binding is reported by the type check, which names it properly.
// Complaining here as well would tell the author the same thing twice, in worse
// words.
func TestUndefendedStaysQuietAboutAnUnknownBinding(t *testing.T) {
	got, err := undefendedIn([]template.Reference{srcRef("nosuch", "field")}, map[string]string{})
	require.NoError(t, err)
	require.Empty(t, got)
}

// A schema that will not compile is not a reason to fail the expression check:
// the definition's own validation reports it.
func TestUndefendedIgnoresASchemaThatWillNotCompile(t *testing.T) {
	got, err := undefendedIn([]template.Reference{srcRef("cfg", "host")},
		map[string]string{"cfg": `{this is not cue`})
	require.NoError(t, err)
	require.Empty(t, got)
}

// A list index is a real position in the schema, and reading past the end is a
// read that may find nothing.
func TestUndefendedHandlesListIndices(t *testing.T) {
	schemas := map[string]string{"cfg": `{items: [{name: string}]}`}

	got, err := undefendedIn([]template.Reference{srcRef("cfg", "items", "0", "name")}, schemas)
	require.NoError(t, err)
	require.Empty(t, got, "position 0 is pinned by the schema")

	got, err = undefendedIn([]template.Reference{srcRef("cfg", "items", "5", "name")}, schemas)
	require.NoError(t, err)
	require.Len(t, got, 1, "position 5 is not promised by a one-element list")
}

func TestReferenceIsSource(t *testing.T) {
	require.True(t, IsSource(srcRef("cfg")))
	require.False(t, IsSource(ctxRef("cluster")))
}

// Context reads were judged by path length alone: anything below the first
// segment was treated as a lookup into an open map and demanded a fallback. But
// context.clusterVersion is a struct with declared fields, so a read of
// .major is as certain as a read of context.namespace.
func TestUndefendedReadsContextShapeNotPathLength(t *testing.T) {
	undefended := func(path ...string) bool {
		out, err := undefendedIn([]template.Reference{{Root: "context", Path: path}}, nil)
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

// A component read waits for its value rather than failing on its absence, so
// it never needs a default, whatever its path looks like.
func TestUndefendedNeverFlagsAComponentRead(t *testing.T) {
	got, err := undefendedIn([]template.Reference{{Root: ComponentIdent, Path: []string{"appLabels", "team"}}}, nil)
	require.NoError(t, err)
	require.Empty(t, got)
}

// undefendedIn is the has()-default rule over a set of reads: those that may be
// absent and carry no guard.
func undefendedIn(refs []template.Reference, schemas map[string]string) ([]template.Reference, error) {
	compiled := CompileSchemas(schemas)
	var out []template.Reference
	for _, ref := range refs {
		if ref.Defaulted {
			continue
		}
		may, err := compiled.CanBeAbsent(ref)
		if err != nil {
			return nil, err
		}
		if may {
			out = append(out, ref)
		}
	}
	return out, nil
}

func TestSchemasKind(t *testing.T) {
	s := CompileSchemas(map[string]string{"cfg": `{n: number, ratio: float, port: int, items: [...{name: string}], labels: [string]: string, note?: string}`})
	for path, want := range map[string]cue.Kind{
		"n": cue.NumberKind, "ratio": cue.FloatKind, "port": cue.IntKind,
		"items.0.name": cue.StringKind, "labels.team": cue.StringKind, "note": cue.StringKind,
	} {
		got, ok := s.Kind(srcRef(append([]string{"cfg"}, strings.Split(path, ".")...)...))
		require.True(t, ok, path)
		require.Equal(t, want, got, path)
	}
	_, ok := s.Kind(srcRef("cfg", "undeclared"))
	require.False(t, ok)
	_, ok = s.Kind(template.Reference{Root: ComponentIdent, Path: []string{"db", "output"}})
	require.False(t, ok, "a component's live output has no schema")
}
