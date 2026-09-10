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

package celexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEvalTreeLeavesItsInputAlone pins that evaluating is a read.
//
// EvalTree is exported and takes the caller's tree. Writing results back into it
// would make the input and the output the same object, so evaluating twice would
// answer differently the second time, the expressions already replaced by their
// values.
//
// That failure is quiet in the worst way: it corrupts any benchmark written
// against EvalTree, because every iteration after the first measures a tree with
// no expressions left in it. A function that consumes its argument corrupts
// whatever measures it.
func TestEvalTreeLeavesItsInputAlone(t *testing.T) {
	values := map[string]map[string]interface{}{"cfg": {"host": "example.com", "port": float64(8080)}}

	tree := map[string]interface{}{
		"url":  "$(source.cfg.host)",
		"port": "$(source.cfg.port)",
		"nested": map[string]interface{}{
			"addr": "https://$(source.cfg.host):$(source.cfg.port)",
		},
		"list":    []interface{}{"$(source.cfg.host)", "plain"},
		"literal": "untouched",
	}

	first, err := EvalTree(tree, values, map[string]interface{}{})
	require.NoError(t, err)

	// The input still reads what it read.
	assert.Equal(t, "$(source.cfg.host)", tree["url"])
	assert.Equal(t, "https://$(source.cfg.host):$(source.cfg.port)",
		tree["nested"].(map[string]interface{})["addr"])
	assert.Equal(t, "$(source.cfg.host)", tree["list"].([]interface{})[0])

	// ...so evaluating it again gives the same answer, which is the property
	// every caller actually depends on.
	second, err := EvalTree(tree, values, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, first, second, "evaluating the same tree twice must give the same result")

	out := first.(map[string]interface{})
	assert.Equal(t, "example.com", out["url"])
	assert.Equal(t, int64(8080), out["port"], "a lone expression keeps its own type")
	assert.Equal(t, "https://example.com:8080", out["nested"].(map[string]interface{})["addr"])
	assert.Equal(t, "untouched", out["literal"])
}
