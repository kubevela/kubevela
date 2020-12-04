package model

import (
	"bytes"
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/token"
	"fmt"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"path/filepath"
)

type Instance interface {
	String() string
	Object() (*unstructured.Unstructured, error)
	IsBase() bool
	Unity(other Instance) error
}

type instance struct {
	v    string
	base bool
}

func (inst *instance) String() string {
	return inst.v
}

func (inst *instance) IsBase() bool {
	return inst.base
}

func (inst *instance) Object() (*unstructured.Unstructured, error) {
	var r cue.Runtime
	cueInst, err := r.Compile("-", inst.v)
	if err != nil {
		return nil, err
	}
	o := new(unstructured.Unstructured)
	jsonv, err := cueInst.Value().MarshalJSON()
	if err != nil {
		return nil, err
	}
	if err := o.UnmarshalJSON(jsonv); err != nil {
		return nil, err
	}
	return o, nil
}

func (inst *instance) Unity(other Instance) error {
	var r cue.Runtime
	raw, err := r.Compile("-", inst.v)
	if err != nil {
		return err
	}
	o, err := r.Compile("-", other.String())
	if err != nil {
		return err
	}
	pv, err := print(raw.Value().Unify(o.Value()))
	if err != nil {
		return err
	}
	inst.v = pv
	return nil
}

func NewBase(v cue.Value) (Instance, error) {
	vs, err := openPrint(v)
	if err != nil {
		return nil, err
	}
	return &instance{
		v:    vs,
		base: true,
	}, nil
}

func NewOther(v cue.Value) (Instance, error) {
	vs, err := openPrint(v)
	if err != nil {
		return nil, err
	}
	return &instance{
		v: vs,
	}, nil
}

func print(v cue.Value) (string, error) {
	v = v.Eval()
	syopts := []cue.Option{cue.All(), cue.DisallowCycles(true), cue.ResolveReferences(true)}

	var w bytes.Buffer
	useSep := false
	format := func(name string, n ast.Node) error {
		if name != "" {
			// TODO: make this relative to DIR
			fmt.Fprintf(&w, "// %s\n", filepath.Base(name))
		} else if useSep {
			fmt.Println("// ---")
		}
		useSep = true

		b, err := format.Node(toFile(n))
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}

	if err := format("", v.Syntax(syopts...)); err != nil {
		return "", err
	}
	inst_str := w.String()
	return inst_str, nil
}

func toFile(n ast.Node) *ast.File {
	switch x := n.(type) {
	case nil:
		return nil
	case *ast.StructLit:
		return &ast.File{Decls: x.Elts}
	case ast.Expr:
		ast.SetRelPos(x, token.NoSpace)
		return &ast.File{Decls: []ast.Decl{&ast.EmbedDecl{Expr: x}}}
	case *ast.File:
		return x
	default:
		panic(fmt.Sprintf("Unsupported node type %T", x))
	}
}

func openPrint(v cue.Value) (string, error) {
	sysopts := []cue.Option{cue.All(), cue.DisallowCycles(true), cue.ResolveReferences(true)}
	f := toFile(v.Syntax(sysopts...))
	for _, decl := range f.Decls {
		listOpen(decl)
	}
	ret, err := format.Node(f)
	return string(ret), err
}

func listOpen(expr ast.Node) {
	switch v := expr.(type) {
	case *ast.Field:
		listOpen(v.Value)
	case *ast.StructLit:
		for _, elt := range v.Elts {
			listOpen(elt)
		}
	case *ast.BinaryExpr:
		listOpen(v.X)
		listOpen(v.Y)
	case *ast.EmbedDecl:
		listOpen(v.Expr)
	case *ast.Comprehension:
		listOpen(v.Value)
	case *ast.ListLit:
		for _, elt := range v.Elts {
			listOpen(elt)
		}
		if len(v.Elts) > 0 {
			if _, ok := v.Elts[len(v.Elts)-1].(*ast.Ellipsis); !ok {
				v.Elts = append(v.Elts, &ast.Ellipsis{})
			}
		}
	}
}
