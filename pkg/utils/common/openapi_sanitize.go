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

package common

import (
	"errors"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/encoding/openapi"
	"k8s.io/klog/v2"
)

var openAPIUnencodableOps = map[token.Token]bool{
	token.NEQ: true,
	token.GTR: true,
	token.LSS: true,
	token.GEQ: true,
	token.LEQ: true,
}

// refineFunc narrows a compiled template down to the value the OpenAPI encoder
// should see. Callers supply their own because the two entry points differ.
type refineFunc func(cue.Value) (cue.Value, error)

func genOpenAPIWithFallback(val cue.Value, refine refineFunc, cfg *openapi.Config) ([]byte, error) {
	refined, err := refine(val)
	if err != nil {
		return nil, err
	}
	b, genErr := openapi.Gen(refined, cfg)
	if genErr == nil {
		return b, nil
	}
	if out, ok := genSanitized(val, refine, cfg, genErr); ok {
		return out, nil
	}
	return nil, genErr
}

// genSanitized never returns the sanitizer's own failure. The caller keeps the
// original encoder error so a rewrite that cannot help leaves diagnostics intact,
// including when cuelang panics part way through the rebuild.
func genSanitized(val cue.Value, refine refineFunc, cfg *openapi.Config, genErr error) (out []byte, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			klog.V(4).Infof("OpenAPI fallback panicked, keeping original error: %v", r)
			out, ok = nil, false
		}
	}()
	if val.Context() == nil {
		return nil, false
	}

	b, dropped, err := rewriteAndGen(val, refine, cfg)
	if err != nil {
		if !errors.Is(err, errNothingDropped) {
			klog.V(4).Infof("OpenAPI fallback failed, keeping original error: %v", err)
		}
		return nil, false
	}
	logDropped(dropped, genErr)
	return b, true
}

var errNothingDropped = errors.New("no unencodable constraint found")

func rewriteAndGen(val cue.Value, refine refineFunc, cfg *openapi.Config) ([]byte, []string, error) {
	sanitized, dropped, err := rewriteUnencodable(val)
	if err != nil {
		return nil, nil, err
	}
	if len(dropped) == 0 {
		return nil, nil, errNothingDropped
	}
	if refine != nil {
		if sanitized, err = refine(sanitized); err != nil {
			return nil, nil, err
		}
	}
	b, err := openapi.Gen(sanitized, cfg)
	if err != nil {
		return nil, nil, err
	}
	return b, dropped, nil
}

func logDropped(dropped []string, genErr error) {
	klog.Warningf("OpenAPI schema generated without constraints the encoder cannot represent: %s (original error: %v)",
		strings.Join(dropped, ", "), genErr)
}

// rewriteUnencodable replaces constraints that cuelang's OpenAPI encoder rejects
// with their plain base type. It rewrites the whole template so definitions the
// parameter block refers to stay in scope and resolve without inlining, which
// would discard pattern constraints and struct ellipsis on unrelated fields. CUE absorbs the base type into the constraint
// (`string & !=""` reduces to `!=""`), so the node is substituted rather than
// deleted, otherwise the field would be left with no type at all.
func rewriteUnencodable(val cue.Value) (cue.Value, []string, error) {
	f, err := asOpenAPIFile(val.Syntax(cue.All(), cue.Docs(true)))
	if err != nil {
		return cue.Value{}, nil, err
	}

	var dropped []string
	rewritten := astutil.Apply(f, nil, func(c astutil.Cursor) bool {
		u, ok := c.Node().(*ast.UnaryExpr)
		if !ok || !openAPIUnencodableOps[u.Op] {
			return true
		}
		lit, ok := u.X.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		dropped = append(dropped, fmt.Sprintf("%s (%s%s)", fieldPath(c), u.Op, lit.Value))
		c.Replace(ast.NewIdent(baseTypeOfLiteral(lit)))
		return true
	})
	if len(dropped) == 0 {
		return cue.Value{}, nil, nil
	}

	out, ok := rewritten.(*ast.File)
	if !ok {
		return cue.Value{}, nil, fmt.Errorf("unexpected node %T after rewrite", rewritten)
	}
	sanitized := val.Context().BuildFile(out)
	if sanitized.Err() != nil {
		return cue.Value{}, nil, sanitized.Err()
	}
	return sanitized, dropped, nil
}

// baseTypeOfLiteral reports the CUE base type a quoted literal belongs to. CUE
// tokenizes both string and bytes literals as token.STRING and distinguishes
// them by quote style, so a bytes constraint must not be replaced with `string`.
func baseTypeOfLiteral(lit *ast.BasicLit) string {
	if strings.HasPrefix(lit.Value, "'") {
		return "__bytes"
	}
	return "__string"
}

func fieldPath(c astutil.Cursor) string {
	var parts []string
	for cur := c; cur != nil; cur = cur.Parent() {
		f, ok := cur.Node().(*ast.Field)
		if !ok {
			continue
		}
		name, _, err := ast.LabelName(f.Label)
		if err != nil || name == "" {
			continue
		}
		parts = append(parts, name)
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	path := strings.Join(parts, ".")
	for _, prefix := range []string{"#parameter.", "parameter."} {
		path = strings.TrimPrefix(path, prefix)
	}
	if path == "" {
		return "<parameter>"
	}
	return path
}

func asOpenAPIFile(n ast.Node) (*ast.File, error) {
	switch x := n.(type) {
	case *ast.File:
		return x, nil
	case *ast.StructLit:
		return &ast.File{Decls: x.Elts}, nil
	case ast.Expr:
		return &ast.File{Decls: []ast.Decl{&ast.EmbedDecl{Expr: x}}}, nil
	}
	return nil, fmt.Errorf("unexpected node %T", n)
}
