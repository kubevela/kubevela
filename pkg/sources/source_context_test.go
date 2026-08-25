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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
)

func TestSplitIndexed(t *testing.T) {
	for _, tc := range []struct {
		in           string
		field, index string
		indexed      bool
	}{
		{"cluster", "cluster", "", false},
		{"appLabels[team]", "appLabels", "team", true},
		{"appAnnotations[a.b/c]", "appAnnotations", "a.b/c", true},
		// A key may itself contain a bracket; the split is on the first [ and the
		// final ], so the whole key survives.
		{"appLabels[a[b]]", "appLabels", "a[b]", true},
		{"appLabels[]", "appLabels", "", true},
		// Unbalanced is not an index, and must stay a field name rather than
		// becoming a lookup against a key nobody wrote.
		{"appLabels[team", "appLabels[team", "", false},
		{"appLabels]", "appLabels]", "", false},
	} {
		t.Run(tc.in, func(t *testing.T) {
			f, i, ok := splitIndexed(tc.in)
			require.Equal(t, tc.field, f)
			require.Equal(t, tc.index, i)
			require.Equal(t, tc.indexed, ok)
		})
	}
}

// Absent and present-but-empty are different answers, and the identity has to
// draw the line: a template may branch on it.
func TestLookupIndexSeparatesAbsentFromEmpty(t *testing.T) {
	require.Equal(t, "blue", lookupIndex(map[string]string{"team": "blue"}, "team"))
	require.Equal(t, "blue", lookupIndex(map[string]interface{}{"team": "blue"}, "team"))
	require.Equal(t, "", lookupIndex(map[string]string{"team": ""}, "team"),
		"present but empty is an empty string")
	require.Nil(t, lookupIndex(map[string]string{"other": "x"}, "team"),
		"absent is nil, which is not the same answer")
	require.Nil(t, lookupIndex(nil, "team"))
	require.Nil(t, lookupIndex("not a map", "team"))
	require.Nil(t, lookupIndex(map[string]int{"team": 1}, "team"),
		"a map this cannot read contributes nothing rather than guessing")
}

func TestIdentityContextGathersOnlyWhatKeyInputsName(t *testing.T) {
	values := map[string]interface{}{
		"cluster":   "eu-west",
		"namespace": "prod",
		"appLabels": map[string]string{"team": "blue", "empty": ""},
	}

	got := identityContext(values, "cfg", []string{"cluster", "appLabels[team]"})
	require.Equal(t, map[string]interface{}{
		"cluster":   "eu-west",
		"appLabels": map[string]interface{}{"team": "blue"},
	}, got, "namespace is not keyed, so it must not reach the identity")
}

func TestIdentityContextIsNilWithoutInputs(t *testing.T) {
	require.Nil(t, identityContext(map[string]interface{}{"cluster": "x"}, "cfg", nil),
		"a source keyed on nothing has one entry, so there is no identity to build")
	require.Nil(t, identityContext(nil, "cfg", []string{}))
}

// context.name is the binding, not whatever the caller happened to be called.
// Without this a source read by two components would key on the component and
// get an entry each.
func TestIdentityContextNameIsTheBinding(t *testing.T) {
	got := identityContext(map[string]interface{}{velaprocess.ContextName: "web"},
		"registry-lookup", []string{velaprocess.ContextName})
	require.Equal(t, map[string]interface{}{velaprocess.ContextName: "registry-lookup"}, got)
}

// An absent field still contributes, as nil. Dropping it would make "no label"
// and "label set to empty" share one cache entry.
func TestIdentityContextKeepsAbsentAndEmptyApart(t *testing.T) {
	absent := identityContext(map[string]interface{}{"appLabels": map[string]string{}},
		"cfg", []string{"appLabels[team]"})
	empty := identityContext(map[string]interface{}{"appLabels": map[string]string{"team": ""}},
		"cfg", []string{"appLabels[team]"})
	require.NotEqual(t, identityJSON(t, absent), identityJSON(t, empty),
		"absent and empty must not produce the same cache key")

	missingField := identityContext(map[string]interface{}{}, "cfg", []string{"cluster"})
	require.Equal(t, map[string]interface{}{"cluster": nil}, missingField)
}

// Two indexed reads of one field share a nested map rather than overwriting.
func TestIdentityContextKeepsEveryIndexOfAField(t *testing.T) {
	got := identityContext(
		map[string]interface{}{"appLabels": map[string]string{"team": "blue", "tier": "gold"}},
		"cfg", []string{"appLabels[team]", "appLabels[tier]"})
	require.Equal(t, map[string]interface{}{
		"appLabels": map[string]interface{}{"team": "blue", "tier": "gold"},
	}, got)
}

