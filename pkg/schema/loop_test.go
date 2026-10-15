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
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cel/template"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

func TestBareRead(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  string // the read, or "" for none
	}{
		{"$(source.cfg.regions)", "source.cfg.regions"},
		{"$( source.cfg.regions )", "source.cfg.regions"},
		{`$(component["my-db"].output.spec)`, `component["my-db"].output.spec`},
		{`$(component.db.cluster("east").output)`, `component.db.cluster("east").output`},
		{"$(steps.list.outputs.names)", "steps.list.outputs.names"},
		{"$(source.cfg.regions.filter(r, r.wave > 1))", ""},
		{"$(source.cfg.a + source.cfg.b)", ""},
		{"$(has(source.cfg.a) ? source.cfg.a : [])", ""},
		{"r-$(source.cfg.region)", ""},
		{"plain text", ""},
		{"$(source.cfg", ""},
	} {
		t.Run(tc.value, func(t *testing.T) {
			ref, ok := BareRead(tc.value)
			if tc.want == "" {
				require.False(t, ok, "read %s", ref)
				return
			}
			require.True(t, ok)
			require.Equal(t, tc.want, ref.String())
		})
	}
}

func TestLoopItemKind(t *testing.T) {
	// A read's kind, from a schema of a list of {name: string, wave: int} at
	// source.cfg.regions.
	regions := LiteralSchema([]interface{}{map[string]interface{}{"name": "eu", "wave": float64(1)}})
	readKind := func(ref template.Reference) (cue.Kind, bool) {
		if ref.Root != propexpr.SourceIdent || len(ref.Path) < 2 || strings.Join(ref.Path[:2], ".") != "cfg.regions" {
			return cue.BottomKind, false
		}
		return KindAt(regions, ref.Path[2:])
	}
	for _, tc := range []struct {
		name  string
		items string
		path  []string
		kind  cue.Kind
		ok    bool
	}{
		{"a literal list's item", `[1, 2]`, nil, cue.IntKind, true},
		{"a literal object's field", `[{"region":"eu"}]`, []string{"region"}, cue.StringKind, true},
		{"items of two kinds", `[1, "a"]`, nil, cue.BottomKind, false},
		{"a read of a typed list", `"$(source.cfg.regions)"`, []string{"wave"}, cue.IntKind, true},
		{"its whole item", `"$(source.cfg.regions)"`, nil, cue.StructKind, true},
		{"a field its items do not have", `"$(source.cfg.regions)"`, []string{"zone"}, cue.BottomKind, false},
		{"a read the reader cannot type", `"$(source.other.list)"`, nil, cue.BottomKind, false},
		{"a computed list", `"$(source.cfg.regions.filter(r, r.wave > 1))"`, []string{"wave"}, cue.BottomKind, false},
		{"a read of the loop itself", `"$(each.item)"`, nil, cue.BottomKind, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := LoopItemKind([]byte(tc.items), readKind)
			if !tc.ok && item == nil {
				return
			}
			require.NotNil(t, item)
			kind, ok := item(tc.path)
			require.Equal(t, tc.ok, ok)
			if ok {
				require.Equal(t, tc.kind, kind)
			}
		})
	}

	for _, items := range []string{"", `{"a":1}`, `3`, `not json`} {
		require.Nil(t, LoopItemKind([]byte(items), readKind), "items %q declare nothing", items)
	}
}
