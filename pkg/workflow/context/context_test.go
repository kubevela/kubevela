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
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	yamlUtil "sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
)

func TestComponent(t *testing.T) {
	wfCtx := newContextForTest(t)
	r := require.New(t)

	_, err := wfCtx.GetComponent("expected-not-found")
	r.Equal(err.Error(), "component expected-not-found not found in application")
	cmf, err := wfCtx.GetComponent("server")
	r.NoError(err)
	components := wfCtx.GetComponents()
	_, ok := components["server"]
	r.Equal(ok, true)

	r.Equal(cmf.Workload.String(), `apiVersion: "v1"
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
	r.Equal(len(cmf.Auxiliaries), 1)
	r.Equal(cmf.Auxiliaries[0].String(), `apiVersion: "v1"
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
	r.NoError(err)
	err = wfCtx.PatchComponent("server", pv)
	r.NoError(err)

	cmf, err = wfCtx.GetComponent("server")
	r.NoError(err)
	r.Equal(cmf.Workload.String(), `apiVersion: "v1"
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
	r.NoError(err)
	expected, err := yaml.Marshal(wfCtx.components)
	r.NoError(err)

	err = wfCtx.LoadFromConfigMap(*wfCtx.store)
	r.NoError(err)
	componentsYaml, err := yaml.Marshal(wfCtx.components)
	r.NoError(err)
	r.Equal(string(expected), string(componentsYaml))
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
		r := require.New(t)
		val, err := value.NewValue(tCase.variable, nil, "")
		r.NoError(err)
		input, err := val.LookupValue("input")
		r.NoError(err)
		err = wfCtx.SetVar(input, tCase.paths...)
		r.NoError(err)
		result, err := wfCtx.GetVar(tCase.paths...)
		r.NoError(err)
		rStr, err := result.String()
		r.NoError(err)
		r.Equal(rStr, tCase.expected)
	}

	r := require.New(t)
	param, err := wfCtx.MakeParameter(map[string]interface{}{
		"name": "foo",
	})
	r.NoError(err)
	mark, err := wfCtx.GetVar("football")
	r.NoError(err)
	err = param.FillObject(mark)
	r.NoError(err)
	rStr, err := param.String()
	r.NoError(err)
	r.Equal(rStr, `name:   "foo"
score:  100
result: 101
`)

	conflictV, err := value.NewValue(`score: 101`, nil, "")
	r.NoError(err)
	err = wfCtx.SetVar(conflictV, "football")
	r.Equal(err.Error(), "football.result: conflicting values 100 and 101")
}

func TestRefObj(t *testing.T) {

	wfCtx := new(WorkflowContext)
	wfCtx.store = &corev1.ConfigMap{}
	wfCtx.store.APIVersion = "v1"
	wfCtx.store.Kind = "ConfigMap"
	wfCtx.store.Name = "app-v1"

	ref := wfCtx.StoreRef()
	r := require.New(t)
	r.Equal(*ref, corev1.ObjectReference{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Name:       "app-v1",
	})
}

func TestContext(t *testing.T) {
	cli := newCliForTest(t, nil)
	r := require.New(t)

	wfCtx, err := NewContext(cli, "default", "app-v1", "testuid")
	r.NoError(err)
	err = wfCtx.Commit()
	r.NoError(err)

	wfCtx, err = LoadContext(cli, "default", "app-v1")
	r.NoError(err)
	err = wfCtx.Commit()
	r.NoError(err)

	cli = newCliForTest(t, nil)
	_, err = LoadContext(cli, "default", "app-v1")
	r.Equal(err.Error(), `configMap "workflow-app-v1-context" not found`)

	wfCtx, err = NewContext(cli, "default", "app-v1", "testuid")
	r.NoError(err)
	r.Equal(len(wfCtx.GetComponents()), 0)
	_, err = wfCtx.GetComponent("server")
	r.Equal(err.Error(), "component server not found in application")
}

func TestGetStore(t *testing.T) {
	cli := newCliForTest(t, nil)
	r := require.New(t)

	wfCtx, err := NewContext(cli, "default", "app-v1", "testuid")
	r.NoError(err)
	err = wfCtx.Commit()
	r.NoError(err)

	store := wfCtx.GetStore()
	r.Equal(store.Name, "workflow-app-v1-context")
}

func TestMutableValue(t *testing.T) {
	cli := newCliForTest(t, nil)
	r := require.New(t)

	wfCtx, err := NewContext(cli, "default", "app-v1", "testuid")
	r.NoError(err)
	err = wfCtx.Commit()
	r.NoError(err)

	wfCtx.SetMutableValue("value", "test", "key")
	v := wfCtx.GetMutableValue("test", "key")
	r.Equal(v, "value")

	wfCtx.DeleteMutableValue("test", "key")
	v = wfCtx.GetMutableValue("test", "key")
	r.Equal(v, "")
}

func TestMemoryValue(t *testing.T) {
	cli := newCliForTest(t, nil)
	r := require.New(t)

	wfCtx, err := NewContext(cli, "default", "app-v1", "testuid")
	r.NoError(err)
	err = wfCtx.Commit()
	r.NoError(err)

	wfCtx.SetValueInMemory("value", "test", "key")
	v, ok := wfCtx.GetValueInMemory("test", "key")
	r.Equal(ok, true)
	r.Equal(v.(string), "value")

	wfCtx.DeleteValueInMemory("test", "key")
	v, ok = wfCtx.GetValueInMemory("test", "key")
	r.Equal(ok, false)

	wfCtx.SetValueInMemory("value", "test", "key")
	count := wfCtx.IncreaseCountValueInMemory("test", "key")
	r.Equal(count, 0)
	count = wfCtx.IncreaseCountValueInMemory("notfound", "key")
	r.Equal(count, 0)
	wfCtx.SetValueInMemory(10, "number", "key")
	count = wfCtx.IncreaseCountValueInMemory("number", "key")
	r.Equal(count, 11)
}

func newCliForTest(t *testing.T, wfCm *corev1.ConfigMap) *test.MockClient {
	r := require.New(t)
	return &test.MockClient{
		MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			o, ok := obj.(*corev1.ConfigMap)
			if ok {
				switch key.Name {
				case "app-v1":
					var cm corev1.ConfigMap
					testCaseJson, err := yamlUtil.YAMLToJSON([]byte(testCaseYaml))
					r.NoError(err)
					err = json.Unmarshal(testCaseJson, &cm)
					r.NoError(err)
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
}

func newContextForTest(t *testing.T) *WorkflowContext {
	r := require.New(t)
	var cm corev1.ConfigMap
	testCaseJson, err := yamlUtil.YAMLToJSON([]byte(testCaseYaml))
	r.NoError(err)
	err = json.Unmarshal(testCaseJson, &cm)
	r.NoError(err)

	wfCtx := &WorkflowContext{
		store: &cm,
	}
	err = wfCtx.LoadFromConfigMap(cm)
	r.NoError(err)
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
