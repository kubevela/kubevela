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

	"cuelang.org/go/cue/cuecontext"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
)

const (
	// TagPatchKey specify the primary key of the list items
	TagPatchKey = "patchKey"
	// TagPatchStrategy specify a strategy of the strategic merge patch
	TagPatchStrategy = "patchStrategy"

	// StrategyRetainKeys notes on the strategic merge patch using the retainKeys strategy
	StrategyRetainKeys = "retainKeys"
	// StrategyReplace notes on the strategic merge patch will allow replacing list
	StrategyReplace = "replace"
	// StrategyJSONPatch notes on the strategic merge patch will follow the RFC 6902 to run JsonPatch
	StrategyJSONPatch = "jsonPatch"
	// StrategyJSONMergePatch notes on the strategic merge patch will follow the RFC 7396 to run JsonMergePatch
	StrategyJSONMergePatch = "jsonMergePatch"
)

var (
	notFoundErr = errors.Errorf("not found")
)

// UnifyParams params for unify
type UnifyParams struct {
	PatchStrategy string
}

// UnifyOption defines the option for unify
type UnifyOption interface {
	ApplyToOption(params *UnifyParams)
}

// UnifyByJSONPatch unify by json patch following RFC 6902
type UnifyByJSONPatch struct{}

// ApplyToOption apply to option
func (op UnifyByJSONPatch) ApplyToOption(params *UnifyParams) {
	params.PatchStrategy = StrategyJSONPatch
}

// UnifyByJSONMergePatch unify by json patch following RFC 7396
type UnifyByJSONMergePatch struct{}

// ApplyToOption apply to option
func (op UnifyByJSONMergePatch) ApplyToOption(params *UnifyParams) {
	params.PatchStrategy = StrategyJSONMergePatch
}

func newUnifyParams(options ...UnifyOption) *UnifyParams {
	params := &UnifyParams{}
	for _, op := range options {
		op.ApplyToOption(params)
	}
	return params
}

// CreateUnifyOptionsForPatcher create unify options for patcher
func CreateUnifyOptionsForPatcher(patcher cue.Value) (options []UnifyOption) {
	if IsJSONPatch(patcher) {
		options = append(options, UnifyByJSONPatch{})
	} else if IsJSONMergePatch(patcher) {
		options = append(options, UnifyByJSONMergePatch{})
	}
	return
}

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

// IsJSONMergePatch check if patcher is json merge patch
func IsJSONMergePatch(patcher cue.Value) bool {
	tags := findCommentTag(patcher.Doc())
	return tags[TagPatchStrategy] == StrategyJSONMergePatch
}

// IsJSONPatch check if patcher is json patch
func IsJSONPatch(patcher cue.Value) bool {
	tags := findCommentTag(patcher.Doc())
	return tags[TagPatchStrategy] == StrategyJSONPatch
}

// StrategyUnify unify the objects by the strategy
func StrategyUnify(base, patch string, options ...UnifyOption) (ret string, err error) {
	params := newUnifyParams(options...)
	var patchOpts []interceptor
	if params.PatchStrategy == StrategyJSONMergePatch || params.PatchStrategy == StrategyJSONPatch {
		base, err = OpenBaiscLit(base)
		if err != nil {
			return base, err
		}
	} else {
		patchOpts = []interceptor{strategyPatchHandle()}
	}
	baseFile, err := parser.ParseFile("-", base, parser.ParseComments)
	if err != nil {
		return "", errors.WithMessage(err, "invalid base cue file")
	}
	patchFile, err := parser.ParseFile("-", patch, parser.ParseComments)
	if err != nil {
		return "", errors.WithMessage(err, "invalid patch cue file")
	}

	return strategyUnify(baseFile, patchFile, params, patchOpts...)
}

func strategyUnify(baseFile *ast.File, patchFile *ast.File, params *UnifyParams, patchOpts ...interceptor) (string, error) {
	for _, option := range patchOpts {
		if err := option(baseFile, patchFile); err != nil {
			return "", errors.WithMessage(err, "process patchOption")
		}
	}

	r := cuecontext.New()

	baseInst := r.BuildFile(baseFile)
	patchInst := r.BuildFile(patchFile)
	if params.PatchStrategy == StrategyJSONMergePatch {
		return jsonMergePatch(baseInst.Value(), patchInst.Value())
	} else if params.PatchStrategy == StrategyJSONPatch {
		return jsonPatch(baseInst.Value(), patchInst.Lookup("operations"))
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

func jsonMergePatch(base cue.Value, patch cue.Value) (string, error) {
	baseJSON, err := base.MarshalJSON()
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal base value")
	}
	patchJSON, err := patch.MarshalJSON()
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal patch value")
	}
	merged, err := jsonpatch.MergePatch(baseJSON, patchJSON)
	if err != nil {
		return "", errors.Wrapf(err, "failed to merge base value and patch value by JsonMergePatch")
	}
	return string(merged), nil
}

func jsonPatch(base cue.Value, patch cue.Value) (string, error) {
	baseJSON, err := base.MarshalJSON()
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal base value")
	}
	patchJSON, err := patch.MarshalJSON()
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal patch value")
	}
	decodedPatch, err := jsonpatch.DecodePatch(patchJSON)
	if err != nil {
		return "", errors.Wrapf(err, "failed to decode patch")
	}

	merged, err := decodedPatch.Apply(baseJSON)
	if err != nil {
		return "", errors.Wrapf(err, "failed to apply json patch")
	}
	return string(merged), nil
}
