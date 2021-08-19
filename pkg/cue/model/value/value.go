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

package value

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/parser"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
)

// Value is an object with cue.runtime and vendors
type Value struct {
	v  cue.Value
	r  cue.Runtime
	pd *packages.PackageDiscover
}

// String return value's cue format string
func (val *Value) String() (string, error) {
	return sets.ToString(val.v, sets.OptBytesToString)
}

// Error return value's error information.
func (val *Value) Error() error {
	v := val.CueValue()
	if !v.Exists() {
		return errors.New("empty value")
	}
	var gerr error
	v.Walk(func(value cue.Value) bool {
		if err := value.Eval().Err(); err != nil {
			gerr = err
			return false
		}
		return true
	}, nil)
	return gerr
}

// UnmarshalTo unmarshal value into golang object
func (val *Value) UnmarshalTo(x interface{}) error {
	data, err := val.v.MarshalJSON()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, x)
}

// NewValue new a value
func NewValue(s string, pd *packages.PackageDiscover, opts ...func(*ast.File) error) (*Value, error) {
	builder := &build.Instance{}

	file, err := parser.ParseFile("-", s, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		if err := opt(file); err != nil {
			return nil, err
		}
	}
	if err := builder.AddSyntax(file); err != nil {
		return nil, err
	}

	if pd != nil {
		if _, err := pd.ImportPackagesAndBuildInstance(builder); err != nil {
			return nil, err
		}
	}

	var r cue.Runtime
	inst, err := r.Build(builder)
	if err != nil {
		return nil, err
	}
	val := new(Value)
	val.r = r
	val.v = inst.Value()
	val.pd = pd
	return val, nil
}

// TagFieldOrder add step tag.
func TagFieldOrder(root *ast.File) error {
	i := 0
	vs := &visitor{
		r: map[string]struct{}{},
	}
	for _, decl := range root.Decls {
		vs.addAttrForExpr(decl, &i)
	}
	return nil
}

func ProcessScript(root *ast.File) error {
	return sets.PreprocessBuiltinFunc(root, "script", func(values []ast.Node) (ast.Expr, error) {
		for _, v := range values {
			lit, ok := v.(*ast.BasicLit)
			if ok {
				src, err := literal.Unquote(lit.Value)
				if err != nil {
					return nil, errors.WithMessage(err, "unquote script value")
				}
				expr, err := parser.ParseExpr("-", src)
				if err != nil {
					return nil, errors.Errorf("script value(%s) is invalid CueLang", src)
				}
				return expr, nil
			}
		}
		return nil, errors.New("script parameter error")
	})
}

type visitor struct {
	r map[string]struct{}
}

func (vs *visitor) done(name string) {
	vs.r[name] = struct{}{}
}

func (vs *visitor) shouldDo(name string) bool {
	_, ok := vs.r[name]
	return !ok
}
func (vs *visitor) addAttrForExpr(node ast.Node, index *int) {
	switch v := node.(type) {
	case *ast.Comprehension:
		st := v.Value.(*ast.StructLit)
		for _, elt := range st.Elts {
			vs.addAttrForExpr(elt, index)
		}
	case *ast.Field:
		basic, ok := v.Label.(*ast.Ident)
		if !ok {
			return
		}
		if !vs.shouldDo(basic.Name) {
			return
		}
		if v.Attrs == nil {
			*index++
			vs.done(basic.Name)
			v.Attrs = []*ast.Attribute{
				{Text: fmt.Sprintf("@step(%d)", *index)},
			}
		}
	}
}

// MakeValue generate an value with same runtime
func (val *Value) MakeValue(s string) (*Value, error) {
	builder := &build.Instance{}
	if err := builder.AddFile("-", s); err != nil {
		return nil, err
	}
	if val.pd != nil {
		if _, err := val.pd.ImportPackagesAndBuildInstance(builder); err != nil {
			return nil, err
		}
	}
	inst, err := val.r.Build(builder)
	if err != nil {
		return nil, err
	}
	v := new(Value)
	v.r = val.r
	v.v = inst.Value()
	v.pd = val.pd
	return v, nil
}