// The rules make a field either bare or indexed, never both, so these orderings
// are unreachable today. They are pinned because the failure is silent: whichever
// read arrived second would replace the first and the identity would stop
// telling entries apart.
func TestIdentityContextDoesNotLoseAContributionEitherOrder(t *testing.T) {
	values := map[string]interface{}{"appLabels": map[string]string{"team": "blue"}}

	indexedFirst := identityContext(values, "cfg", []string{"appLabels[team]", "appLabels"})
	bareFirst := identityContext(values, "cfg", []string{"appLabels", "appLabels[team]"})

	for name, got := range map[string]map[string]interface{}{
		"indexed first": indexedFirst, "bare first": bareFirst,
	} {
		nested, ok := got["appLabels"].(map[string]interface{})
		require.True(t, ok, "%s: the field should hold both reads", name)
		require.Equal(t, "blue", nested["team"], "%s: the indexed read survives", name)
		require.Contains(t, nested, wholeFieldKey, "%s: the bare read survives too", name)
	}
}

func identityJSON(t *testing.T, m map[string]interface{}) string {
	t.Helper()
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	return string(raw)
}

// The rendered context is CUE the template unifies with, so its shape matters as
// much as its contents.
func TestSourceContextFileRendersOnlyTheAllowedFields(t *testing.T) {
	values := map[string]interface{}{
		"cluster":               "eu-west",
		"namespace":             "prod",
		"secretish":             "should not appear",
		velaprocess.ContextName: "web",
	}

	out, err := sourceContextFile(values, "registry-lookup",
		[]string{"cluster", "namespace", velaprocess.ContextName})
	require.NoError(t, err)
	require.True(t, len(out) > len("context: "))
	require.Equal(t, "context: ", out[:9], "the template unifies with a context: field")

	var got struct {
		Cluster   string `json:"cluster"`
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Secretish string `json:"secretish"`
	}
	require.NoError(t, json.Unmarshal([]byte(out[len("context: "):]), &got))
	require.Equal(t, "eu-west", got.Cluster)
	require.Equal(t, "prod", got.Namespace)
	require.Equal(t, "registry-lookup", got.Name,
		"name comes from the binding, never from the component that read it")
	require.Empty(t, got.Secretish, "a field the rules do not list must not be rendered")
}

// A nil value is dropped rather than rendered as null: the template declares
// these as strings, and unifying a declared string with null fails the render.
func TestSourceContextFileDropsAbsentFields(t *testing.T) {
	out, err := sourceContextFile(map[string]interface{}{"cluster": nil}, "cfg", []string{"cluster"})
	require.NoError(t, err)
	require.NotContains(t, out, "null")
}

func TestSourceContextFileFailsOnUnrenderableValue(t *testing.T) {
	_, err := sourceContextFile(map[string]interface{}{"cluster": make(chan int)}, "cfg", []string{"cluster"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "rendering source context")
}

// An unrecognised surface must offer everything the rules allow. Failing closed
// would silently strip the context from any caller that forgot to name itself.
func TestAvailableFieldsFailsOpenOnAnUnknownSurface(t *testing.T) {
	fields := []string{"cluster", "namespace", velaprocess.ContextName}
	require.Equal(t, fields, availableFields(fields, ""))
	require.Equal(t, fields, availableFields(fields, "not-a-surface"))
}

// Every field the rules key on is offered by every surface that resolves a
// source, so narrowing removes nothing today. context.name is kept regardless,
// since it comes from the binding rather than the caller.
func TestAvailableFieldsKeepsTheKeyedFieldsOnEverySurface(t *testing.T) {
	for _, surface := range ConsumableSurfaces {
		got := availableFields([]string{"cluster", velaprocess.ContextName}, surface)
		require.Contains(t, got, velaprocess.ContextName, "%s: name is always available", surface)
		require.Contains(t, got, "cluster", "%s: cluster is keyed, so it must be offered", surface)
	}
}

func TestSourceContextUsesTheCurrentRules(t *testing.T) {
	out, err := sourceContext(map[string]interface{}{"cluster": "eu-west"}, "cfg", SurfaceComponent)
	require.NoError(t, err)
	require.Contains(t, out, `"name":"cfg"`)
	require.Contains(t, out, `"cluster":"eu-west"`)
}
