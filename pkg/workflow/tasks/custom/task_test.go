package custom

import (
	"context"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

func TestTaskLoader(t *testing.T) {
	cm := corev1.ConfigMap{}
	err := yaml.Unmarshal([]byte(testCaseYaml), &cm)
	assert.NilError(t, err)

	wfCtx := new(wfContext.WorkflowContext)
	err = wfCtx.LoadFromConfigMap(cm)
	assert.NilError(t, err)

	discover := providers.NewProviders()
	discover.Register("test", map[string]providers.Handler{
		"output": func(ctx wfContext.Context, v *value.Value, act types.Action) error {
			ipPathVal, err := v.LookupValue("myIP")
			assert.NilError(t, err)
			ipPath, err := ipPathVal.CueValue().String()
			assert.NilError(t, err)
			val, err := value.NewValue(`
ip: "1.1.1.1"            
`, nil)
			assert.NilError(t, err)
			ip, err := val.LookupValue("ip")
			assert.NilError(t, err)
			return ctx.SetVar(ip, ipPath)
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
	}

	for _, step := range steps {
		gen, err := tasksLoader.GetTaskGenerator(step.Type)
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
	}

}

func mockLoadTemplate(_ context.Context, name string) (string, error) {
	templ := `
parameter: {}
process: {
	#provider: "test"
	#do: "%s"
	parameter
}
`
	switch name {
	case "output":
		return fmt.Sprintf(templ, "output"), nil
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
