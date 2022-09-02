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

package stdlib

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"gotest.tools/assert"

	"github.com/kubevela/workflow/pkg/stdlib"
)

func TestGetPackages(t *testing.T) {
	pkgs, err := getPackages()
	assert.NilError(t, err)
	cuectx := cuecontext.New()
	for path, content := range pkgs {
		file, err := parser.ParseFile(path, content)
		assert.NilError(t, err)
		_ = cuectx.BuildFile(file)
	}

	file, err := parser.ParseFile("-", `
import "vela/custom"
out: custom.context`)
	assert.NilError(t, err)
	builder := &build.Instance{}
	err = builder.AddSyntax(file)
	assert.NilError(t, err)
	err = stdlib.AddImportsFor(builder, "context: id: \"xxx\"")
	assert.NilError(t, err)

	inst := cuectx.BuildInstance(builder)
	str, err := inst.LookupPath(cue.ParsePath("out.id")).String()
	assert.NilError(t, err)
	assert.Equal(t, str, "xxx")
}