// FillRaw unify the value with the cue format string x at the given path.
func (val *Value) FillRaw(x string, paths ...string) error {
	xInst, err := val.r.Compile("-", x)
	if err != nil {
		return err
	}
	v := val.v.Fill(xInst.Value(), paths...)
	if v.Err() != nil {
		return v.Err()
	}
	val.v = v
	return nil
}

// CueValue return cue.Value
func (val *Value) CueValue() cue.Value {
	return val.v
}

// FillObject unify the value with object x at the given path.
func (val *Value) FillObject(x interface{}, paths ...string) error {
	insert := x
	if v, ok := x.(*Value); ok {
		if v.r != val.r {
			return errors.New("filled value not created with same Runtime")
		}
		insert = v.v
	}
	newV := val.v.Fill(insert, paths...)
	if newV.Err() != nil {
		return newV.Err()
	}
	val.v = newV
	return nil
}

// LookupValue reports the value at a path starting from val
func (val *Value) LookupValue(paths ...string) (*Value, error) {
	v := val.v.Lookup(paths...)
	if !v.Exists() {
		return nil, errors.Errorf("var(path=%s) not exist", strings.Join(paths, "."))
	}
	return &Value{
		v: v,
		r: val.r,
	}, nil
}

type field struct {
	Name  string
	Value *Value
	no    int64
}

// StepByList process item in list.
func (val *Value) StepByList(handle func(name string, in *Value) (bool, error)) error {
	iter, err := val.CueValue().List()
	if err != nil {
		return err
	}
	for iter.Next() {
		stop, err := handle(iter.Label(), &Value{
			v:  iter.Value(),
			r:  val.r,
			pd: val.pd,
		})
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	return nil
}

// StepByFields process the fields in order
func (val *Value) StepByFields(handle func(name string, in *Value) (bool, error)) error {
	for i := 0; ; i++ {
		field, end, err := val.fieldIndex(i)
		if err != nil {
			return err
		}

		if end {
			return nil
		}
		stop, err := handle(field.Name, field.Value)
		if err != nil {
			return errors.WithMessagef(err, "step %s", field.Name)
		}
		if stop {
			return nil
		}

		if !isDef(field.Name) {
			if err := val.FillObject(field.Value, field.Name); err != nil {
				return err
			}
		}

		if end {
			break
		}
	}
	return nil
}

func (val *Value) fieldIndex(index int) (*field, bool, error) {
	fields, err := val.fields()
	if err != nil {
		return nil, false, err
	}
	if index >= len(fields) {
		return nil, true, nil
	}
	return fields[index], false, nil
}

func (val *Value) fields() ([]*field, error) {
	st, err := val.v.Struct()
	if err != nil {
		return nil, err
	}
	var fields []*field
	for i := 0; i < st.Len(); i++ {
		v := st.Field(i)
		attr := v.Value.Attribute("step")
		no, err := attr.Int(0)
		if err != nil {
			no = 100
		}
		fields = append(fields, &field{
			no:   no,
			Name: v.Name,
			Value: &Value{
				r: val.r,
				v: v.Value,
			}})
	}
	sort.Sort(sortFields(fields))
	return fields, nil
}

type sortFields []*field

func (sf sortFields) Len() int {
	return len(sf)
}
func (sf sortFields) Less(i, j int) bool {
	return sf[i].no < sf[j].no
}

func (sf sortFields) Swap(i, j int) {
	sf[i], sf[j] = sf[j], sf[i]
}

// Field return the cue value corresponding to the specified field
func (val *Value) Field(label string) (cue.Value, error) {
	var v cue.Value
	if isDef(label) {
		v = val.v.LookupDef(label)
	} else {
		v = val.v.Lookup(label)
	}

	if !v.Exists() {
		return v, errors.Errorf("label %s not found", label)
	}

	if v.IncompleteKind() == cue.BottomKind {
		return v, errors.Errorf("label %s's value not computed", label)
	}
	return v, nil
}

func isDef(s string) bool {
	return strings.HasPrefix(s, "#")
}
