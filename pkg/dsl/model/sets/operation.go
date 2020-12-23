package sets

import (
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"github.com/pkg/errors"
)

const (
	// TagPatchKey specify the primary key of the list items
	TagPatchKey = "patchKey"
)

var (
	notFoundErr = errors.Errorf("not found")
)

type interceptor func(value cue.Value) (cue.Value, error)

func strategyListMerge(base cue.Value, r cue.Runtime) interceptor {
	baseNode := convert2Node(base)
	return func(value cue.Value) (cue.Value, error) {
		lnode := convert2Node(value)
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
		f, err := toFile(lnode)
		if err != nil {
			return cue.Value{}, err
		}
		inst, err := r.CompileFile(f)
		if err != nil {
			return cue.Value{}, err
		}
		return inst.Value(), nil
	}
}

// StrategyUnify unify the objects by the strategy
func StrategyUnify(base, patch string) (string, error) {
	var r cue.Runtime
	raw, err := r.Compile("-", base)
	if err != nil {
		return "", err
	}
	var lr cue.Runtime
	o, err := lr.Compile("-", patch)
	if err != nil {
		return "", err
	}
	handle := strategyListMerge(raw.Value(), r)
	newOne, err := handle(o.Value())
	if err != nil {
		return "", err
	}
	ret := raw.Value().Unify(newOne).Eval()

	rv, err := toString(ret)
	if err != nil {
		return rv, err
	}

	if err := ret.Err(); err != nil {
		return rv, err
	}

	if err := ret.Validate(cue.All()); err != nil {
		return rv, err
	}

	if err := doordog(rv); err != nil {
		return rv, err
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
				kval[kv[0]] = kv[1]
			}
		}
	}
	return kval
}

func doordog(v string) error {
	lines := strings.Split(v, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "_|_") && !strings.HasPrefix(line, "//") {
			return errors.Errorf("bottom found <detail>: \n%s", v)
		}
	}
	return nil
}
