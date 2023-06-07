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
	"context"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/kubevela/pkg/cue/cuex/model/sets"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	wfContext "github.com/kubevela/workflow/pkg/context"
	"github.com/kubevela/workflow/pkg/cue/model/value"
)

// NewViewContext new view context
func NewViewContext() wfContext.Context {
	return &ViewContext{vars: cuecontext.New().CompileString("")}
}

// ViewContext is view context
type ViewContext struct {
	vars cue.Value
}

// GetVar get variable from workflow context.
func (c ViewContext) GetVar(paths ...string) (cue.Value, error) {
	v := c.vars.LookupPath(value.FieldPath(paths...))
	if !v.Exists() {
		return v, fmt.Errorf("var %s not found", strings.Join(paths, "."))
	}
	return v, nil
}

// SetVar set variable to workflow context.
func (c ViewContext) SetVar(v cue.Value, paths ...string) error {
	// convert value to string to set
	str, err := sets.ToString(v)
	if err != nil {
		return err
	}

	c.vars, err = value.FillRaw(c.vars, str, paths...)
	if err != nil {
		return err
	}
	if err := c.vars.Err(); err != nil {
		return err
	}
	return nil
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
func (c ViewContext) Commit(ctx context.Context) error {
	return errors.New("not support func Commit")
}

// StoreRef return the store reference of workflow context.
func (c ViewContext) StoreRef() *corev1.ObjectReference {
	return nil
}
