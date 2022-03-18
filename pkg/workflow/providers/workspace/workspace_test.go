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

package workspace

import (
	"encoding/json"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
)

func TestProvider_Load(t *testing.T) {
	wfCtx := newWorkflowContextForTest(t)
	p := &provider{}
	v, err := value.NewValue(`
component: "server"
`, nil, "")
	assert.NilError(t, err)
	err = p.Load(wfCtx, v, &mockAction{})
	assert.NilError(t, err)
	v, err = v.LookupValue("value")
	assert.NilError(t, err)
	str, err := v.String()
	assert.NilError(t, err)
	assert.Equal(t, str, expectedManifest)

	// check Get Components
	v, err = value.NewValue(`{}`, nil, "")
	assert.NilError(t, err)
	err = p.Load(wfCtx, v, &mockAction{})
	assert.NilError(t, err)
	v, err = v.LookupValue("value.server")
	assert.NilError(t, err)
	str, err = v.String()
	assert.NilError(t, err)
	assert.Equal(t, str, expectedManifest)

	errTestCases := []string{
		`component: "not-found"`,
		`component: 124`,
		`component: _|_`,
	}

	for _, tCase := range errTestCases {
		errv, err := value.NewValue(tCase, nil, "")
		assert.NilError(t, err)
		err = p.Load(wfCtx, errv, &mockAction{})
		assert.Equal(t, err != nil, true)
	}
}

func TestProvider_Export(t *testing.T) {
	wfCtx := newWorkflowContextForTest(t)
	p := &provider{}
	v, err := value.NewValue(`
value: {
	spec: containers: [{
      // +patchKey=name
      env:[{name: "ClusterIP",value: "1.1.1.1"}]
    }]
}
component: "server"
`, nil, "")
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

	errCases := []string{`
value: "1.1.1.1"
`, `
component: "not-found"
value: {}
`, `
component: "server"
`}

	for _, tCase := range errCases {
		v, err = value.NewValue(tCase, nil, "")
		assert.NilError(t, err)
		err = p.Export(wfCtx, v, &mockAction{})
		assert.Equal(t, err != nil, true)
	}
}

func TestProvider_DoVar(t *testing.T) {
	wfCtx := newWorkflowContextForTest(t)
	p := &provider{}

	v, err := value.NewValue(`
method: "Put"
path: "clusterIP"
value: "1.1.1.1"
`, nil, "")
	assert.NilError(t, err)
	err = p.DoVar(wfCtx, v, &mockAction{})
	assert.NilError(t, err)
	varV, err := wfCtx.GetVar("clusterIP")
	assert.NilError(t, err)
	s, err := varV.CueValue().String()
	assert.NilError(t, err)
	assert.Equal(t, s, "1.1.1.1")

	v, err = value.NewValue(`
method: "Get"
path: "clusterIP"
`, nil, "")
	assert.NilError(t, err)
	err = p.DoVar(wfCtx, v, &mockAction{})
	assert.NilError(t, err)
	varV, err = v.LookupValue("value")
	assert.NilError(t, err)
	s, err = varV.CueValue().String()
	assert.NilError(t, err)
	assert.Equal(t, s, "1.1.1.1")

	errCases := []string{`
value: "1.1.1.1"
`, `
method: "Get"
`, `
path: "ClusterIP"
`, `
method: "Put"
path: "ClusterIP"
`}

	for _, tCase := range errCases {
		v, err = value.NewValue(tCase, nil, "")
		assert.NilError(t, err)
		err = p.DoVar(wfCtx, v, &mockAction{})
		assert.Equal(t, err != nil, true)
	}
}

func TestProvider_Wait(t *testing.T) {
	wfCtx := newWorkflowContextForTest(t)
	p := &provider{}
	act := &mockAction{}
	v, err := value.NewValue(`
continue: 100!=100
message: "test log"
`, nil, "")
	assert.NilError(t, err)
	err = p.Wait(wfCtx, v, act)
	assert.NilError(t, err)
	assert.Equal(t, act.wait, true)
	assert.Equal(t, act.msg, "test log")

	act = &mockAction{}
	v, err = value.NewValue(`
continue: 100==100
message: "not invalid"
`, nil, "")
	assert.NilError(t, err)
	err = p.Wait(wfCtx, v, act)
	assert.NilError(t, err)
	assert.Equal(t, act.wait, false)
	assert.Equal(t, act.msg, "")

	act = &mockAction{}
	v, err = value.NewValue(`
continue: bool
message: string
`, nil, "")
	assert.NilError(t, err)
	err = p.Wait(wfCtx, v, act)
	assert.NilError(t, err)
	assert.Equal(t, act.wait, true)

	act = &mockAction{}
	v, err = value.NewValue(``, nil, "")
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

	act = &mockAction{}
	v, err := value.NewValue(`
message: "terminate"
`, nil, "")
	assert.NilError(t, err)
	err = p.Break(wfCtx, v, act)
	assert.NilError(t, err)
	assert.Equal(t, act.terminate, true)
	assert.Equal(t, act.msg, "terminate")
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
	expectedManifest = `workload: {
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
