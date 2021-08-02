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

package custom

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

func TestTaskLoader(t *testing.T) {

	wfCtx := newWorkflowContextForTest(t)
	discover := providers.NewProviders()
	discover.Register("test", map[string]providers.Handler{
		"output": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			ip, _ := v.MakeValue(`
myIP: "1.1.1.1"            
`)
			return v.FillObject(ip)
		},
		"input": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			val, err := v.LookupValue("prefixIP")
			assert.NilError(t, err)
			str, err := val.CueValue().String()
			assert.NilError(t, err)
			assert.Equal(t, str, "1.1.1.1")
			return nil
		},
		"wait": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			act.Wait("I am waiting")
			return nil
		},
		"terminate": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			act.Terminate("I am terminated")
			return nil
		},
		"executeFailed": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			return errors.New("execute error")
		},
		"ok": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			return nil
		},
	})

	tasksLoader := NewTaskLoader(mockLoadTemplate, nil, discover)

	steps := []v1beta1.WorkflowStep{
		{
			Name: "output",
			Type: "output",
			Outputs: v1beta1.StepOutputs{{
				ExportKey: "myIP",
				Name:      "podIP",
			}},
		},
		{
			Name: "input",
			Type: "input",
			Inputs: v1beta1.StepInputs{{
				From:         "podIP",
				ParameterKey: "prefixIP",
			}},
		},
		{
			Name: "wait",
			Type: "wait",
		},
		{
			Name: "terminate",
			Type: "terminate",
		},
		{
			Name: "rendering",
			Type: "renderFailed",
		},
		{
			Name: "execute",
			Type: "executeFailed",
		},
		{
			Name: "steps",
			Type: "steps",
		},
	}

	for _, step := range steps {
		gen, err := tasksLoader.GetTaskGenerator(context.Background(), step.Type)
		assert.NilError(t, err)
		run, err := gen(step)
		assert.NilError(t, err)
		status, action, err := run(wfCtx)
		assert.NilError(t, err)
		if step.Name == "wait" {
			assert.Equal(t, status.Phase, common.WorkflowStepPhaseRunning)
			assert.Equal(t, status.Reason, StatusReasonWait)
			assert.Equal(t, status.Message, "I am waiting")
			continue
		}
		if step.Name == "terminate" {
			assert.Equal(t, action.Terminated, true)
			assert.Equal(t, status.Reason, StatusReasonTerminate)
			assert.Equal(t, status.Message, "I am terminated")
			continue
		}
		if step.Name == "rendering" {
			assert.Equal(t, status.Phase, common.WorkflowStepPhaseFailed)
			assert.Equal(t, status.Reason, StatusReasonRendering)
			continue
		}
		if step.Name == "execute" {
			assert.Equal(t, status.Phase, common.WorkflowStepPhaseFailed)
			assert.Equal(t, status.Reason, StatusReasonExecute)
			continue
		}
		assert.Equal(t, status.Phase, common.WorkflowStepPhaseSucceeded)
	}

}

func TestErrCases(t *testing.T) {
	wfCtx := newWorkflowContextForTest(t)
	closeVar, err := value.NewValue(`
close({
   x: 100
})
`, nil)
	assert.NilError(t, err)
	err = wfCtx.SetVar(closeVar, "score")
	assert.NilError(t, err)
	discover := providers.NewProviders()
	discover.Register("test", map[string]providers.Handler{
		"input": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			val, err := v.LookupValue("prefixIP")
			assert.NilError(t, err)
			str, err := val.CueValue().String()
			assert.NilError(t, err)
			assert.Equal(t, str, "1.1.1.1")
			return nil
		},
		"ok": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			return nil
		},
		"error": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			return errors.New("mock error")
		},
	})
	tasksLoader := NewTaskLoader(mockLoadTemplate, nil, discover)

	steps := []v1beta1.WorkflowStep{
		{
			Name: "input-err",
			Type: "ok",
			Properties: runtime.RawExtension{Raw: []byte(`
{"score": {"y": 101}}
`)},
			Inputs: v1beta1.StepInputs{{
				From:         "score",
				ParameterKey: "score",
			}},
		},
		{
			Name: "input",
			Type: "input",
			Inputs: v1beta1.StepInputs{{
				From:         "podIP",
				ParameterKey: "prefixIP",
			}},
		},
		{
			Name: "output",
			Type: "ok",
			Outputs: v1beta1.StepOutputs{{
				Name:      "podIP",
				ExportKey: "myIP",
			}},
		},
		{
			Name: "output-var-conflict",
			Type: "ok",
			Outputs: v1beta1.StepOutputs{{
				Name:      "score",
				ExportKey: "name",
			}},
		},
		{
			Name: "wait",
			Type: "wait",
		},
		{
			Name: "err",
			Type: "error",
		},
	}
	for _, step := range steps {
		gen, err := tasksLoader.GetTaskGenerator(context.Background(), step.Type)
		assert.NilError(t, err)
		run, err := gen(step)
		assert.NilError(t, err)
		status, _, err := run(wfCtx)
		switch step.Name {
		case "input":
			assert.Equal(t, err != nil, true)
		case "ouput", "output-var-conflict":
			assert.Equal(t, status.Reason, StatusReasonOutput)
			assert.Equal(t, status.Phase, common.WorkflowStepPhaseFailed)
		default:
			assert.Equal(t, status.Phase, common.WorkflowStepPhaseFailed)
		}
	}
}

