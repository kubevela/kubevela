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
	"testing"

	"github.com/stretchr/testify/require"
)

// The marker is honoured in both schema: and output:, so a definition written
// either way redacts. Silently exposing a value because the marker sat in the
// other block is the failure this guards.
func TestExtractSensitiveOutputPathsReadsBothBlocks(t *testing.T) {
	for _, block := range []string{"schema", "output"} {
		t.Run(block, func(t *testing.T) {
			got := ExtractSensitiveOutputPaths(block + `: {
	host: string
	// +sensitive
	password: string
}`)
			require.Equal(t, []string{"password"}, got)
		})
	}
}

func TestExtractSensitiveOutputPathsDescendsAndDedupes(t *testing.T) {
	got := ExtractSensitiveOutputPaths(`
schema: {
	db: {
		host: string
		// +sensitive
		password: string
		tls: {
			// +sensitive
			key: string
		}
	}
	// +sensitive
	token: string
}
output: {
	// the same path again, from the other block
	// +sensitive
	token: string
}`)
	require.ElementsMatch(t, []string{"db.password", "db.tls.key", "token"}, got)
	require.Len(t, got, 3, "a path marked in both blocks is recorded once")
}

// A quoted label is a field name like any other. Missing it would leave the
// value unmasked while the definition plainly asked for it.
func TestExtractSensitiveOutputPathsHandlesQuotedLabels(t *testing.T) {
	got := ExtractSensitiveOutputPaths(`
schema: {
	// +sensitive
	"api-key": string
	"nested": {
		// +sensitive
		"inner-secret": string
	}
}`)
	require.ElementsMatch(t, []string{"api-key", "nested.inner-secret"}, got)
}

func TestExtractSensitiveOutputPathsOnTemplatesWithNothingToFind(t *testing.T) {
	require.Nil(t, ExtractSensitiveOutputPaths(``))
	require.Nil(t, ExtractSensitiveOutputPaths(`this is not cue {{{`),
		"an unparseable template yields nothing rather than panicking")
	require.Nil(t, ExtractSensitiveOutputPaths(`schema: {host: string}`),
		"no marker, no paths")
	require.Nil(t, ExtractSensitiveOutputPaths(`template: {output: {password: string}}`),
		"only top-level schema: and output: are scanned")
	require.Nil(t, ExtractSensitiveOutputPaths(`schema: string`),
		"a schema that is not a struct has no fields to mark")
}

// The marker has to be on the field, not merely somewhere in the file.
func TestExtractSensitiveOutputPathsNeedsTheMarkerOnTheField(t *testing.T) {
	got := ExtractSensitiveOutputPaths(`
// +sensitive
schema: {
	host: string
}`)
	require.Nil(t, got, "a marker on the block does not mark every field in it")
}

func TestMaskedPathCoversDescendantsButNotSiblings(t *testing.T) {
	masks := map[string]struct{}{"db.password": {}, "token": {}}
	require.True(t, MaskedPath("db.password", masks), "the marked path itself")
	require.True(t, MaskedPath("db.password.inner", masks), "anything beneath it")
	require.True(t, MaskedPath("token", masks))
	require.False(t, MaskedPath("db.host", masks), "a sibling is not masked")
	require.False(t, MaskedPath("db", masks), "the parent is not masked by a child")
	require.False(t, MaskedPath("tokenish", masks), "a prefix that is not a path segment")
	require.False(t, MaskedPath("db.password", nil))
}

// A read of a whole struct or list carries values whose own paths sit below the
// mark, so redaction has to descend into what was substituted.
func TestRedactValueDescendsIntoSubstitutedCollections(t *testing.T) {
	masks := map[string]struct{}{"db.password": {}, "members.token": {}}

	whole := RedactValue("db", map[string]interface{}{
		"host": "db.internal", "password": "hunter2",
	}, masks)
	require.Equal(t, map[string]interface{}{"host": "db.internal", "password": "***"}, whole)

	list := RedactValue("members", []interface{}{
		map[string]interface{}{"name": "ana", "token": "t-secret"},
		map[string]interface{}{"name": "bo", "token": "t-other"},
	}, masks)
	require.Equal(t, []interface{}{
		map[string]interface{}{"name": "ana", "token": "***"},
		map[string]interface{}{"name": "bo", "token": "***"},
	}, list)
}

func TestRedactValueLeavesUnmarkedValuesAlone(t *testing.T) {
	require.Equal(t, "plain", RedactValue("host", "plain", map[string]struct{}{"password": {}}))
	require.Equal(t, "plain", RedactValue("host", "plain", nil),
		"no marks, no work")
	require.Equal(t, 8080, RedactValue("port", 8080, map[string]struct{}{"password": {}}))
	require.Equal(t, "***", RedactValue("password", "hunter2", map[string]struct{}{"password": {}}))
}

func TestJoinMaskPath(t *testing.T) {
	require.Equal(t, "db", joinMaskPath("", "db"))
	require.Equal(t, "db.password", joinMaskPath("db", "password"))
}

// An expression reading through a list index produces a path with the index in
// it - "members.0.token" - while the mark from the schema is "members.token".
// The recorded read went into status unredacted.
func TestMaskedPathTreatsListIndicesAsTransparent(t *testing.T) {
	masks := map[string]struct{}{"members.token": {}}
	require.True(t, MaskedPath("members.0.token", masks))
	require.True(t, MaskedPath("members.12.token", masks))
	require.True(t, MaskedPath("members.0.token.value", masks))
	// A named field is not an index, so it must not collapse.
	require.False(t, MaskedPath("members.other.token", masks))
	require.False(t, MaskedPath("members.token2", masks))
}

// A marker written inside a list element was dropped entirely, because the walk
// descended into structs but not into the elements of a list.
func TestExtractSensitiveOutputPathsDescendsIntoLists(t *testing.T) {
	template := `
schema: {
	members: [...{
		name: string
		// +sensitive
		token: string
	}]
}
`
	require.Equal(t, []string{"members.token"}, ExtractSensitiveOutputPaths(template))
}
