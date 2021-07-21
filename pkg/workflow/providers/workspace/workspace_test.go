package workspace

import (
	"encoding/json"
	"testing"

	"github.com/ghodss/yaml"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
)

func TestProvider_Load(t *testing.T) {
	wfCtx := newWorkflowContextForTest(t)
	p := &provider{}
	v, err := value.NewValue(`
component: "server"
`, nil)
	assert.NilError(t, err)
	err = p.Load(wfCtx, v, &mockAction{})
	assert.NilError(t, err)
	str, err := v.String()
	assert.NilError(t, err)
	assert.Equal(t, str, expectedManifest)
}

func TestProvider_Export(t *testing.T) {
	wfCtx := newWorkflowContextForTest(t)
	p := &provider{}
	v, err := value.NewValue(`
type: "patch"
value: {
	spec: containers: [{
      // +patchKey=name
      env:[{name: "ClusterIP",value: "1.1.1.1"}]
    }]
}
component: "server"
`, nil)
	assert.NilError(t, err)
	err = p.Export(wfCtx, v, &mockAction{})
	assert.NilError(t, err)
	component, err := wfCtx.GetComponent("server")
	assert.NilError(t, err)
	s := component.Workload.String()
	assert.Equal(t, s, `apiVersion: "v1"
kind:       "Pod"
metadata: {
	labels: {
		app: "nginx"
	}
}
spec: {
	containers: [{
		name: "main"
		// +patchKey=name
		env: [{
			name:  "APP"
			value: "nginx"
		}, {
			name:  "ClusterIP"
			value: "1.1.1.1"
		}, ...]
		image:           "nginx:1.14.2"
		imagePullPolicy: "IfNotPresent"
		ports: [{
			containerPort: 8080
			protocol:      "TCP"
		}, ...]
	}, ...]
}
`)

	v, err = value.NewValue(`
type: "var"
path: "clusterIP"
value: "1.1.1.1"
`, nil)
	assert.NilError(t, err)
	err = p.Export(wfCtx, v, &mockAction{})
	assert.NilError(t, err)
	varV, err := wfCtx.GetVar("clusterIP")
	assert.NilError(t, err)
	s, err = varV.CueValue().String()
	assert.NilError(t, err)
	assert.Equal(t, s, "1.1.1.1")
}

func TestProvider_Wait(t *testing.T) {
	wfCtx := newWorkflowContextForTest(t)
	p := &provider{}
	act := &mockAction{}
	v, err := value.NewValue(`
continue: 100!=100
`, nil)
	assert.NilError(t, err)
	err = p.Wait(wfCtx, v, act)
	assert.NilError(t, err)
	assert.Equal(t, act.wait, true)

	act = &mockAction{}
	v, err = value.NewValue(`
continue: 100==100
`, nil)
	assert.NilError(t, err)
	err = p.Wait(wfCtx, v, act)
	assert.NilError(t, err)
	assert.Equal(t, act.wait, false)

	act = &mockAction{}
	v, err = value.NewValue(`
continue: bool
`, nil)
	assert.NilError(t, err)
	err = p.Wait(wfCtx, v, act)
	assert.NilError(t, err)
	assert.Equal(t, act.wait, true)

	act = &mockAction{}
	v, err = value.NewValue(``, nil)
	assert.NilError(t, err)
	err = p.Wait(wfCtx, v, act)
	assert.NilError(t, err)
	assert.Equal(t, act.wait, true)
}

func TestProvider_Break(t *testing.T) {
	wfCtx := newWorkflowContextForTest(t)
	p := &provider{}
	act := &mockAction{}
	err := p.Break(wfCtx, nil, act)
	assert.NilError(t, err)
	assert.Equal(t, act.terminate, true)
}

type mockAction struct {
	suspend   bool
	terminate bool
	wait      bool
	msg       string
}

func (act *mockAction) Suspend(msg string) {
	act.suspend = true
	act.msg = msg
}

func (act *mockAction) Terminate(msg string) {
	act.terminate = true
	act.msg = msg
}
func (act *mockAction) Wait(msg string) {
	act.wait = true
	act.msg = msg
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
	return wfCtx
}

var (
	testCaseYaml = `apiVersion: v1
data:
  components: '{"server":"{\"Scopes\":null,\"StandardWorkload\":\"{\\\"apiVersion\\\":\\\"v1\\\",\\\"kind\\\":\\\"Pod\\\",\\\"metadata\\\":{\\\"labels\\\":{\\\"app\\\":\\\"nginx\\\"}},\\\"spec\\\":{\\\"containers\\\":[{\\\"env\\\":[{\\\"name\\\":\\\"APP\\\",\\\"value\\\":\\\"nginx\\\"}],\\\"image\\\":\\\"nginx:1.14.2\\\",\\\"imagePullPolicy\\\":\\\"IfNotPresent\\\",\\\"name\\\":\\\"main\\\",\\\"ports\\\":[{\\\"containerPort\\\":8080,\\\"protocol\\\":\\\"TCP\\\"}]}]}}\",\"Traits\":[\"{\\\"apiVersion\\\":\\\"v1\\\",\\\"kind\\\":\\\"Service\\\",\\\"metadata\\\":{\\\"name\\\":\\\"my-service\\\"},\\\"spec\\\":{\\\"ports\\\":[{\\\"port\\\":80,\\\"protocol\\\":\\\"TCP\\\",\\\"targetPort\\\":8080}],\\\"selector\\\":{\\\"app\\\":\\\"nginx\\\"}}}\"]}"}'
kind: ConfigMap
metadata:
  name: app-v1
`
	expectedManifest = `component: "server"
workload: {
	apiVersion: "v1"
	kind:       "Pod"
	metadata: {
		labels: {
			app: "nginx"
		}
	}
	spec: {
		containers: [{
			name: "main"
			env: [{
				name:  "APP"
				value: "nginx"
			}, ...]
			image:           "nginx:1.14.2"
			imagePullPolicy: "IfNotPresent"
			ports: [{
				containerPort: 8080
				protocol:      "TCP"
			}, ...]
		}, ...]
	}
}
auxiliaries: [{
	apiVersion: "v1"
	kind:       "Service"
	metadata: {
		name: "my-service"
	}
	spec: {
		ports: [{
			protocol:   "TCP"
			port:       80
			targetPort: 8080
		}, ...]
		selector: {
			app: "nginx"
		}
	}
}]
`
)
