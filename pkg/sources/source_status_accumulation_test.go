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
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wfprocess "github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/pkg/cue/process"
)

// twoSourceContext is a render context carrying two bindings of the same type,
// which is the smallest setup that shows one pass discarding another's record.
func twoSourceContext(t *testing.T) wfprocess.Context {
	t.Helper()
	pCtx := process.NewContext(process.ContextData{
		Namespace: "default", CompName: "web", AppName: "app",
	})
	pCtx.PushData(process.ContextAppSources, map[string]map[string]interface{}{"a": {}, "b": {}})
	pCtx.PushData(process.ContextAppSourceTypes, map[string]string{"a": "demo", "b": "demo"})
	pCtx.PushData(process.ContextAppSourceTemplates, map[string]string{"demo": `
schema: {v: string}
$internal: {key: "demo", keyInputs: []}
output: {v: "hello"}
`})
	return pCtx
}

func statusNames(m map[string]SourceResolutionStatus) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func statusesOn(t *testing.T, ctx wfprocess.Context) map[string]SourceResolutionStatus {
	t.Helper()
	m, _ := ctx.GetData(SourceResolutionStatusKey).(map[string]SourceResolutionStatus)
	return m
}

// TestStatusesAccumulateAcrossRenderPasses pins the behaviour a component and
// its traits depend on.
//
// They render against one process.Context, one pass each. Each pass used to
// push a status map built from scratch, and PushData replaces, so whichever
// binding the component read was discarded the moment a trait read a different
// one. That is invisible in the Application's own output - the values still
// substitute correctly - but dispatcher.resolvedSourceHashes stamps a
// resolved-hash only for the bindings it finds in that final map, so the
// component's own sources silently stopped triggering auto-update, and lost
// their consumer attribution in status.sources[].
func TestStatusesAccumulateAcrossRenderPasses(t *testing.T) {
	pCtx := twoSourceContext(t)

	// The component's render reads source a.
	_, err := ResolveSourceExpressions(pCtx, map[string]interface{}{"x": "$(source.a.v)"}, SurfaceComponent)
	require.NoError(t, err)
	require.Equal(t, []string{"a"}, statusNames(statusesOn(t, pCtx)))

	// Its trait renders against the same context and reads source b.
	_, err = ResolveSourceExpressions(pCtx, map[string]interface{}{"y": "$(source.b.v)"}, SurfaceTrait)
	require.NoError(t, err)

	got := statusesOn(t, pCtx)
	assert.Equal(t, []string{"a", "b"}, statusNames(got),
		"a trait's pass must add to the component's statuses, not replace them")

	// The surviving entry has to be usable, not just present: the hash is
	// computed over ConsumedFields, so an entry that lost them is as good as
	// absent to the dispatcher.
	if a, ok := got["a"]; ok {
		assert.Equal(t, "hello", a.ConsumedFields["v"],
			"source a kept its entry but lost what was consumed from it")
		assert.NotEmpty(t, a.Reads, "source a kept its entry but lost its consumer attribution")
	}
}

// TestStatusesAccumulateForTheSameBindingTwice covers the other ordering: two
// passes reading the *same* binding, where the second must not drop what the
// first recorded.
func TestStatusesAccumulateForTheSameBindingTwice(t *testing.T) {
	pCtx := twoSourceContext(t)

	_, err := ResolveSourceExpressions(pCtx, map[string]interface{}{"x": "$(source.a.v)"}, SurfaceComponent)
	require.NoError(t, err)
	_, err = ResolveSourceExpressions(pCtx, map[string]interface{}{"y": "$(source.a.v)"}, SurfaceTrait)
	require.NoError(t, err)

	got := statusesOn(t, pCtx)
	require.Contains(t, got, "a")
	assert.Equal(t, "hello", got["a"].ConsumedFields["v"])
	assert.Len(t, got["a"].Reads, 2,
		"both readers of the same binding should be attributed, one per property")
}

// TestMergeSurvivesCollectionValues guards the dedupe key.
//
// SourceRead carries an interface{} Value, so using SourceRead itself as a map
// key compiles happily and then panics at run time the first time a source
// resolves to a map or a list - which is an ordinary thing for a source to do.
func TestMergeSurvivesCollectionValues(t *testing.T) {
	withValue := func(v interface{}) map[string]SourceResolutionStatus {
		return map[string]SourceResolutionStatus{"a": {
			Name:           "a",
			ConsumedFields: map[string]interface{}{"data": v},
			Reads:          []SourceRead{{SourceAttr: "data", Property: "p", Value: v}},
		}}
	}

	require.NotPanics(t, func() {
		mergeStatuses(withValue(map[string]interface{}{"k": "v"}), withValue([]interface{}{1, 2}))
	}, "a source resolving to a collection must not panic the status merge")
}
