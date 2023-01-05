/*
 Copyright 2021. The KubeVela Authors.

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

package velaql

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	wfContext "github.com/kubevela/workflow/pkg/context"
	"github.com/kubevela/workflow/pkg/cue/model/value"
)

// NewViewContext new view context
func NewViewContext() (wfContext.Context, error) {
	viewContext := &ViewContext{}
	var err error
	viewContext.vars, err = value.NewValue("", nil, "")
	return viewContext, err
}

// ViewContext is view context
type ViewContext struct {
	vars *value.Value
}

// GetVar get variable from workflow context.
func (c ViewContext) GetVar(paths ...string) (*value.Value, error) {
	return c.vars.LookupValue(paths...)
}

// SetVar set variable to workflow context.
func (c ViewContext) SetVar(v *value.Value, paths ...string) error {
	str, err := v.String()
	if err != nil {
		return errors.WithMessage(err, "compile var")
	}
	if err := c.vars.FillRaw(str, paths...); err != nil {
		return err
	}
	return c.vars.Error()
}

// GetStore get configmap of workflow context.
func (c ViewContext) GetStore() *corev1.ConfigMap {
	return nil
}

// GetMutableValue get mutable data from workflow context.
func (c ViewContext) GetMutableValue(paths ...string) string {
	return ""
}

// SetMutableValue set mutable data in workflow context config map.
func (c ViewContext) SetMutableValue(data string, paths ...string) {
}

// IncreaseCountValueInMemory increase count in workflow context memory store.
func (c ViewContext) IncreaseCountValueInMemory(paths ...string) int {
	return 0
}

// SetValueInMemory set data in workflow context memory store.
func (c ViewContext) SetValueInMemory(data interface{}, paths ...string) {
}

// GetValueInMemory get data in workflow context memory store.
func (c ViewContext) GetValueInMemory(paths ...string) (interface{}, bool) {
	return "", true
}

// DeleteValueInMemory delete data in workflow context memory store.
func (c ViewContext) DeleteValueInMemory(paths ...string) {
}

// DeleteMutableValue delete mutable data in workflow context.
func (c ViewContext) DeleteMutableValue(paths ...string) {
}

// Commit the workflow context and persist it's content.
func (c ViewContext) Commit() error {
	return errors.New("not support func Commit")
}

// MakeParameter make 'value' with string
func (c ViewContext) MakeParameter(parameter string) (*value.Value, error) {
	if parameter == "" {
		parameter = "{}"
	}

	return c.vars.MakeValue(parameter)
}

// StoreRef return the store reference of workflow context.
func (c ViewContext) StoreRef() *corev1.ObjectReference {
	return nil
}
