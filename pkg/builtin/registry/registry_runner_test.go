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
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/bmizerany/assert"
)

func TestContext(t *testing.T) {
	lpV := `test: "just a test"`
	inst := cuecontext.New().CompileString(lpV)
	ctx := Meta{Obj: inst}
	val := ctx.Lookup("test")
	assert.Equal(t, true, val.Exists())

	intV := `iTest: 64`
	iInst := cuecontext.New().CompileString(intV)
	iCtx := Meta{Obj: iInst.Value()}
	iVal := iCtx.Int64("iTest")
	assert.Equal(t, int64(64), iVal)
}

func TestRunner(t *testing.T) {
	key := "mock"
	RegisterRunner(key, newMockRunner)

	task := LookupRunner(key)
	if task == nil {
		t.Errorf("there is no task %s", key)
	}
	runner, err := task(cue.Value{})
	if err != nil {
		t.Errorf("fail to get runner, %v", err)
	}
	rs, err := runner.Run(&Meta{Obj: cue.Value{}})
	assert.Equal(t, nil, err)
	assert.Equal(t, "mock", rs)
}

func newMockRunner(v cue.Value) (Runner, error) {
	return &MockRunner{name: "mock"}, nil
}

type MockRunner struct {
	name string
}

func (r *MockRunner) Run(meta *Meta) (res interface{}, err error) {
	return r.name, nil
}
