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
	"encoding/json"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
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

// GetComponent Get ComponentManifest from workflow context.
func (c ViewContext) GetComponent(name string) (*wfContext.ComponentManifest, error) {
	return nil, errors.New("not support func GetComponent")
}

// GetComponents Get All ComponentManifest from workflow context.
func (c ViewContext) GetComponents() map[string]*wfContext.ComponentManifest {
	return nil
}

// PatchComponent patch component with value.
func (c ViewContext) PatchComponent(name string, patchValue *value.Value) error {
	return errors.New("not support func PatchComponent")
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

// GetData get data from workflow context config map.
func (c ViewContext) GetDataInConfigMap(paths ...string) string {
	// return c.store.Data[strings.Join(paths, ".")]
	return ""
}

// SetVar set variable to workflow context.
func (c ViewContext) SetDataInConfigMap(data string, paths ...string) {
	// c.store.Data[strings.Join(paths, ".")] = data
	// c.modified = true
}

// Commit the workflow context and persist it's content.
func (c ViewContext) Commit() error {
	return errors.New("not support func Commit")
}

// MakeParameter make 'value' with interface{}
func (c ViewContext) MakeParameter(parameter interface{}) (*value.Value, error) {
	var s = "{}"
	if parameter != nil {
		bt, err := json.Marshal(parameter)
		if err != nil {
			return nil, err
		}
		s = string(bt)
	}

	return c.vars.MakeValue(s)
}

// StoreRef return the store reference of workflow context.
func (c ViewContext) StoreRef() *corev1.ObjectReference {
	return nil
}
