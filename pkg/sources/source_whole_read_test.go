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

func wholeReadContext(t *testing.T, template string) wfprocess.Context {
	t.Helper()
	pCtx := process.NewContext(process.ContextData{
		Namespace: "default", CompName: "web", AppName: "app",
	})
	pCtx.PushData(process.ContextAppSources, map[string]map[string]interface{}{"cfg": {}})
	pCtx.PushData(process.ContextAppSourceTypes, map[string]string{"cfg": "whole"})
	pCtx.PushData(process.ContextAppSourceTemplates, map[string]string{"whole": template})
	return pCtx
}

const wholeTemplate = `
schema: {host: string, port: int}
$internal: {key: "whole", keyInputs: []}
output: {host: "example.com", port: 8080}
`

// TestWholeSourceReadIsRecorded covers reading a binding entire rather than a
// field of it.
//
// `$(source.cfg)` substituted correctly but recorded nothing. The reference is
// Path=["cfg"], so the field path is "", and looking up "" walked one empty
// segment and found no key by that name - so recordConsumedValue never ran.
//
// The value still reached the component, which is why this was invisible. What
// was lost is one layer down: resolvedSourceHashes stamps a resolved-hash only
// for bindings with consumed fields, so the source could change forever and the
// workload was never re-dispatched. autoUpdate was silently a no-op for exactly
// the read that takes the whole thing.
func TestWholeSourceReadIsRecorded(t *testing.T) {
	pCtx := wholeReadContext(t, wholeTemplate)

	out, err := ResolveSourceExpressions(pCtx, map[string]interface{}{"all": "$(source.cfg)"}, SurfaceComponent)
	require.NoError(t, err)

	// Precondition: it substitutes, and always did.
	assert.Equal(t, map[string]interface{}{
		"all": map[string]interface{}{"host": "example.com", "port": int64(8080)},
	}, out)

	statuses := statusesOn(t, pCtx)
	require.Contains(t, statuses, "cfg")
	st := statuses["cfg"]

	require.NotEmpty(t, st.ConsumedFields,
		"a whole-source read must be recorded, or no hash is stamped and autoUpdate never fires")
	assert.Equal(t, map[string]interface{}{"host": "example.com", "port": int64(8080)},
		st.ConsumedFields[""], "the whole source is what was consumed")

	require.Len(t, st.Reads, 1)
	assert.Equal(t, "all", st.Reads[0].Property)
	assert.Equal(t, "", st.Reads[0].SourceAttr, "an empty attr is how 'the whole binding' is spelled")
}

// TestWholeSourceReadStillRedacts pins that recording the whole thing does not
// publish a secret inside it. RedactValue descends from the read path, and
// joinMaskPath treats an empty prefix as the root, so a mark one level down
// still applies.
func TestWholeSourceReadStillRedacts(t *testing.T) {
	pCtx := wholeReadContext(t, `
schema: {
  user: string
  // +sensitive
  password: string
}
$internal: {key: "whole", keyInputs: []}
output: {user: "admin", password: "hunter2"}
`)

	_, err := ResolveSourceExpressions(pCtx, map[string]interface{}{"all": "$(source.cfg)"}, SurfaceComponent)
	require.NoError(t, err)

	st := statusesOn(t, pCtx)["cfg"]
	require.NotEmpty(t, st.ConsumedFields)

	// Redacted the way the controller does it: per read, against the paths the
	// definition declared sensitive.
	masks := map[string]struct{}{}
	for _, p := range st.SensitivePaths {
		masks[p] = struct{}{}
	}
	whole, ok := RedactValue("", st.ConsumedFields[""], masks).(map[string]interface{})
	require.True(t, ok, "the whole read should redact as a map")

	assert.Equal(t, "admin", whole["user"])
	assert.Equal(t, "***", whole["password"],
		"a sensitive field must stay masked when the whole binding is read")
}

// TestFieldReadsStillRecorded guards the ordinary case against the change.
func TestFieldReadsStillRecorded(t *testing.T) {
	pCtx := wholeReadContext(t, wholeTemplate)

	_, err := ResolveSourceExpressions(pCtx, map[string]interface{}{
		"h": "$(source.cfg.host)",
		"p": "$(source.cfg.port)",
	}, SurfaceComponent)
	require.NoError(t, err)

	st := statusesOn(t, pCtx)["cfg"]
	assert.Equal(t, "example.com", st.ConsumedFields["host"])
	assert.Equal(t, int64(8080), st.ConsumedFields["port"])
	assert.NotContains(t, st.ConsumedFields, "", "a field read must not be recorded as a whole read")
}
