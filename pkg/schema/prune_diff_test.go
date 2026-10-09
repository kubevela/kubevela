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
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParameterSchemaDiff generates the schema of every definition under
// PARAMETER_SCHEMA_DIFF_DIRS (colon separated) both ways and fails on any
// definition where the pruned schema differs from, or fails where, the
// whole-template one succeeds. PARAMETER_SCHEMA_DIFF_OUT names a file for the
// per-definition report.
func TestParameterSchemaDiff(t *testing.T) {
	dirs := os.Getenv("PARAMETER_SCHEMA_DIFF_DIRS")
	if dirs == "" {
		t.Skip("PARAMETER_SCHEMA_DIFF_DIRS not set")
	}
	type row struct {
		File, Old, New, Path, Result string
	}
	var rows []row
	counts := map[string]int{}
	ctx := context.Background()
	for _, dir := range strings.Split(dirs, ":") {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".cue") {
				return err
			}
			b, err := os.ReadFile(filepath.Clean(p))
			if err != nil {
				return err
			}
			tmpl, ok, err := definitionTemplate(string(b))
			r := row{File: p}
			switch {
			case err != nil:
				r.Result = "unparseable: " + err.Error()
			case !ok:
				return nil
			default:
				oldS, oldErr := ParsePropertiesToSchema(ctx, tmpl)
				newS, path, newErr := ParseParameterSchema(ctx, tmpl)
				r.Old, r.New, r.Path = status(oldErr), status(newErr), string(path)
				switch {
				case oldErr != nil && newErr != nil:
					r.Result = "both-fail"
				case oldErr != nil:
					r.Result = "fixed"
				case newErr != nil:
					r.Result = "regressed"
				case sameJSON(t, oldS, newS):
					r.Result = "same"
				default:
					r.Result = "differs"
				}
			}
			counts[strings.SplitN(r.Result, ":", 2)[0]]++
			counts["path:"+r.Path]++
			rows = append(rows, r)
			return nil
		})
		require.NoError(t, err)
	}

	var keys []string
	for k := range counts {
		keys = append(keys, fmt.Sprintf("%s=%d", k, counts[k]))
	}
	sort.Strings(keys)
	t.Logf("%d definitions: %s", len(rows), strings.Join(keys, " "))

	if out := os.Getenv("PARAMETER_SCHEMA_DIFF_OUT"); out != "" {
		b, err := json.MarshalIndent(rows, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(out, b, 0600))
	}
	for _, r := range rows {
		if r.Result != "same" && r.Result != "fixed" {
			t.Errorf("%s: %s (old: %s, new: %s)", r.File, r.Result, r.Old, r.New)
		}
	}
}

// definitionTemplate extracts what `vela def apply` stores as the CUE
// template: the file's imports plus the body of its `template` field.
func definitionTemplate(src string) (string, bool, error) {
	f, err := parser.ParseFile("def", src, parser.ParseComments)
	if err != nil {
		return "", false, err
	}
	out := &ast.File{}
	found := false
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.ImportDecl:
			out.Decls = append(out.Decls, d)
		case *ast.Field:
			if name, _, _ := ast.LabelName(d.Label); name == "template" {
				if s, ok := d.Value.(*ast.StructLit); ok {
					out.Decls = append(out.Decls, s.Elts...)
					found = true
				}
			}
		}
	}
	if !found {
		return "", false, nil
	}
	b, err := format.Node(out)
	return string(b), true, err
}

func status(err error) string {
	if err == nil {
		return "ok"
	}
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

func sameJSON(t *testing.T, a, b any) bool {
	var x, y any
	for _, p := range []struct {
		in  any
		out *any
	}{{a, &x}, {b, &y}} {
		raw, err := json.Marshal(p.in)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, p.out))
	}
	return reflect.DeepEqual(x, y)
}

