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

package process

import (
	"testing"

	"cuelang.org/go/cue"
	"github.com/bmizerany/assert"

	"github.com/oam-dev/kubevela/pkg/dsl/model"
)

func TestContext(t *testing.T) {
	baseTemplate := `
image: "myserver"
`

	var r cue.Runtime
	inst, err := r.Compile("-", baseTemplate)
	if err != nil {
		t.Error(err)
		return
	}
	base, err := model.NewBase(inst.Value())
	if err != nil {
		t.Error(err)
		return
	}

	serviceTemplate := `
	apiVersion: "v1"
    kind:       "ConfigMap"
`

	svcInst, err := r.Compile("-", serviceTemplate)
	if err != nil {
		t.Error(err)
		return
	}

	svcIns, err := model.NewOther(svcInst.Value())
	if err != nil {
		t.Error(err)
		return
	}

	svcAux := Auxiliary{
		Ins:  svcIns,
		Name: "service",
	}

	ctx := NewContext("mycomp", "myapp", "myapp-v1", "", nil)
	ctx.SetBase(base)
	ctx.AppendAuxiliaries(svcAux)

	ctxInst, err := r.Compile("-", ctx.BaseContextFile())
	if err != nil {
		t.Error(err)
		return
	}

	gName, err := ctxInst.Lookup("context", ContextName).String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "mycomp", gName)

	myAppName, err := ctxInst.Lookup("context", ContextAppName).String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "myapp", myAppName)

	myAppRevision, err := ctxInst.Lookup("context", ContextAppRevision).String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "myapp-v1", myAppRevision)

	inputJs, err := ctxInst.Lookup("context", OutputFieldName).MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, `{"image":"myserver"}`, string(inputJs))

	outputsJs, err := ctxInst.Lookup("context", OutputsFieldName, "service").MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, "{\"apiVersion\":\"v1\",\"kind\":\"ConfigMap\"}", string(outputsJs))
}
