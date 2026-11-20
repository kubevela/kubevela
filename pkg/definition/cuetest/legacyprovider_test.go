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

package cuetest

import (
	"context"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

// A provider function taking no $params is not a legacy one: its mocked
// results go under $returns, and are recorded as its returns.
func TestMockOfAProviderWithoutParams(t *testing.T) {
	mocks, err := parseMocksFor(cuecontext.New().CompileString(`{
		"vela/multicluster": "#ListClusters": $returns: outputs: clusters: ["eu-1", "us-1"]
	}`), execProviders)
	require.NoError(t, err)
	compiler, rec, err := useExecProviders(mocks)
	require.NoError(t, err)

	v, err := compiler.CompileString(context.Background(), `import "vela/multicluster"
list: multicluster.#ListClusters`)
	require.NoError(t, err)
	var clusters []string
	require.NoError(t, v.LookupPath(cue.ParsePath("list.$returns.outputs.clusters")).Decode(&clusters))
	require.Equal(t, []string{"eu-1", "us-1"}, clusters)
	require.Len(t, rec.calls, 1)
	require.Equal(t, map[string]any{"outputs": map[string]any{"clusters": []any{"eu-1", "us-1"}}}, rec.calls[0].Returns)
}