// TestGeneratorDiff runs GenerateParameterSchemas over the same directories
// and reports, per kind of difference, how its OpenAPI differs from
// ParsePropertiesToSchema's. PARAMETER_GENERATOR_DIFF_OUT names a file for the
// report.
func TestGeneratorDiff(t *testing.T) {
	dirs := os.Getenv("PARAMETER_SCHEMA_DIFF_DIRS")
	if dirs == "" {
		t.Skip("PARAMETER_SCHEMA_DIFF_DIRS not set")
	}
	ctx := context.Background()
	kinds := map[string]int{}
	examples := map[string]string{}
	var failed []string
	total := 0
	for _, dir := range strings.Split(dirs, ":") {
		require.NoError(t, filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".cue") {
				return err
			}
			b, err := os.ReadFile(filepath.Clean(p))
			if err != nil {
				return err
			}
			tmpl, ok, err := definitionTemplate(string(b))
			if err != nil || !ok {
				return nil
			}
			total++
			oldS, oldErr := ParsePropertiesToSchema(ctx, tmpl)
			ps, newErr := GenerateParameterSchemas(ctx, tmpl)
			if newErr != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", p, newErr))
				return nil
			}
			if oldErr != nil {
				kinds["old failed, new ok"]++
				return nil
			}
			var o, n any
			for _, x := range []struct {
				in  any
				out *any
			}{{oldS, &o}, {ps.OpenAPI, &n}} {
				raw, _ := json.Marshal(x.in)
				_ = json.Unmarshal(raw, x.out)
			}
			for _, k := range jsonDiff("", o, n) {
				kinds[k.kind]++
				if _, ok := examples[k.kind]; !ok {
					examples[k.kind] = filepath.Base(p) + " " + k.path
				}
			}
			return nil
		}))
	}
	var keys []string
	for k := range kinds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out strings.Builder
	fmt.Fprintf(&out, "%d definitions, %d failed with the new generator\n", total, len(failed))
	for _, f := range failed {
		fmt.Fprintf(&out, "  FAIL %s\n", f)
	}
	for _, k := range keys {
		fmt.Fprintf(&out, "%5d  %-40s e.g. %s\n", kinds[k], k, examples[k])
	}
	if o := os.Getenv("PARAMETER_GENERATOR_DIFF_OUT"); o != "" {
		require.NoError(t, os.WriteFile(o, []byte(out.String()), 0600))
	}
	assert.Empty(t, failed)
}

type diffEntry struct{ kind, path string }

// jsonDiff lists where two decoded JSON documents differ, named by the key
// that differs and whether it was added, removed or changed.
func jsonDiff(path string, a, b any) []diffEntry {
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if aok && bok {
		var out []diffEntry
		for k, av := range am {
			bv, ok := bm[k]
			switch {
			case !ok:
				out = append(out, diffEntry{"removed " + k, path + "/" + k})
			case k == "properties":
				out = append(out, propsDiff(path+"/properties", av, bv)...)
			default:
				out = append(out, jsonDiff(path+"/"+k, av, bv)...)
			}
		}
		for k := range bm {
			if _, ok := am[k]; !ok {
				out = append(out, diffEntry{"added " + k, path + "/" + k})
			}
		}
		return out
	}
	if reflect.DeepEqual(a, b) {
		return nil
	}
	key := path[strings.LastIndex(path, "/")+1:]
	return []diffEntry{{"changed " + key, path}}
}

func propsDiff(path string, a, b any) []diffEntry {
	am, _ := a.(map[string]any)
	bm, _ := b.(map[string]any)
	var out []diffEntry
	for k, av := range am {
		if bv, ok := bm[k]; ok {
			out = append(out, jsonDiff(path+"/"+k, av, bv)...)
		} else {
			out = append(out, diffEntry{"property removed", path + "/" + k})
		}
	}
	for k := range bm {
		if _, ok := am[k]; !ok {
			out = append(out, diffEntry{"property added", path + "/" + k})
		}
	}
	return out
}