func TestNestedSteps(t *testing.T) {

	var (
		echo    string
		mockErr = errors.New("mock error")
	)

	wfCtx := newWorkflowContextForTest(t)
	discover := providers.NewProviders()
	discover.Register("test", map[string]providers.Handler{
		"ok": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			echo = echo + "ok"
			return nil
		},
		"error": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			return mockErr
		},
	})
	exec := &executor{
		handlers: discover,
	}

	testCases := []struct {
		base     string
		expected string
		hasErr   bool
	}{
		{
			base: `
process: {
	#provider: "test"
	#do: "ok"
}

#up: [process]
`,
			expected: "okok",
		},
		{
			base: `
process: {
	#provider: "test"
	#do: "ok"
}

#up: [process,{
  #do: "steps"
  p1: process
  #up: [process]
}]
`,
			expected: "okokokok",
		},
		{
			base: `
process: {
	#provider: "test"
	#do: "ok"
}

#up: [process,{
  p1: process
  #up: [process]
}]
`,
			expected: "okok",
		},
		{
			base: `
process: {
	#provider: "test"
	#do: "ok"
}

#up: [process,{
  #do: "steps"
  err: {
    #provider: "test"
	#do: "error"
  }
  #up_1: [{},process]
}]
`,
			expected: "okok",
			hasErr:   true,
		},

		{
			base: `
	#provider: "test"
	#do: "ok"
`,
			expected: "ok",
		},
	}

	for _, tc := range testCases {
		echo = ""
		v, err := value.NewValue(tc.base, nil)
		assert.NilError(t, err)
		err = exec.doSteps(wfCtx, v)
		assert.Equal(t, err != nil, tc.hasErr)
		assert.Equal(t, echo, tc.expected)
	}

}

func newWorkflowContextForTest(t *testing.T) wfContext.Context {
	cm := corev1.ConfigMap{}
	testCaseJson, err := yaml.YAMLToJSON([]byte(testCaseYaml))
	assert.NilError(t, err)
	err = json.Unmarshal(testCaseJson, &cm)
	assert.NilError(t, err)

	wfCtx := new(wfContext.WorkflowContext)
	err = wfCtx.LoadFromConfigMap(cm)
	assert.NilError(t, err)
	v, _ := value.NewValue(`name: "app"`, nil)
	assert.NilError(t, wfCtx.SetVar(v, types.ContextKeyMetadata))
	return wfCtx
}
func mockLoadTemplate(_ context.Context, name string) (string, error) {
	templ := `
parameter: {}
process: {
	#provider: "test"
	#do: "%s"
	parameter
}
// check injected context.
name: context.name
`
	switch name {
	case "output":
		return fmt.Sprintf(templ+`myIP: process.myIP`, "output"), nil
	case "input":
		return fmt.Sprintf(templ, "input"), nil
	case "wait":
		return fmt.Sprintf(templ, "wait"), nil
	case "terminate":
		return fmt.Sprintf(templ, "terminate"), nil
	case "renderFailed":
		return `
output: xx
`, nil
	case "executeFailed":
		return fmt.Sprintf(templ, "executeFailed"), nil
	case "ok":
		return fmt.Sprintf(templ, "ok"), nil
	case "error":
		return fmt.Sprintf(templ, "error"), nil
	case "steps":
		return `
#do: "steps"
ok: {
	#provider: "test"
	#do: "ok"
}
`, nil
	}

	return "", nil
}

var (
	testCaseYaml = `apiVersion: v1
data:
  components: '{"server":"{\"Scopes\":null,\"StandardWorkload\":\"{\\\"apiVersion\\\":\\\"v1\\\",\\\"kind\\\":\\\"Pod\\\",\\\"metadata\\\":{\\\"labels\\\":{\\\"app\\\":\\\"nginx\\\"}},\\\"spec\\\":{\\\"containers\\\":[{\\\"env\\\":[{\\\"name\\\":\\\"APP\\\",\\\"value\\\":\\\"nginx\\\"}],\\\"image\\\":\\\"nginx:1.14.2\\\",\\\"imagePullPolicy\\\":\\\"IfNotPresent\\\",\\\"name\\\":\\\"main\\\",\\\"ports\\\":[{\\\"containerPort\\\":8080,\\\"protocol\\\":\\\"TCP\\\"}]}]}}\",\"Traits\":[\"{\\\"apiVersion\\\":\\\"v1\\\",\\\"kind\\\":\\\"Service\\\",\\\"metadata\\\":{\\\"name\\\":\\\"my-service\\\"},\\\"spec\\\":{\\\"ports\\\":[{\\\"port\\\":80,\\\"protocol\\\":\\\"TCP\\\",\\\"targetPort\\\":8080}],\\\"selector\\\":{\\\"app\\\":\\\"nginx\\\"}}}\"]}"}'
kind: ConfigMap
metadata:
  name: app-v1
`
)
