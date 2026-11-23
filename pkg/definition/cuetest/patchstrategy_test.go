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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/utils"
)

func TestPatchStrategyConflicts(t *testing.T) {
	cases := map[string]struct {
		src  string
		want []string
	}{
		"a strategy in one branch and none in another": {
			src: `
patch: {
	let content = {
		resources: {
			if parameter.both {
				// +patchStrategy=retainKeys
				requests: cpu: "1"
			}
			if !parameter.both {
				requests: cpu: "1"
			}
		}
	}
	spec: containers: [content]
}`,
			want: []string{"let content.resources.requests: retainKeys at line 7, none at line 10"},
		},
		"two strategies for one field": {
			src: `
patch: spec: {
	if a {
		// +patchStrategy=replace
		env: []
	}
	if b {
		// +patchStrategy=retainKeys
		env: []
	}
}`,
			want: []string{"patch.spec.env: replace at line 5, retainKeys at line 9"},
		},
		"the same strategy in every branch": {
			src: `
patch: spec: {
	if a {
		// +patchStrategy=replace
		env: []
	}
	if !a {
		// +patchStrategy=replace
		env: []
	}
}`,
		},
		"a parameter of the same name is another field": {
			src: `
patch: spec: {
	// +patchStrategy=replace
	env: parameter.env
}
parameter: env: [...string]`,
		},
		"list elements are one path": {
			src: `
patch: spec: containers: [{
	// +patchStrategy=replace
	ports: []
}, {
	ports: []
}]`,
			want: []string{"patch.spec.containers[].ports: replace at line 4, none at line 6"},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := patchStrategyConflicts("def.cue", []byte(c.src))
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}
}

// TestShippedPatchStrategiesAgree fails a shipped definition that gives one field different
// patch strategies in different branches. CUE carries every declaration's strategy comment
// onto the field, taken branch or not, so the field gets a strategy the branch never asked for.
func TestShippedPatchStrategiesAgree(t *testing.T) {
	requireShippedDefinitionTests(t)
	var conflicts []string
	require.NoError(t, filepath.WalkDir(shippedDefinitions, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".cue") || strings.HasSuffix(path, utils.CUETestFileSuffix) {
			return err
		}
		src, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}
		found, err := patchStrategyConflicts(path, src)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(shippedDefinitions, path)
		for _, c := range found {
			conflicts = append(conflicts, rel+": "+c)
		}
		return nil
	}))
	require.Empty(t, conflicts, "give the field the same strategy everywhere, or build the patch that differs as its own let and choose between them")
}

// strategyDecl is one declaration of a field and the patch strategy its comment gives.
type strategyDecl struct {
	strategy string
	line     int
}

// patchStrategyConflicts lists the fields of a CUE file declared with different patch strategies,
// "none" counting as one. A path runs through if and for clauses, which are transparent, and
// names list elements "[]" and a let's value "let <name>".
func patchStrategyConflicts(name string, src []byte) ([]string, error) {
	f, err := parser.ParseFile(name, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	decls := map[string][]strategyDecl{}
	for _, d := range f.Decls {
		walkStrategyDecl(d, "", decls)
	}
	var conflicts []string
	for path, ds := range decls {
		var seen []strategyDecl
		strategic := false
		for _, d := range ds {
			strategic = strategic || d.strategy != "none"
			known := false
			for _, s := range seen {
				known = known || s.strategy == d.strategy
			}
			if !known {
				seen = append(seen, d)
			}
		}
		if !strategic || len(seen) < 2 {
			continue
		}
		parts := make([]string, len(seen))
		for i, s := range seen {
			parts[i] = fmt.Sprintf("%s at line %d", s.strategy, s.line)
		}
		conflicts = append(conflicts, path+": "+strings.Join(parts, ", "))
	}
	sort.Strings(conflicts)
	return conflicts, nil
}

func walkStrategyDecl(d ast.Decl, path string, into map[string][]strategyDecl) {
	switch d := d.(type) {
	case *ast.Field:
		label, _, err := ast.LabelName(d.Label)
		if err != nil {
			return
		}
		p := label
		if path != "" {
			p = path + "." + label
		}
		into[p] = append(into[p], strategyDecl{strategy: patchStrategyOf(d), line: d.Pos().Line()})
		walkStrategyExpr(d.Value, p, into)
	case *ast.LetClause:
		walkStrategyExpr(d.Expr, "let "+d.Ident.Name, into)
	case *ast.Comprehension:
		walkStrategyExpr(d.Value, path, into)
	case *ast.EmbedDecl:
		walkStrategyExpr(d.Expr, path, into)
	}
}

func walkStrategyExpr(e ast.Expr, path string, into map[string][]strategyDecl) {
	switch e := e.(type) {
	case *ast.StructLit:
		for _, d := range e.Elts {
			walkStrategyDecl(d, path, into)
		}
	case *ast.ListLit:
		for _, elt := range e.Elts {
			walkStrategyExpr(elt, path+"[]", into)
		}
	case *ast.Comprehension:
		walkStrategyExpr(e.Value, path, into)
	case *ast.BinaryExpr:
		walkStrategyExpr(e.X, path, into)
		walkStrategyExpr(e.Y, path, into)
	case *ast.ParenExpr:
		walkStrategyExpr(e.X, path, into)
	}
}

func patchStrategyOf(f *ast.Field) string {
	for _, cg := range ast.Comments(f) {
		for _, c := range cg.List {
			if _, s, ok := strings.Cut(c.Text, "+patchStrategy="); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return "none"
}
