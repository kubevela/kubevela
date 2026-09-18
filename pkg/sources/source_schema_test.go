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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
)

// The extracted schema was memoised in an unbounded map keyed by the whole
// template, so every edit to every definition retained another copy of its text
// for the life of the process.
func TestSchemaExprCacheIsBounded(t *testing.T) {
	for i := 0; i < schemaExprCacheSize*2; i++ {
		_, err := extractSourceSchemaExpr(fmt.Sprintf("schema: {v%d: string}\noutput: {v%d: \"x\"}\n", i, i))
		require.NoError(t, err)
	}
	require.LessOrEqual(t, schemaExprCache.Len(), schemaExprCacheSize)
}

// The output was rendered to JSON text and parsed back before being checked
// against the schema. A float64 with no fractional part marshals as `2`, which
// CUE reads as an int, so a source declaring `ratio: float` was rejected at
// render by its own schema with "conflicting values 2 and float".
//
// The schema is still a closed contract: an undeclared field, a missing one, or
// a wrong type all have to keep failing.
func TestValidateResolvedOutputChecksTheSchemaAsAContract(t *testing.T) {
	const template = `
schema: {
	ratio: float
	port:  int
	host:  string
	tags?: [...string]
}
output: {ratio: 1.0, port: 8080, host: "x"}
`
	for _, tc := range []struct {
		name    string
		output  map[string]interface{}
		wantErr string
	}{
		{
			name:   "an integral float is a float",
			output: map[string]interface{}{"ratio": 2.0, "port": 8080, "host": "x"},
		},
		{
			name:   "a fractional float still works",
			output: map[string]interface{}{"ratio": 2.5, "port": 8080, "host": "x"},
		},
		{
			name:   "an optional field may be absent",
			output: map[string]interface{}{"ratio": 2.0, "port": 8080, "host": "x"},
		},
		{
			name:    "an undeclared field is refused",
			output:  map[string]interface{}{"ratio": 2.0, "port": 8080, "host": "x", "extra": "no"},
			wantErr: "extra",
		},
		{
			name:    "a missing required field is refused",
			output:  map[string]interface{}{"ratio": 2.0, "port": 8080},
			wantErr: "host",
		},
		{
			name:    "a wrong type is refused",
			output:  map[string]interface{}{"ratio": 2.0, "port": "8080", "host": "x"},
			wantErr: "port",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &sourceResolver{sourceSchemas: map[string]string{}}
			err := r.validateResolvedOutput("s", template, tc.output)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// A field the schema declares `float` holding exactly 2.0 was passed to CEL as
// an int, because a JSON number with no fractional part reads as one. So
// `source.cfg.ratio * 2.0` type-checked as double at admission and failed at
// render with "no such overload".
//
// Retyping against the schema is what settles it, and it has to settle a cached
// value the same way it settles a fresh one.
func TestTypeToSchemaRestoresWhatJSONLost(t *testing.T) {
	const template = `
schema: {
	ratio:    float
	port:     int
	replicas: int
	name:     string
}
output: {ratio: 1.0, port: 80, replicas: 1, name: "x"}
`
	// What a value looks like coming back out of the cache.
	cached := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(
		[]byte(`{"ratio":2,"port":8080,"replicas":3,"name":"x"}`), &cached))
	require.IsType(t, float64(0), cached["ratio"], "precondition: JSON loses the distinction")
	require.IsType(t, float64(0), cached["port"])

	r := &sourceResolver{sourceSchemas: map[string]string{}}
	typed := r.typeToSchema(template, cached)

	require.IsType(t, float64(0), typed["ratio"], "a declared float stays a float")
	require.IsType(t, int64(0), typed["port"], "a declared int becomes an int")
	require.IsType(t, int64(0), typed["replicas"])
	require.Equal(t, "x", typed["name"])

	// Both kinds of arithmetic now work, which is the whole point.
	env, err := celexpr.DynEnv()
	require.NoError(t, err)
	in := map[string]interface{}{
		"source":  map[string]interface{}{"cfg": typed},
		"context": map[string]interface{}{},
	}
	got, err := celexpr.EvalPropertyTyped(env, "$(source.cfg.ratio * 2.0)", in)
	require.NoError(t, err)
	require.Equal(t, float64(4), got)

	got, err = celexpr.EvalPropertyTyped(env, "$(source.cfg.port + 1)", in)
	require.NoError(t, err)
	require.Equal(t, int64(8081), got)
}
