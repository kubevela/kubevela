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

package schema_test

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/definition/cuetest"
	"github.com/oam-dev/kubevela/pkg/schema"
)

// What a shipped definition renders has the kinds its output schemas declare, in
// every case its tests render: a declared kind the render contradicts would have
// admission refuse a read that resolves.
func TestShippedDefinitionsRenderWhatTheirOutputSchemasDeclare(t *testing.T) {
	var mismatches []string
	ungenerated := map[string]string{}
	cases, checked := 0, 0
	for _, dir := range []string{"component", "trait"} {
		suites, err := cuetest.Load("../../vela-templates/definitions/internal/" + dir)
		require.NoError(t, err)
		for _, suite := range suites {
			require.NoError(t, suite.Err, suite.File)
			for _, c := range suite.Cases {
				if c.Pending || (c.Test != cuetest.TestComponentRender && c.Test != cuetest.TestTraitRender) ||
					c.Expect.LookupPath(cue.ParsePath("error")).Exists() {
					continue
				}
				rendered, err := cuetest.Render(c.Subject, c.Input)
				if err != nil {
					continue
				}
				tmpl := c.Subject.Template
				if c.Subject.Object != nil {
					if stored, ok, _ := unstructured.NestedString(c.Subject.Object.Object, "spec", "schematic", "cue", "template"); ok {
						tmpl = stored
					}
				}
				schemas, err := schema.GenerateOutputSchemas(context.Background(), tmpl)
				if err != nil {
					// No schema: the definition's reads fit anything, which is safe.
					ungenerated[c.Subject.Name] = err.Error()
					continue
				}
				cases++
				where := fmt.Sprintf("%s %q", c.Subject.Name, c.Name)
				if c.Test == cuetest.TestComponentRender && len(c.Input.Traits) == 0 {
					checked += compare(where+" output", schemas.Output, rendered.Output, nil, &mismatches)
					for name, obj := range rendered.Outputs {
						checked += compare(where+" outputs."+name, schemas.Outputs[name], obj, nil, &mismatches)
					}
				}
				if c.Test == cuetest.TestTraitRender {
					for _, tr := range rendered.Traits {
						for name, obj := range tr.Outputs {
							checked += compare(where+" outputs."+name, schemas.Outputs[name], obj, nil, &mismatches)
						}
					}
				}
			}
		}
	}
	t.Logf("%d rendered cases, %d declared leaves compared", cases, checked)
	for name, err := range ungenerated {
		t.Logf("no output schema for %s: %s", name, err)
	}
	require.Greater(t, cases, 0)
	require.Greater(t, checked, 0, "no declared leaf was compared")
	sort.Strings(mismatches)
	require.Empty(t, mismatches, "declared kinds the render contradicts:\n%s", strings.Join(mismatches, "\n"))
}

// compare checks every value in v against the kind s declares at its path,
// returning how many it compared.
func compare(where string, s *openapi3.Schema, v interface{}, path []string, out *[]string) int {
	if s == nil {
		return 0
	}
	n := 0
	if declared, ok := schema.KindAt(s, path); ok {
		n++
		if actual := kindOfValue(v); !fits(declared, actual) {
			*out = append(*out, fmt.Sprintf("%s .%s: declared %s, rendered %s (%v)", where, strings.Join(path, "."), declared, actual, v))
		}
	}
	switch x := v.(type) {
	case map[string]interface{}:
		for k, e := range x {
			n += compare(where, s, e, append(append([]string{}, path...), k), out)
		}
	case []interface{}:
		for i, e := range x {
			n += compare(where, s, e, append(append([]string{}, path...), strconv.Itoa(i)), out)
		}
	}
	return n
}

func kindOfValue(v interface{}) cue.Kind {
	switch x := v.(type) {
	case string:
		return cue.StringKind
	case bool:
		return cue.BoolKind
	case int, int64:
		return cue.IntKind
	case float64:
		if x == math.Trunc(x) {
			return cue.IntKind
		}
		return cue.FloatKind
	case []interface{}:
		return cue.ListKind
	case map[string]interface{}:
		return cue.StructKind
	case nil:
		return cue.NullKind
	}
	return cue.BottomKind
}

// fits reports whether a rendered kind is one the declared kind holds: a number
// holds an integer.
func fits(declared, actual cue.Kind) bool {
	if declared == actual {
		return true
	}
	return declared == cue.NumberKind && (actual == cue.IntKind || actual == cue.FloatKind)
}
