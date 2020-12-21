package model

import (
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/token"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/pkg/dsl/model/sets"
)

// Instance defines Model Interface
type Instance interface {
	String() string
	Object(m *runtime.Scheme) (runtime.Object, error)
	IsBase() bool
	Unify(other Instance) error
}

type instance struct {
	v    string
	base bool
}

// String return instance's cue format string
func (inst *instance) String() string {
	return inst.v
}

// IsBase indicate whether the instance is base model
func (inst *instance) IsBase() bool {
	return inst.base
}

// Object convert to runtime.Object
func (inst *instance) Object(m *runtime.Scheme) (runtime.Object, error) {
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

	if m != nil {
		object, err := m.New(o.GetObjectKind().GroupVersionKind())
		if err == nil {
			if err := json.Unmarshal(jsonv, object); err != nil {
				return nil, err
			}
			return object, nil
		}
		if !runtime.IsNotRegisteredError(err) {
			return nil, err
		}
	}

	return o, nil
}

// Unify implement unity operations between instances
func (inst *instance) Unify(other Instance) error {
	pv, err := sets.StrategyUnify(inst.v, other.String())
	if err != nil {
		return err
	}
	inst.v = pv
	return nil
}

// NewBase create a base instance
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

// NewOther create a non-base instance
func NewOther(v cue.Value) (Instance, error) {
	vs, err := openPrint(v)
	if err != nil {
		return nil, err
	}
	return &instance{
		v: vs,
	}, nil
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
	sysopts := []cue.Option{cue.All(), cue.DisallowCycles(true), cue.ResolveReferences(true), cue.Docs(true)}
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
