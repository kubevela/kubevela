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

package schema

import (
	"encoding/json"
	"testing"

	"cuelang.org/go/cue"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kubevela/pkg/cel/template"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// readOf is the one read in a $( ) expression, as the engine plans it.
func readOf(t *testing.T, expr string) template.Reference {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"v": "$(" + expr + ")"})
	require.NoError(t, err)
	plan, err := celexpr.Vela.PlanJSON(raw)
	require.NoError(t, err)
	for _, root := range []string{propexpr.ComponentIdent, propexpr.EachIdent, propexpr.SourceIdent} {
		if reads := plan.Reads(root); len(reads) > 0 {
			require.Len(t, reads, 1)
			return reads[0].Reference
		}
	}
	t.Fatalf("no read in %s", expr)
	return template.Reference{}
}

func TestComponentReadKind(t *testing.T) {
	deployment := openAPI(t, `{"type":"object","properties":{
		"metadata":{"type":"object","properties":{"name":{"type":"string"}}},
		"spec":{"type":"object","properties":{"replicas":{"type":"integer"}}},
		"status":{}}}`)
	service := openAPI(t, `{"type":"object","properties":{
		"spec":{"type":"object","properties":{"ports":{"type":"array","items":{"type":"object",
			"properties":{"port":{"type":"integer"}}}}}}}}`)
	produced := map[string]Produced{
		"db":      {Output: deployment, Outputs: map[string]*openapi3.Schema{"svc": service}},
		"my-db":   {Output: deployment},
		"patched": {Outputs: map[string]*openapi3.Schema{"svc": service}},
	}
	for name, tc := range map[string]struct {
		expr string
		kind cue.Kind
		ok   bool
	}{
		"a workload field":                      {"component.db.output.metadata.name", cue.StringKind, true},
		"an integer field":                      {"component.db.output.spec.replicas", cue.IntKind, true},
		"the whole workload":                    {"component.db.output", cue.StructKind, true},
		"a resource's field":                    {"component.db.outputs.svc.spec.ports[0].port", cue.IntKind, true},
		"at a cluster":                          {`component.db.cluster("east").output.spec.replicas`, cue.IntKind, true},
		"at a cluster and namespace":            {`component.db.cluster("east").namespace("prod").output.metadata.name`, cue.StringKind, true},
		"a hyphenated name, by index":           {`component["my-db"].output.spec.replicas`, cue.IntKind, true},
		"a field the template does not write":   {"component.db.output.metadata.uid", cue.BottomKind, false},
		"status":                                {"component.db.output.status.readyReplicas", cue.BottomKind, false},
		"a resource the producer does not have": {"component.db.outputs.ingress.spec", cue.BottomKind, false},
		"the whole component":                   {"component.db", cue.BottomKind, false},
		"the outputs map":                       {"component.db.outputs", cue.BottomKind, false},
		"a component with no schemas":           {"component.cache.output.spec.replicas", cue.BottomKind, false},
		"a workload left untyped":               {"component.patched.output.spec.replicas", cue.BottomKind, false},
		"its resources still typed":             {"component.patched.outputs.svc.spec.ports[0].port", cue.IntKind, true},
	} {
		t.Run(name, func(t *testing.T) {
			kind, ok := ComponentReadKind(produced, readOf(t, tc.expr))
			require.Equal(t, tc.ok, ok)
			if ok {
				require.Equal(t, tc.kind, kind)
			}
		})
	}

	t.Run("a read of another root", func(t *testing.T) {
		_, ok := ComponentReadKind(produced, template.Reference{Root: propexpr.SourceIdent, Path: []string{"db", "output"}})
		require.False(t, ok)
	})
}

func TestEachReadKind(t *testing.T) {
	item := LiteralSchema([]interface{}{
		map[string]interface{}{"region": "eu", "wave": float64(1)},
		map[string]interface{}{"region": "us", "wave": float64(2)},
	})
	itemKind := func(path []string) (cue.Kind, bool) { return KindAt(item, append([]string{"0"}, path...)) }
	for name, tc := range map[string]struct {
		expr string
		kind cue.Kind
		ok   bool
	}{
		"the index is an int":        {"each.index", cue.IntKind, true},
		"the whole item":             {"each.item", cue.StructKind, true},
		"an item's field":            {"each.item.wave", cue.IntKind, true},
		"an item's string field":     {"each.item.region", cue.StringKind, true},
		"a field no item has":        {"each.item.zone", cue.BottomKind, false},
		"below the index":            {"each.index.x", cue.BottomKind, false},
		"the whole of each":          {"each", cue.BottomKind, false},
		"a field each does not have": {"each.count", cue.BottomKind, false},
	} {
		t.Run(name, func(t *testing.T) {
			kind, ok := EachReadKind(itemKind, readOf(t, tc.expr))
			require.Equal(t, tc.ok, ok)
			if ok {
				require.Equal(t, tc.kind, kind)
			}
		})
	}

	t.Run("items of unknown type still type the index", func(t *testing.T) {
		kind, ok := EachReadKind(nil, readOf(t, "each.index"))
		require.True(t, ok)
		require.Equal(t, cue.IntKind, kind)
		_, ok = EachReadKind(nil, readOf(t, "each.item.wave"))
		require.False(t, ok)
	})
}
