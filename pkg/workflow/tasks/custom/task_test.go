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

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

func TestTaskLoader(t *testing.T) {
	wfCtx := newWorkflowContextForTest(t)
	r := require.New(t)
	discover := providers.NewProviders()
	discover.Register("test", map[string]providers.Handler{
		"output": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			ip, _ := v.MakeValue(`
myIP: value: "1.1.1.1"            
`)
			return v.FillObject(ip)
		},
		"input": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			val, err := v.LookupValue("set", "prefixIP")
			r.NoError(err)
			str, err := val.CueValue().String()
			r.NoError(err)
			r.Equal(str, "1.1.1.1")
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

	pCtx := process.NewContext(process.ContextData{
		AppName:         "app",
		CompName:        "app",
		Namespace:       "default",
		AppRevisionName: "app-v1",
	})
	tasksLoader := NewTaskLoader(mockLoadTemplate, nil, discover, 0, pCtx)

	steps := []v1beta1.WorkflowStep{
		{
			Name: "output",
			Type: "output",
			Outputs: common.StepOutputs{{
				ValueFrom: "myIP.value",
				Name:      "podIP",
			}},
		},
		{
			Name: "input",
			Type: "input",
			Inputs: common.StepInputs{{
				From:         "podIP",
				ParameterKey: "set.prefixIP",
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
		r.NoError(err)
		run, err := gen(step, &types.GeneratorOptions{})
		r.NoError(err)
		status, action, err := run.Run(wfCtx, &types.TaskRunOptions{})
		r.NoError(err)
		if step.Name == "wait" {
			r.Equal(status.Phase, common.WorkflowStepPhaseRunning)
			r.Equal(status.Reason, types.StatusReasonWait)
			r.Equal(status.Message, "I am waiting")
			continue
		}
		if step.Name == "terminate" {
			r.Equal(action.Terminated, true)
			r.Equal(status.Reason, types.StatusReasonTerminate)
			r.Equal(status.Message, "I am terminated")
			continue
		}
		if step.Name == "rendering" {
			r.Equal(status.Phase, common.WorkflowStepPhaseFailed)
			r.Equal(status.Reason, types.StatusReasonRendering)
			continue
		}
		if step.Name == "execute" {
			r.Equal(status.Phase, common.WorkflowStepPhaseFailed)
			r.Equal(status.Reason, types.StatusReasonExecute)
			continue
		}
		r.Equal(status.Phase, common.WorkflowStepPhaseSucceeded)
	}

}

func TestErrCases(t *testing.T) {
	wfCtx := newWorkflowContextForTest(t)
	r := require.New(t)
	closeVar, err := value.NewValue(`
close({
   x: 100
})
`, nil, "", value.TagFieldOrder)
	r.NoError(err)
	err = wfCtx.SetVar(closeVar, "score")
	r.NoError(err)
	discover := providers.NewProviders()
	discover.Register("test", map[string]providers.Handler{
		"input": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			val, err := v.LookupValue("prefixIP")
			r.NoError(err)
			str, err := val.CueValue().String()
			r.NoError(err)
			r.Equal(str, "1.1.1.1")
			return nil
		},
		"ok": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			return nil
		},
		"error": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			return errors.New("mock error")
		},
	})
	pCtx := process.NewContext(process.ContextData{
		AppName:         "app",
		CompName:        "app",
		Namespace:       "default",
		AppRevisionName: "app-v1",
	})
	tasksLoader := NewTaskLoader(mockLoadTemplate, nil, discover, 0, pCtx)

	steps := []v1beta1.WorkflowStep{
		{
			Name: "input-err",
			Type: "ok",
			Properties: &runtime.RawExtension{Raw: []byte(`
		{"score": {"y": 101}}
		`)},
			Inputs: common.StepInputs{{
				From:         "score",
				ParameterKey: "score",
			}},
		},
		{
			Name: "input",
			Type: "input",
			Inputs: common.StepInputs{{
				From:         "podIP",
				ParameterKey: "prefixIP",
			}},
		},
		{
			Name: "output-var-conflict",
			Type: "ok",
			Outputs: common.StepOutputs{{
				Name:      "score",
				ValueFrom: "name",
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
		{
			Name: "failed-after-retries",
			Type: "error",
		},
	}
	for _, step := range steps {
		gen, err := tasksLoader.GetTaskGenerator(context.Background(), step.Type)
		r.NoError(err)
		run, err := gen(step, &types.GeneratorOptions{})
		r.NoError(err)
		status, operation, err := run.Run(wfCtx, &types.TaskRunOptions{})
		switch step.Name {
		case "input-err":
			r.Equal(operation.Waiting, false)
			r.Equal(status.Phase, common.WorkflowStepPhaseFailed)
		case "input":
			r.Equal(err.Error(), "do preStartHook: get input from [podIP]: var(path=podIP) not exist")
		case "output-var-conflict":
			r.Contains(status.Message, "conflict")
			r.Equal(operation.Waiting, false)
			r.Equal(status.Phase, common.WorkflowStepPhaseSucceeded)
		case "failed-after-retries":
			wfContext.CleanupMemoryStore("app-v1", "default")
			newCtx := newWorkflowContextForTest(t)
			for i := 0; i < types.MaxWorkflowStepErrorRetryTimes; i++ {
				status, operation, err = run.Run(newCtx, &types.TaskRunOptions{})
				r.NoError(err)
				r.Equal(operation.Waiting, true)
				r.Equal(operation.FailedAfterRetries, false)
				r.Equal(status.Phase, common.WorkflowStepPhaseFailed)
			}
			status, operation, err = run.Run(newCtx, &types.TaskRunOptions{})
			r.NoError(err)
			r.Equal(operation.Waiting, false)
			r.Equal(operation.FailedAfterRetries, true)
			r.Equal(status.Phase, common.WorkflowStepPhaseFailed)
			r.Equal(status.Reason, types.StatusReasonFailedAfterRetries)
		default:
			r.Equal(operation.Waiting, true)
			r.Equal(status.Phase, common.WorkflowStepPhaseFailed)
		}
	}
}

func TestSteps(t *testing.T) {

	var (
		echo    string
		mockErr = errors.New("mock error")
	)

	wfCtx := newWorkflowContextForTest(t)
	r := require.New(t)
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
  } @step(1)
  #up: [{},process] @step(2)
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
		{
			base: `
process: {
	#provider: "test"
	#do: "ok"
    err: true
}

if process.err {
  err: {
     #provider: "test"
	 #do: "error"
  }
}

apply: {
	#provider: "test"
	#do: "ok"
}

#up: [process,{}]
`,
			expected: "ok",
			hasErr:   true,
		},
	}

	for _, tc := range testCases {
		echo = ""
		v, err := value.NewValue(tc.base, nil, "", value.TagFieldOrder)
		r.NoError(err)
		err = exec.doSteps(wfCtx, v)
		r.Equal(err != nil, tc.hasErr)
		r.Equal(echo, tc.expected)
	}

}

func TestPendingInputCheck(t *testing.T) {
	wfCtx := newWorkflowContextForTest(t)
	r := require.New(t)
	discover := providers.NewProviders()
	discover.Register("test", map[string]providers.Handler{
		"ok": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			return nil
		},
	})
	step := v1beta1.WorkflowStep{
		Name: "pending",
		Type: "ok",
		Inputs: common.StepInputs{{
			From:         "score",
			ParameterKey: "score",
		}},
	}
	pCtx := process.NewContext(process.ContextData{
		AppName:         "myapp",
		CompName:        "mycomp",
		Namespace:       "default",
		AppRevisionName: "myapp-v1",
	})
	tasksLoader := NewTaskLoader(mockLoadTemplate, nil, discover, 0, pCtx)
	gen, err := tasksLoader.GetTaskGenerator(context.Background(), step.Type)
	r.NoError(err)
	run, err := gen(step, &types.GeneratorOptions{})
	r.NoError(err)
	p, _ := run.Pending(wfCtx, nil)
	r.Equal(p, true)
	score, err := value.NewValue(`
100
`, nil, "")
	r.NoError(err)
	err = wfCtx.SetVar(score, "score")
	r.NoError(err)
	p, _ = run.Pending(wfCtx, nil)
	r.Equal(p, false)
}

func TestPendingDependsOnCheck(t *testing.T) {
	wfCtx := newWorkflowContextForTest(t)
	r := require.New(t)
	discover := providers.NewProviders()
	discover.Register("test", map[string]providers.Handler{
		"ok": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			return nil
		},
	})
	step := v1beta1.WorkflowStep{
		Name:      "pending",
		Type:      "ok",
		DependsOn: []string{"depend"},
	}
	pCtx := process.NewContext(process.ContextData{
		AppName:         "myapp",
		CompName:        "mycomp",
		Namespace:       "default",
		AppRevisionName: "myapp-v1",
	})
	tasksLoader := NewTaskLoader(mockLoadTemplate, nil, discover, 0, pCtx)
	gen, err := tasksLoader.GetTaskGenerator(context.Background(), step.Type)
	r.NoError(err)
	run, err := gen(step, &types.GeneratorOptions{})
	r.NoError(err)
	p, _ := run.Pending(wfCtx, nil)
	r.Equal(p, true)
	ss := map[string]common.StepStatus{
		"depend": {
			Phase: common.WorkflowStepPhaseSucceeded,
		},
	}
	p, _ = run.Pending(wfCtx, ss)
	r.Equal(p, false)
}

