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

package registry

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/utils/util"
)

var (
	tasks = map[string]Task{}
)

// Task process app-file
type Task func(ctx CallCtx, params interface{}) error

// RegisterTask register task for appfile
func RegisterTask(name string, task Task) {
	tasks[name] = task
}

func GetTasks() map[string]Task {
	return tasks
}

// CallCtx is task handle context
type CallCtx interface {
	LookUp(...string) (interface{}, error)
	IO() util.IOStreams
}

type callContext struct {
	data      map[string]interface{}
	ioStreams util.IOStreams
}

// IO return io streams handler
func (ctx *callContext) IO() util.IOStreams {
	return ctx.ioStreams
}

// LookUp find value by paths
func (ctx *callContext) LookUp(paths ...string) (interface{}, error) {
	var walkData interface{} = ctx.data

	for _, path := range paths {
		walkData = lookup(walkData, path)
		if walkData == nil {
			return nil, errors.Errorf("lookup field '%s' : not found", strings.Join(paths, "."))
		}
	}
	return walkData, nil
}

func lookup(v interface{}, key string) interface{} {
	val, ok := v.(map[string]interface{})
	if ok {
		return val[key]
	}
	return nil
}

func newCallCtx(io util.IOStreams, data map[string]interface{}) CallCtx {
	return &callContext{
		ioStreams: io,
		data:      data,
	}
}

// Run executes tasks
// Deprecated: Run is deprecated, you should use DoTasks is builtin package, it will automatically register all internal functions
func Run(spec map[string]interface{}, io util.IOStreams) (map[string]interface{}, error) {
	var (
		ctx     = newCallCtx(io, spec)
		retSpec = map[string]interface{}{}
	)

	tasks := GetTasks()

	for key, params := range spec {
		if do, ok := tasks[key]; ok {
			if err := do(ctx, params); err != nil {
				return nil, errors.WithMessagef(err, "do task %s", key)
			}
		} else {
			retSpec[key] = params
		}
	}
	return retSpec, nil
}
