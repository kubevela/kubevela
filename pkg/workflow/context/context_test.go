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

package context

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"gopkg.in/yaml.v3"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	yamlUtil "sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
)

func TestComponent(t *testing.T) {
	wfCtx := newContextForTest(t)

	_, err := wfCtx.GetComponent("expected-not-found")
	assert.Equal(t, err != nil, true)
	cmf, err := wfCtx.GetComponent("server")
	assert.NilError(t, err)
	components := wfCtx.GetComponents()
	_, ok := components["server"]
	assert.Equal(t, ok, true)

	assert.Equal(t, cmf.Workload.String(), `apiVersion: "v1"
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
`)
	assert.Equal(t, len(cmf.Auxiliaries), 1)
	assert.Equal(t, cmf.Auxiliaries[0].String(), `apiVersion: "v1"
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
`)

	pv, err := value.NewValue(`
spec: containers: [{
// +patchKey=name
env:[{name: "ClusterIP",value: "1.1.1.1"}]}]
`, nil, "")
	assert.NilError(t, err)
	err = wfCtx.PatchComponent("server", pv)
	assert.NilError(t, err)

	cmf, err = wfCtx.GetComponent("server")
	assert.NilError(t, err)
	assert.Equal(t, cmf.Workload.String(), `apiVersion: "v1"
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

	err = wfCtx.writeToStore()
	assert.NilError(t, err)
	expected, err := yaml.Marshal(wfCtx.components)
	assert.NilError(t, err)

	err = wfCtx.LoadFromConfigMap(wfCtx.store)
	assert.NilError(t, err)
	componentsYaml, err := yaml.Marshal(wfCtx.components)
	assert.NilError(t, err)
	assert.Equal(t, string(expected), string(componentsYaml))
}

func TestVars(t *testing.T) {
	wfCtx := newContextForTest(t)

	testCases := []struct {
		variable string
		paths    []string
		expected string
	}{
		{
			variable: `input: "1.1.1.1"`,
			paths:    []string{"clusterIP"},
			expected: `"1.1.1.1"
`,
		},
		{
			variable: "input: 100",
			paths:    []string{"football", "score"},
			expected: "100\n",
		},
		{
			variable: `
input: {
    score: int
	result: score+1
}`,
			paths: []string{"football"},
			expected: `score:  100
result: 101
`,
		},
	}
	for _, tCase := range testCases {
		val, err := value.NewValue(tCase.variable, nil, "")
		assert.NilError(t, err)
		input, err := val.LookupValue("input")
		assert.NilError(t, err)
		err = wfCtx.SetVar(input, tCase.paths...)
		assert.NilError(t, err)
		result, err := wfCtx.GetVar(tCase.paths...)
		assert.NilError(t, err)
		rStr, err := result.String()
		assert.NilError(t, err)
		assert.Equal(t, rStr, tCase.expected)
	}

	param, err := wfCtx.MakeParameter(map[string]interface{}{
		"name": "foo",
	})
	assert.NilError(t, err)
	mark, err := wfCtx.GetVar("football")
	assert.NilError(t, err)
	err = param.FillObject(mark)
	assert.NilError(t, err)
	rStr, err := param.String()
	assert.NilError(t, err)
	assert.Equal(t, rStr, `name:   "foo"
score:  100
result: 101
`)

	conflictV, _ := value.NewValue(`score: 101`, nil, "")
	err = wfCtx.SetVar(conflictV, "football")
	assert.Equal(t, err != nil, true)
}

func TestRefObj(t *testing.T) {

	wfCtx := new(WorkflowContext)
	wfCtx.store = corev1.ConfigMap{}
	wfCtx.store.APIVersion = "v1"
	wfCtx.store.Kind = "ConfigMap"
	wfCtx.store.Name = "app-v1"

	ref := wfCtx.StoreRef()
	assert.Equal(t, *ref, corev1.ObjectReference{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Name:       "app-v1",
	})
}

func TestContext(t *testing.T) {
	var wfCm *corev1.ConfigMap
	cli := &test.MockClient{
		MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			o, ok := obj.(*corev1.ConfigMap)
			if ok {
				switch key.Name {
				case "app-v1":
					var cm corev1.ConfigMap
					testCaseJson, err := yamlUtil.YAMLToJSON([]byte(testCaseYaml))
					assert.NilError(t, err)
					err = json.Unmarshal(testCaseJson, &cm)
					assert.NilError(t, err)
					*o = cm
					return nil
				case generateStoreName("app-v1"):
					if wfCm != nil {
						*o = *wfCm
						return nil
					}
				}
			}
			return kerrors.NewNotFound(corev1.Resource("configMap"), key.Name)
		},
		MockCreate: func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
			o, ok := obj.(*corev1.ConfigMap)
			if ok {
				wfCm = o
			}
			return nil
		},
		MockUpdate: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			o, ok := obj.(*corev1.ConfigMap)
			if ok {
				if wfCm == nil {
					return kerrors.NewNotFound(corev1.Resource("configMap"), o.Name)
				}
				*wfCm = *o
			}
			return nil
		},
	}

	wfCtx, err := NewContext(cli, "default", "app-v1", "testuid")
	assert.NilError(t, err)
	err = wfCtx.Commit()
	assert.NilError(t, err)

	wfCtx, err = LoadContext(cli, "default", "app-v1")
	assert.NilError(t, err)
	err = wfCtx.Commit()
	assert.NilError(t, err)

	wfCm = nil
	_, err = LoadContext(cli, "default", "app-v1")
	assert.Equal(t, err != nil, true)

	wfCtx, err = NewContext(cli, "default", "app-v1", "testuid")
	assert.NilError(t, err)
	assert.Equal(t, len(wfCtx.GetComponents()), 0)
	_, err = wfCtx.GetComponent("server")
	assert.Equal(t, err != nil, true)
}

func newContextForTest(t *testing.T) *WorkflowContext {
	var cm corev1.ConfigMap
	testCaseJson, err := yamlUtil.YAMLToJSON([]byte(testCaseYaml))
	assert.NilError(t, err)
	err = json.Unmarshal(testCaseJson, &cm)
	assert.NilError(t, err)

	wfCtx := new(WorkflowContext)
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
)