func TestSkip(t *testing.T) {
	r := require.New(t)
	discover := providers.NewProviders()
	discover.Register("test", map[string]providers.Handler{
		"ok": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			return nil
		},
	})
	step := v1beta1.WorkflowStep{
		Name: "skip",
		Type: "ok",
	}
	pCtx := process.NewContext(process.ContextData{
		AppName:         "myapp",
		CompName:        "mycomp",
		Namespace:       "default",
		AppRevisionName: "myapp-v1",
	})
	tasksLoader := NewTaskLoader(mockLoadTemplate, nil, discover, 0, pCtx)
	gen, err := tasksLoader.GetTaskGenerator(context.Background(), step.Type)
	r.NoError(err)
	runner, err := gen(step, &types.GeneratorOptions{})
	r.NoError(err)
	wfCtx := newWorkflowContextForTest(t)
	status, operations, err := runner.Run(wfCtx, &types.TaskRunOptions{
		PreCheckHooks: []types.TaskPreCheckHook{
			func(step v1beta1.WorkflowStep, options *types.PreCheckOptions) (*types.PreCheckResult, error) {
				return &types.PreCheckResult{Skip: true}, nil
			},
		},
	})
	r.NoError(err)
	r.Equal(status.Phase, common.WorkflowStepPhaseSkipped)
	r.Equal(status.Reason, types.StatusReasonSkip)
	r.Equal(operations.Skip, true)
}

