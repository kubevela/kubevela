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

package model

import (
	"cuelang.org/go/cue"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
)

// Instance defines Model Interface
type Instance interface {
	String() (string, error)
	Value() cue.Value
	Unstructured() (*unstructured.Unstructured, error)
	IsBase() bool
	Unify(other cue.Value, options ...sets.UnifyOption) error
	Compile() ([]byte, error)
}

type instance struct {
	v    cue.Value
	base bool
}

// String return instance's cue format string
func (inst *instance) String() (string, error) {
	return sets.ToString(inst.v)
}

func (inst *instance) Value() cue.Value {
	return inst.v
}

// IsBase indicate whether the instance is base model
func (inst *instance) IsBase() bool {
	return inst.base
}

func (inst *instance) Compile() ([]byte, error) {
	if err := inst.v.Err(); err != nil {
		return nil, err
	}
	// compiled object should be final and concrete value
	if err := inst.v.Validate(cue.Concrete(true), cue.Final()); err != nil {
		return nil, err
	}
	return inst.v.MarshalJSON()
}

// Unstructured convert cue values to unstructured.Unstructured
// TODO(wonderflow): will it be better if we try to decode it to concrete object(such as K8s Deployment) by using runtime.Schema?
func (inst *instance) Unstructured() (*unstructured.Unstructured, error) {
	jsonv, err := inst.Compile()
	if err != nil {
		klog.ErrorS(err, "failed to have the workload/trait unstructured", "Definition", inst.v)
		return nil, errors.Wrap(err, "failed to have the workload/trait unstructured")
	}
	o := &unstructured.Unstructured{}
	if err := o.UnmarshalJSON(jsonv); err != nil {
		return nil, err
	}
	return o, nil
}

// Unify implement unity operations between instances
func (inst *instance) Unify(other cue.Value, options ...sets.UnifyOption) error {
	pv, err := sets.StrategyUnify(inst.v, other, options...)
	if err != nil {
		return err
	}
	inst.v = pv
	return nil
}

// NewBase create a base instance
func NewBase(v cue.Value) (Instance, error) {
	return &instance{
		v:    v,
		base: true,
	}, nil
}

// NewOther create a non-base instance
func NewOther(v cue.Value) (Instance, error) {
	return &instance{
		v: v,
	}, nil
}
