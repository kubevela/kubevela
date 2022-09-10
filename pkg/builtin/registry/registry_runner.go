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
	"context"
	"fmt"
	"io"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/errors"

	"github.com/kubevela/workflow/pkg/cue/model/value"
)

// Meta provides context for running a task.
type Meta struct {
	Context context.Context
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Obj     cue.Value
	Err     error
}

// Lookup fetches the value of context by filed
func (m *Meta) Lookup(field string) cue.Value {
	f := m.Obj.LookupPath(value.FieldPath(field))
	if !f.Exists() {
		m.Err = fmt.Errorf("invalid lookup argument")
		return cue.Value{}
	}
	if err := f.Err(); err != nil {
		m.Err = errors.Promote(err, "lookup")
	}
	return f
}

// Int64 fetch the value formatted int64 of context by filed
func (m *Meta) Int64(field string) int64 {
	f := m.Obj.LookupPath(value.FieldPath(field))
	value, err := f.Int64()
	if err != nil {
		m.Err = fmt.Errorf("invalid int64 argument, %w", err)

		return 0
	}
	return value
}

// String fetch the value formatted string of context by filed
func (m *Meta) String(field string) string {
	f := m.Obj.LookupPath(value.FieldPath(field))
	value, err := f.String()
	if err != nil {
		m.Err = fmt.Errorf("invalid string argument, %w", err)
		return ""
	}
	return value
}

// Bytes fetch the value formatted bytes of context by filed
func (m *Meta) Bytes(field string) []byte {
	f := m.Obj.LookupPath(value.FieldPath(field))
	value, err := f.Bytes()
	if err != nil {
		m.Err = fmt.Errorf("invalid bytes argument, %w", err)
		return nil
	}
	return value
}

// RunnerFunc creates a Runner.
type RunnerFunc func(v cue.Value) (Runner, error)

// Runner defines a command type.
type Runner interface {
	// Init is called with the original configuration before any task is run.
	// As a result, the configuration may be incomplete, but allows some
	// validation before tasks are kicked off.
	// Init(v cue.Value)

	// Runner runs given the current value and returns a new value which is to
	// be unified with the original result.
	Run(meta *Meta) (results interface{}, err error)
}

// RegisterRunner registers a task for cue commands.
func RegisterRunner(key string, f RunnerFunc) {
	runners.Store(key, f)
}

// LookupRunner returns the RunnerFunc for a key.
func LookupRunner(key string) RunnerFunc {
	v, ok := runners.Load(key)
	if !ok {
		return nil
	}
	return v.(RunnerFunc)
}

var runners sync.Map