func TestTimeout(t *testing.T) {
	r := require.New(t)
	discover := providers.NewProviders()
	discover.Register("test", map[string]providers.Handler{
		"ok": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			return nil
		},
	})
	step := v1beta1.WorkflowStep{
		Name: "timeout",
		Type: "ok",
	}
	pCtx := process.NewContext(process.ContextData{
		AppName:         "myapp-timeout",
		CompName:        "mycomp",
		Namespace:       "default",
		AppRevisionName: "myapp-v1",
	})
	tasksLoader := NewTaskLoader(mockLoadTemplate, nil, discover, 0, pCtx)
	gen, err := tasksLoader.GetTaskGenerator(context.Background(), step.Type)
	r.NoError(err)
	runner, err := gen(step, &types.GeneratorOptions{})
	r.NoError(err)
	ctx := newWorkflowContextForTest(t)
	status, _, err := runner.Run(ctx, &types.TaskRunOptions{
		PreCheckHooks: []types.TaskPreCheckHook{
			func(step v1beta1.WorkflowStep, options *types.PreCheckOptions) (*types.PreCheckResult, error) {
				return &types.PreCheckResult{Timeout: true}, nil
			},
		},
	})
	r.NoError(err)
	r.Equal(status.Phase, common.WorkflowStepPhaseFailed)
	r.Equal(status.Reason, types.StatusReasonTimeout)
}

