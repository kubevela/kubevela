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

	"github.com/oam-dev/kubevela/pkg/cue/model"
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
	targetRequiredSecrets := []RequiredSecrets{{
		ContextName: "conn1",
		Data:        map[string]interface{}{"password": "123"},
	}}
	targetParams := map[string]interface{}{
		"parameter1": "string",
		"parameter2": map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
		"parameter3": []string{"item1", "item2"},
	}
	targetData := map[string]interface{}{
		"int":    10,
		"string": "mytxt",
		"bool":   false,
		"map": map[string]interface{}{
			"key": "value",
		},
		"slice": []string{
			"str1", "str2", "str3",
		},
	}
	targetArbitraryData := map[string]interface{}{
		"int":    10,
		"string": "mytxt",
		"bool":   false,
		"map": map[string]interface{}{
			"key": "value",
		},
		"slice": []string{
			"str1", "str2", "str3",
		},
	}

	ctx := NewContext("myns", "mycomp", "myapp", "myapp-v1")
	ctx.InsertSecrets("db-conn", targetRequiredSecrets)
	ctx.SetBase(base)
	ctx.AppendAuxiliaries(svcAux)
	ctx.SetParameters(targetParams)
	ctx.PushData(ContextDataArtifacts, targetData)
	ctx.PushData("arbitraryData", targetArbitraryData)

	ctxInst, err := r.Compile("-", ctx.ExtendedContextFile())
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

	myAppRevisionNum, err := ctxInst.Lookup("context", ContextAppRevisionNum).Int64()
	assert.Equal(t, nil, err)
	assert.Equal(t, int64(1), myAppRevisionNum)

	inputJs, err := ctxInst.Lookup("context", OutputFieldName).MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, `{"image":"myserver"}`, string(inputJs))

	outputsJs, err := ctxInst.Lookup("context", OutputsFieldName, "service").MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, "{\"apiVersion\":\"v1\",\"kind\":\"ConfigMap\"}", string(outputsJs))

	ns, err := ctxInst.Lookup("context", ContextNamespace).String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "myns", ns)

	requiredSecrets, err := ctxInst.Lookup("context", "conn1").MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, "{\"password\":\"123\"}", string(requiredSecrets))

	params, err := ctxInst.Lookup("context", ParametersFieldName).MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, "{\"parameter1\":\"string\",\"parameter2\":{\"key1\":\"value1\",\"key2\":\"value2\"},\"parameter3\":[\"item1\",\"item2\"]}", string(params))

	artifacts, err := ctxInst.Lookup("context", ContextDataArtifacts).MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, "{\"bool\":false,\"string\":\"mytxt\",\"int\":10,\"map\":{\"key\":\"value\"},\"slice\":[\"str1\",\"str2\",\"str3\"]}", string(artifacts))

	arbitraryData, err := ctxInst.Lookup("context", "arbitraryData").MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, "{\"bool\":false,\"string\":\"mytxt\",\"int\":10,\"map\":{\"key\":\"value\"},\"slice\":[\"str1\",\"str2\",\"str3\"]}", string(arbitraryData))
}
