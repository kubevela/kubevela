/*
Copyright 2021 The KubeVela Authors.

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

package sets

import (
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"github.com/pkg/errors"
)

const (
	// TagPatchKey specify the primary key of the list items
	TagPatchKey = "patchKey"
	// TagPatchStrategy specify a strategy of the strategic merge patch
	TagPatchStrategy = "patchStrategy"

	// StrategyRetainKeys notes on the strategic merge patch using the retainKeys strategy
	StrategyRetainKeys = "retainKeys"
	// StrategyOpen notes on the strategic merge patch will allow any merge
	StrategyOpen = "open"
	// StrategyReplace notes on the strategic merge patch will allow replacing list
	StrategyReplace = "replace"
)

var (
	notFoundErr = errors.Errorf("not found")
)

type interceptor func(baseNode ast.Node, patchNode ast.Node) error

func listMergeProcess(field *ast.Field, key string, baseList, patchList *ast.ListLit) {
	kmaps := map[string]ast.Expr{}
	nElts := []ast.Expr{}

	for i, elt := range patchList.Elts {
		if _, ok := elt.(*ast.Ellipsis); ok {
			continue
		}
		nodev, err := lookUp(elt, strings.Split(key, ".")...)
		if err != nil {
			return
		}
		blit, ok := nodev.(*ast.BasicLit)
		if !ok {
			return
		}
		kmaps[blit.Value] = patchList.Elts[i]
	}

	hasStrategyRetainKeys := isStrategyRetainKeys(field)

	for i, elt := range baseList.Elts {
		if _, ok := elt.(*ast.Ellipsis); ok {
			continue
		}

		nodev, err := lookUp(elt, strings.Split(key, ".")...)
		if err != nil {
			return
		}
		blit, ok := nodev.(*ast.BasicLit)
		if !ok {
			return
		}

		if v, ok := kmaps[blit.Value]; ok {
			if hasStrategyRetainKeys {
				baseList.Elts[i] = ast.NewStruct()
			}
			nElts = append(nElts, v)
			delete(kmaps, blit.Value)
		} else {
			nElts = append(nElts, ast.NewStruct())
		}

	}

	for _, elt := range patchList.Elts {
		for _, v := range kmaps {
			if elt == v {
				nElts = append(nElts, v)
				break
			}
		}
	}

	nElts = append(nElts, &ast.Ellipsis{})
	patchList.Elts = nElts
}

func strategyPatchHandle() interceptor {
	return func(baseNode ast.Node, patchNode ast.Node) error {
		walker := newWalker(func(node ast.Node, ctx walkCtx) {
			field, ok := node.(*ast.Field)
			if !ok {
				return
			}

			value := peelCloseExpr(field.Value)

			switch val := value.(type) {
			case *ast.ListLit:
				key := ctx.Tags()[TagPatchKey]
				patchStrategy := ""
				tags := findCommentTag(field.Comments())
				for tk, tv := range tags {
					if tk == TagPatchKey {
						key = tv
					}
					if tk == TagPatchStrategy {
						patchStrategy = tv
					}
				}

				paths := append(ctx.Pos(), labelStr(field.Label))
				baseSubNode, err := lookUp(baseNode, paths...)
				if err != nil {
					return
				}
				baselist, ok := baseSubNode.(*ast.ListLit)
				if !ok {
					return
				}
				if patchStrategy == StrategyReplace {
					baselist.Elts = val.Elts
				} else if key != "" {
					listMergeProcess(field, key, baselist, val)
				}

			default:
				if !isStrategyRetainKeys(field) {
					return
				}

				srcNode, _ := lookUp(baseNode, ctx.Pos()...)
				if srcNode != nil {
					switch v := srcNode.(type) {
					case *ast.StructLit:
						for _, elt := range v.Elts {
							if fe, ok := elt.(*ast.Field); ok &&
								labelStr(fe.Label) == labelStr(field.Label) {
								fe.Value = field.Value
							}
						}
					case *ast.File: // For the top level element
						for _, decl := range v.Decls {
							if fe, ok := decl.(*ast.Field); ok &&
								labelStr(fe.Label) == labelStr(field.Label) {
								fe.Value = field.Value
							}
						}
					}
				}
			}
		})
		walker.walk(patchNode)
		return nil
	}
}

func isStrategyRetainKeys(node *ast.Field) bool {
	tags := findCommentTag(node.Comments())
	for tk, tv := range tags {
		if tk == TagPatchStrategy && tv == StrategyRetainKeys {
			return true
		}
	}
	return false
}

// IsOpenPatch check if patcher has open annotation
func IsOpenPatch(patcher cue.Value) bool {
	tags := findCommentTag(patcher.Doc())
	for tk, tv := range tags {
		if tk == TagPatchStrategy && tv == StrategyOpen {
			return true
		}
	}
	return false
}

// StrategyUnify unify the objects by the strategy
func StrategyUnify(base, patch string) (string, error) {
	baseFile, err := parser.ParseFile("-", base, parser.ParseComments)
	if err != nil {
		return "", errors.WithMessage(err, "invalid base cue file")
	}
	patchFile, err := parser.ParseFile("-", patch, parser.ParseComments)
	if err != nil {
		return "", errors.WithMessage(err, "invalid patch cue file")
	}

	return strategyUnify(baseFile, patchFile, strategyPatchHandle())
}

func strategyUnify(baseFile *ast.File, patchFile *ast.File, patchOpts ...interceptor) (string, error) {
	for _, option := range patchOpts {
		if err := option(baseFile, patchFile); err != nil {
			return "", errors.WithMessage(err, "process patchOption")
		}
	}

	var r cue.Runtime

	baseInst, err := r.CompileFile(baseFile)
	if err != nil {
		return "", errors.WithMessage(err, "compile base file")
	}
	patchInst, err := r.CompileFile(patchFile)
	if err != nil {
		return "", errors.WithMessage(err, "compile patch file")
	}

	ret := baseInst.Value().Unify(patchInst.Value())

	rv, err := toString(ret)
	if err != nil {
		return rv, errors.WithMessage(err, " format result toString")
	}

	if err := ret.Err(); err != nil {
		return rv, errors.WithMessage(err, "result check err")
	}

	if err := ret.Validate(cue.All()); err != nil {
		return rv, errors.WithMessage(err, "result validate")
	}

	return rv, nil
}

func findCommentTag(commentGroup []*ast.CommentGroup) map[string]string {
	marker := "+"
	kval := map[string]string{}
	for _, group := range commentGroup {
		for _, lineT := range group.List {
			line := lineT.Text
			line = strings.TrimPrefix(line, "//")
			line = strings.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			if !strings.HasPrefix(line, marker) {
				continue
			}
			kv := strings.SplitN(line[len(marker):], "=", 2)
			if len(kv) == 2 {
				val := strings.TrimSpace(kv[1])
				if len(strings.Fields(val)) > 1 {
					continue
				}
				kval[strings.TrimSpace(kv[0])] = val
			}
		}
	}
	return kval
}
