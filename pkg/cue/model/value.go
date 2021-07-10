package model

import (
	"cuelang.org/go/cue"
	"encoding/json"
	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
	"github.com/pkg/errors"
	"strings"
)

type Value struct {
	cue.Value
	r cue.Runtime
}

func (val *Value) Fmt() (string, error) {
	return sets.ToString(val.Value)
}

func (val *Value) UnmarshalTo(x interface{}) error {
	data, err := val.Value.MarshalJSON()
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
	val.Value = inst.Value()
	return val, nil
}

func (val *Value) FillRaw(x string, paths ...string) error {
	xInst, err := val.r.Compile("-", x)
	if err != nil {
		return err
	}
	v := val.Value.Fill(xInst.Value(), paths...)
	if v.Err() != nil {
		return v.Err()
	}
	val.Value = v
	return nil
}

func (val *Value) FillObject(x interface{}, paths ...string) error {
	val.Value = val.Value.Fill(x, paths...)
	return nil
}

func (val *Value) LookupValue(paths ...string) (*Value, error) {
	v := val.Value.Lookup(paths...)
	if !v.Exists() {
		return nil, errors.Errorf("var(path=%s) not exist", strings.Join(paths, "."))
	}
	return &Value{
		Value: v,
		r:     val.r,
	}, nil
}
