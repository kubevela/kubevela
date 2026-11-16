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

package template

import (
	"context"
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/workflow/providers"
)

// requestStep renders the request step against a reply, provider calls left unresolved, and
// returns the rendered value.
func requestStep(t *testing.T, statusCode int, body string) cue.Value {
	t.Helper()
	src := readWorkflowStepCue(t, "request.cue") + fmt.Sprintf(`
template: parameter: url: "https://api.example.com/version"
template: req: $returns: {statusCode: %d, body: %q}
`, statusCode, body)
	v, err := providers.DefaultCompiler.Get().CompileStringWithOptions(context.Background(), src, cuex.DisableResolveProviderFunctions{})
	require.NoError(t, err)
	return v.LookupPath(cue.ParsePath("template"))
}

func TestTheRequestStepFailsOnlyOnAnErrorStatus(t *testing.T) {
	failing := func(v cue.Value) bool { return v.LookupPath(cue.ParsePath("fail.requestFail")).Exists() }

	v := requestStep(t, 400, "{}")
	require.True(t, failing(v), "a 400 fails the step")
	msg, err := v.LookupPath(cue.ParsePath("fail.requestFail.message")).String()
	require.NoError(t, err)
	require.Equal(t, "request of https://api.example.com/version is fail: 400", msg)

	v = requestStep(t, 200, `{"version":"1.0"}`)
	require.NoError(t, v.Validate(cue.Concrete(false)))
	require.False(t, failing(v))
	version, err := v.LookupPath(cue.ParsePath("response.version")).String()
	require.NoError(t, err)
	require.Equal(t, "1.0", version, "a JSON reply is parsed into response")

	for name, body := range map[string]string{"a reply that is not JSON": "ok", "an empty reply": ""} {
		v := requestStep(t, 200, body)
		require.NoError(t, v.Validate(cue.Concrete(false)), name+" renders")
		require.False(t, failing(v), name+" succeeds")
		require.False(t, v.LookupPath(cue.ParsePath("response")).Exists(), name+" has no parsed response")
		kept, err := v.LookupPath(cue.ParsePath("responseBody")).String()
		require.NoError(t, err)
		require.Equal(t, body, kept, name+" is kept as sent")
	}
}