func TestValidateIfValue(t *testing.T) {
	ctx := newWorkflowContextForTest(t)
	pCtx := process.NewContext(process.ContextData{
		AppName:         "app",
		CompName:        "app",
		Namespace:       "default",
		AppRevisionName: "app-v1",
	})

	testCases := []struct {
		name        string
		step        v1beta1.WorkflowStep
		status      map[string]common.StepStatus
		expected    bool
		expectedErr string
	}{
		{
			name: "timeout true",
			step: v1beta1.WorkflowStep{
				If: "status.step1.timeout",
			},
			status: map[string]common.StepStatus{
				"step1": {
					Reason: "Timeout",
				},
			},
			expected: true,
		},
		{
			name: "context true",
			step: v1beta1.WorkflowStep{
				If: `context.name == "app"`,
			},
			expected: true,
		},
		{
			name: "failed true",
			step: v1beta1.WorkflowStep{
				If: `status.step1.phase != "failed"`,
			},
			status: map[string]common.StepStatus{
				"step1": {
					Phase: common.WorkflowStepPhaseSucceeded,
				},
			},
			expected: true,
		},
		{
			name: "input true",
			step: v1beta1.WorkflowStep{
				If: `inputs.test == "yes"`,
				Inputs: common.StepInputs{
					{
						From: "test",
					},
				},
			},
			expected: true,
		},
		{
			name: "input false with dash",
			step: v1beta1.WorkflowStep{
				If: `inputs["test-input"] == "yes"`,
				Inputs: common.StepInputs{
					{
						From: "test-input",
					},
				},
			},
			expectedErr: "not found",
			expected:    false,
		},
		{
			name: "dash in if",
			step: v1beta1.WorkflowStep{
				If: "status.step1-test.timeout",
			},
			expectedErr: "invalid if value",
			expected:    false,
		},
		{
			name: "dash in status",
			step: v1beta1.WorkflowStep{
				If: `status["step1-test"].timeout`,
			},
			status: map[string]common.StepStatus{
				"step1-test": {
					Reason: "Timeout",
				},
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			v, err := ValidateIfValue(ctx, tc.step, tc.status, &types.PreCheckOptions{
				ProcessContext: pCtx,
			})
			if tc.expectedErr != "" {
				r.Contains(err.Error(), tc.expectedErr)
				r.Equal(v, false)
				return
			}
			r.NoError(err)
			r.Equal(v, tc.expected)
		})
	}
}

func newWorkflowContextForTest(t *testing.T) wfContext.Context {
	r := require.New(t)
	cm := corev1.ConfigMap{}
	testCaseJson, err := yaml.YAMLToJSON([]byte(testCaseYaml))
	r.NoError(err)
	err = json.Unmarshal(testCaseJson, &cm)
	r.NoError(err)

	cli := &test.MockClient{
		MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			o, ok := obj.(*corev1.ConfigMap)
			if ok {
				*o = cm
			}
			return nil
		},
		MockUpdate: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			return nil
		},
	}
	wfCtx, err := wfContext.NewContext(cli, "default", "app-v1", "testuid")
	r.NoError(err)
	v, _ := value.NewValue(`name: "app"`, nil, "")
	r.NoError(wfCtx.SetVar(v, types.ContextKeyMetadata))
	v, _ = value.NewValue(`"yes"`, nil, "")
	r.NoError(wfCtx.SetVar(v, "test"))
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
