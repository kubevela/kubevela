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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wfprocess "github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/pkg/cue/process"
)

// chainedContext carries a binding whose own properties read an earlier one,
// which is how chaining is written.
func chainedContext(t *testing.T) (wfprocess.Context, map[string]map[string]interface{}) {
	t.Helper()
	bindings := map[string]map[string]interface{}{
		"upstream": {},
		"chained":  {"input": "$(source.upstream.v)"},
	}
	pCtx := process.NewContext(process.ContextData{
		Namespace: "default", CompName: "web", AppName: "app",
	})
	pCtx.PushData(process.ContextAppSources, bindings)
	pCtx.PushData(process.ContextAppSourceTypes, map[string]string{
		"upstream": "plain", "chained": "echo",
	})
	pCtx.PushData(process.ContextAppSourceTemplates, map[string]string{
		"plain": `
schema: {v: string}
$internal: {key: "plain", keyInputs: []}
output: {v: "hello"}
`,
		"echo": `
schema: {v: string}
$internal: {key: "echo", keyInputs: []}
parameter: {input: string}
output: {v: parameter.input}
`,
	})
	return pCtx, bindings
}

// TestResolvingDoesNotRewriteTheBindings pins that a render reads the binding
// properties rather than consuming them.
//
// The properties map comes straight off the process context with no copy, so a
// walker assigning its results back into it would consume the bindings. A
// component and all its traits render against one context: the first pass would
// replace the chained binding's `$(source.upstream.v)` with the literal it
// resolved to, and every later pass would see a binding that reads nothing, the
// chain invisible after the first render that touched it.
func TestResolvingDoesNotRewriteTheBindings(t *testing.T) {
	pCtx, bindings := chainedContext(t)

	_, err := ResolveSourceExpressions(pCtx, map[string]interface{}{"x": "$(source.chained.v)"}, SurfaceComponent)
	require.NoError(t, err)

	assert.Equal(t, "$(source.upstream.v)", bindings["chained"]["input"],
		"the binding's declared properties must survive the render that read them")

	live, _ := pCtx.GetData(process.ContextAppSources).(map[string]map[string]interface{})
	require.NotNil(t, live)
	assert.Equal(t, "$(source.upstream.v)", live["chained"]["input"],
		"the properties on the context must not be consumed by a render")
}

// TestChainStillResolvesOnASecondPass is the consequence, stated as behaviour: a
// trait rendering after its component must resolve the chain the same way.
func TestChainStillResolvesOnASecondPass(t *testing.T) {
	pCtx, _ := chainedContext(t)

	first, err := ResolveSourceExpressions(pCtx, map[string]interface{}{"x": "$(source.chained.v)"}, SurfaceComponent)
	require.NoError(t, err)
	second, err := ResolveSourceExpressions(pCtx, map[string]interface{}{"x": "$(source.chained.v)"}, SurfaceTrait)
	require.NoError(t, err)

	assert.Equal(t, first, second, "a second pass must resolve the chain to the same value")
	assert.Equal(t, map[string]interface{}{"x": "hello"}, second)

	// And the upstream link is still reported, not just the chained one.
	got := statusesOn(t, pCtx)
	assert.Contains(t, got, "upstream", "the chained binding's own read must still be recorded")
	assert.Contains(t, got, "chained")
}
