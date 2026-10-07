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

package health

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A snippet that opens with an import, which is what the CUE upgrade pass
// produces when it rewrites list arithmetic.
const importingCustom = `import "list"

parts: list.Concat([["ready:"], ["1/1"]])
message: "Healthy - \(parts[1])"
`

const importingHealth = `import "list"

checks: list.Concat([[true], [true]])
isHealth: checks[0] && checks[1]
`

func importContext() map[string]interface{} {
	return map[string]interface{}{
		"output": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"spec":       map[string]interface{}{"replicas": 1},
			"status":     map[string]interface{}{"replicas": 1, "readyReplicas": 1},
		},
	}
}

// A lone definition gets no `$super` at all, so a snippet that opens with an
// import still parses. Injecting one regardless put a field above the import
// and the whole snippet failed to compile, silently costing the status message.
func TestSnippetOpeningWithAnImportStillEvaluates(t *testing.T) {
	res, err := GetStatus(importContext(), &StatusRequest{
		Health: importingHealth,
		Custom: importingCustom,
	})
	require.NoError(t, err)
	require.True(t, res.Healthy)
	require.Equal(t, "Healthy - 1/1", res.Message)
}

// And with a parent, where `$super` genuinely has something to say, it goes
// after the snippet so the import keeps the top of the file.
func TestSuperIsAddedAfterAnImportingSnippet(t *testing.T) {
	child := `import "list"

joined: list.Concat([["child"], ["says"]])
message: "\($super.message), then \(joined[0])"
`
	res, err := GetStatus(importContext(), &StatusRequest{
		Health:    importingHealth,
		Custom:    child,
		Ancestors: []Snippets{{Health: importingHealth, Custom: importingCustom}},
	})
	require.NoError(t, err)
	require.True(t, res.Healthy)
	require.Equal(t, "Healthy - 1/1, then child", res.Message)
}
