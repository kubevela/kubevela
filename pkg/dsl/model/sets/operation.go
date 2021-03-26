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
)

var (
	notFoundErr = errors.Errorf("not found")
)

type interceptor func(node ast.Node) (ast.Node, error)

func listMergeByKey(baseNode ast.Node) interceptor {
	return func(lnode ast.Node) (ast.Node, error) {
		walker := newWalker(func(node ast.Node, ctx walkCtx) {
			clist, ok := node.(*ast.ListLit)
			if !ok {
				return
			}
			key, ok := ctx.Tags()[TagPatchKey]
			if !ok {
				return
			}
			baseNode, err := lookUp(baseNode, ctx.Pos()...)
			if err != nil {
				return
			}
			baselist, ok := baseNode.(*ast.ListLit)
			if !ok {
				return
			}

			kmaps := map[string]ast.Expr{}
			nElts := []ast.Expr{}

			for i, elt := range clist.Elts {
				if _, ok := elt.(*ast.Ellipsis); ok {
					continue
				}
				nodev, err := lookUp(elt, key)
				if err != nil {
					return
				}
				blit, ok := nodev.(*ast.BasicLit)
				if !ok {
					return
				}
				kmaps[blit.Value] = clist.Elts[i]
			}
			for _, elt := range baselist.Elts {
				if _, ok := elt.(*ast.Ellipsis); ok {
					continue
				}

				nodev, err := lookUp(elt, key)
				if err != nil {
					return
				}
				blit, ok := nodev.(*ast.BasicLit)
				if !ok {
					return
				}

				if v, ok := kmaps[blit.Value]; ok {
					nElts = append(nElts, v)
					delete(kmaps, blit.Value)
				} else {
					nElts = append(nElts, ast.NewStruct())
				}

			}

			for _, elt := range clist.Elts {
				for _, v := range kmaps {
					if elt == v {
						nElts = append(nElts, v)
						break
					}
				}
			}

			nElts = append(nElts, &ast.Ellipsis{})
			clist.Elts = nElts
		})
		walker.walk(lnode)
		return lnode, nil
	}
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

	return strategyUnify(baseFile, patchFile, listMergeByKey(baseFile))
}

func strategyUnify(baseFile *ast.File, patchFile *ast.File, patchOpts ...interceptor) (string, error) {
	for _, option := range patchOpts {
		if _, err := option(patchFile); err != nil {
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
