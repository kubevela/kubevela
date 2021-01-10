package model

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/dsl/model/sets"
)

// Instance defines Model Interface
type Instance interface {
	String() string
	Unstructured() (*unstructured.Unstructured, error)
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

func (inst *instance) compile() ([]byte, error) {
	var r cue.Runtime
	cueInst, err := r.Compile("-", inst.v)
	if err != nil {
		return nil, err
	}

	return cueInst.Value().MarshalJSON()
}

// Unstructured convert cue values to unstructured.Unstructured
// TODO(wonderflow): will it be better if we try to decode it to concrete object(such as K8s Deployment) by using runtime.Schema?ß
func (inst *instance) Unstructured() (*unstructured.Unstructured, error) {
	jsonv, err := inst.compile()
	if err != nil {
		return nil, err
	}

	o := &unstructured.Unstructured{}

	if err := o.UnmarshalJSON(jsonv); err != nil {
		return nil, err
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

func openPrint(v cue.Value) (string, error) {
	sysopts := []cue.Option{cue.All(), cue.DisallowCycles(true), cue.ResolveReferences(true), cue.Docs(true)}
	f, err := sets.ToFile(v.Syntax(sysopts...))
	if err != nil {
		return "", nil
	}
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
