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

	"cuelang.org/go/cue/cuecontext"
	"github.com/bmizerany/assert"

	"github.com/kubevela/workflow/pkg/cue/model"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/apis/types"
)

func TestContext(t *testing.T) {
	baseTemplate := `
image: "myserver"
`

	inst := cuecontext.New().CompileString(baseTemplate)
	base, err := model.NewBase(inst)
	if err != nil {
		t.Error(err)
		return
	}

	serviceTemplate := `
	apiVersion: "v1"
    kind:       "ConfigMap"
`

	svcInst := cuecontext.New().CompileString(serviceTemplate)

	svcIns, err := model.NewOther(svcInst)
	if err != nil {
		t.Error(err)
		return
	}

	svcAux := process.Auxiliary{
		Ins:  svcIns,
		Name: "service",
	}

	svcAuxWithAbnormalName := process.Auxiliary{
		Ins:  svcIns,
		Name: "service-1",
	}

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

	ctx := NewContext(ContextData{
		AppName:         "myapp",
		CompName:        "mycomp",
		Namespace:       "myns",
		AppRevisionName: "myapp-v1",
		WorkflowName:    "myworkflow",
		PublishVersion:  "mypublishversion",
	})
	ctx.SetBase(base)
	ctx.AppendAuxiliaries(svcAux)
	ctx.AppendAuxiliaries(svcAuxWithAbnormalName)
	ctx.SetParameters(targetParams)
	ctx.PushData(ContextDataArtifacts, targetData)
	ctx.PushData("arbitraryData", targetArbitraryData)

	c, err := ctx.BaseContextFile()
	if err != nil {
		t.Error(err)
		return
	}
	ctxInst := cuecontext.New().CompileString(c)

	gName, err := ctxInst.LookupPath(value.FieldPath("context", ContextName)).String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "mycomp", gName)

	myAppName, err := ctxInst.LookupPath(value.FieldPath("context", ContextAppName)).String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "myapp", myAppName)

	myAppRevision, err := ctxInst.LookupPath(value.FieldPath("context", ContextAppRevision)).String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "myapp-v1", myAppRevision)

	myAppRevisionNum, err := ctxInst.LookupPath(value.FieldPath("context", ContextAppRevisionNum)).Int64()
	assert.Equal(t, nil, err)
	assert.Equal(t, int64(1), myAppRevisionNum)

	myWorkflowName, err := ctxInst.LookupPath(value.FieldPath("context", ContextWorkflowName)).String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "myworkflow", myWorkflowName)

	myPublishVersion, err := ctxInst.LookupPath(value.FieldPath("context", ContextPublishVersion)).String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "mypublishversion", myPublishVersion)

	inputJs, err := ctxInst.LookupPath(value.FieldPath("context", OutputFieldName)).MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, `{"image":"myserver"}`, string(inputJs))

	outputsJs, err := ctxInst.LookupPath(value.FieldPath("context", OutputsFieldName, "service")).MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, "{\"apiVersion\":\"v1\",\"kind\":\"ConfigMap\"}", string(outputsJs))

	outputsJs, err = ctxInst.LookupPath(value.FieldPath("context", OutputsFieldName, "service-1")).MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, "{\"apiVersion\":\"v1\",\"kind\":\"ConfigMap\"}", string(outputsJs))

	ns, err := ctxInst.LookupPath(value.FieldPath("context", ContextNamespace)).String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "myns", ns)

	params, err := ctxInst.LookupPath(value.FieldPath("context", ParameterFieldName)).MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, "{\"parameter1\":\"string\",\"parameter2\":{\"key1\":\"value1\",\"key2\":\"value2\"},\"parameter3\":[\"item1\",\"item2\"]}", string(params))

	artifacts, err := ctxInst.LookupPath(value.FieldPath("context", ContextDataArtifacts)).MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, "{\"bool\":false,\"int\":10,\"map\":{\"key\":\"value\"},\"slice\":[\"str1\",\"str2\",\"str3\"],\"string\":\"mytxt\"}", string(artifacts))

	arbitraryData, err := ctxInst.LookupPath(value.FieldPath("context", "arbitraryData")).MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, "{\"bool\":false,\"int\":10,\"map\":{\"key\":\"value\"},\"slice\":[\"str1\",\"str2\",\"str3\"],\"string\":\"mytxt\"}", string(arbitraryData))
}

func TestParseClusterVersion(t *testing.T) {
	types.ControlPlaneClusterVersion = types.ClusterVersion{Minor: "18+"}
	got := parseClusterVersion(types.ClusterVersion{})
	assert.Equal(t, got["minor"], int64(18))

	types.ControlPlaneClusterVersion = types.ClusterVersion{Minor: "22-"}
	got = parseClusterVersion(types.ClusterVersion{})
	assert.Equal(t, got["minor"], int64(22))
}
