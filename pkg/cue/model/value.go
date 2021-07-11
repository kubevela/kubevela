package model

import (
	"cuelang.org/go/cue"
	"encoding/json"
	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
	"github.com/pkg/errors"
	"strings"
)

type Value struct {
	v cue.Value
	r cue.Runtime
}

func (val *Value) String() (string, error) {
	return sets.ToString(val.v)
}

func (val *Value) UnmarshalTo(x interface{}) error {
	data, err := val.v.MarshalJSON()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, x)
}

func NewValue(s string) (*Value, error) {
	var r cue.Runtime
	inst, err := r.Compile("-", s)
	if err != nil {
		return nil, err
	}
	val := new(Value)
	val.r = r
	val.v = inst.Value()
	return val, nil
}

func (val *Value) MakeValue(s string) (*Value, error) {
	inst, err := val.r.Compile("-", s)
	if err != nil {
		return nil, err
	}
	v := new(Value)
	v.r = val.r
	v.v = inst.Value()
	return v, nil
}

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

func (val *Value) FillObject(x interface{}, paths ...string) error {
	if v, ok := x.(*Value); ok {
		if v.r != val.r {
			return errors.New("filled value not created with same Runtime")
		}
		val.v = val.v.Fill(v.v, paths...)
		return nil
	}
	val.v = val.v.Fill(x, paths...)
	return nil
}

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

type Filed struct {
	Name  string
	Value *Value
}

func (val *Value) ObjectFileds() ([]*Filed, error) {
	st, err := val.v.Struct()
	if err != nil {
		return nil, err
	}
	fileds := []*Filed{}
	for i := 0; i < st.Len(); i++ {
		v := st.Field(i).Value
		if _, err := v.Struct(); err != nil {
			continue
		}
		fileds = append(fileds, &Filed{
			Name: st.Field(i).Name,
			Value: &Value{
				r: val.r,
				v: st.Field(i).Value,
			},
		})
	}
	return fileds, nil
}

func (val *Value) WalkFields(handle func(in *Value) error) error {
	st, err := val.v.Struct()
	if err != nil {
		return err
	}
	for i := 0; i < st.Len(); i++ {
		field, err := val.fieldIndex(i)
		if err != nil {
			return err
		}
		if err:=handle(field.Value);err!=nil{
			return err
		}

		if field.Value.v.Kind()==cue.StructKind{
			if err:=val.FillObject(field.Value, field.Name);err!=nil{
				return err
			}
		}

	}
	return nil
}

func (val *Value) fieldIndex(index int) (*Filed, error) {
	st, err := val.v.Struct()
	if err != nil {
		return nil, err
	}
	if index >= st.Len() {
		return nil, errors.New("get value field by index overhead")
	}
	field := st.Field(index)
	return &Filed{
		Name: field.Name,
		Value: &Value{
			r: val.r,
			v: field.Value,
		}}, nil
}

func (val *Value) Filed(label string) (cue.Value, error) {
	var v cue.Value
	if isDef(label) {
		v = val.v.LookupDef(label)
	} else {
		v = val.v.Lookup(label)
	}
	if !v.Exists() || v.Kind() == cue.BottomKind {
		return v, errors.Errorf("label %s not exist", label)
	}
	return v, nil
}

func isDef(s string) bool {
	return strings.HasPrefix(s, "#")
}
