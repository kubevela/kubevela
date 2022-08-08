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

package model

import (
	"fmt"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestInstance(t *testing.T) {

	testCases := []struct {
		src string
		gvk schema.GroupVersionKind
	}{{
		src: `apiVersion: "apps/v1"
kind: "Deployment"
metadata: name: "test"
`,
		gvk: schema.GroupVersionKind{
			Group:   "apps",
			Version: "v1",
			Kind:    "Deployment",
		}},
	}

	for _, v := range testCases {
		inst := cuecontext.New().CompileString(v.src)
		base, err := NewBase(inst.Value())
		if err != nil {
			t.Error(err)
			return
		}
		baseObj, err := base.Unstructured()
		if err != nil {
			t.Error(err)
			return
		}

		assert.Equal(t, v.gvk, baseObj.GetObjectKind().GroupVersionKind())
		assert.Equal(t, true, base.IsBase())

		other, err := NewOther(inst.Value())
		if err != nil {
			t.Error(err)
			return
		}
		otherObj, err := other.Unstructured()
		if err != nil {
			t.Error(err)
			return
		}

		assert.Equal(t, v.gvk, otherObj.GetObjectKind().GroupVersionKind())
		assert.Equal(t, false, other.IsBase())
	}
}

func TestIncompleteError(t *testing.T) {
	base := `parameter: {
	name: string
	// +usage=Which image would you like to use for your service
	// +short=i
	image: string
	// +usage=Which port do you want customer traffic sent to
	// +short=p
	port: *8080 | int
	env: [...{
		name:  string
		value: string
	}]
	cpu?: string
}
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: parameter.name
	spec: {
		selector:
			matchLabels:
				app: parameter.name
		template: {
			metadata:
				labels:
					app: parameter.name
			spec: containers: [{
				image: parameter.image
				name:  parameter.name
				env:   parameter.env
				ports: [{
					containerPort: parameter.port
					protocol:      "TCP"
					name:          "default"
				}]
				if parameter["cpu"] != _|_ {
					resources: {
						limits:
							cpu: parameter.cpu
						requests:
							cpu: parameter.cpu
					}
				}
			}]
	}
	}
}
`

	inst := cuecontext.New().CompileString(base)
	newbase, err := NewBase(inst.Value())
	assert.NoError(t, err)
	data, err := newbase.Unstructured()
	assert.Error(t, err)
	var expnil *unstructured.Unstructured
	assert.Equal(t, expnil, data)
}

func TestError(t *testing.T) {
	ctx := cuecontext.New()
	ins := &instance{
		v: ctx.CompileString(``),
	}
	_, err := ins.Unstructured()
	assert.Equal(t, err.Error(), "Object 'Kind' is missing in '{}'")
	ins = &instance{
		v: ctx.CompileString(`
apiVersion: "apps/v1"
kind:       "Deployment"
metadata: name: parameter.name
`),
	}
	_, err = ins.Unstructured()
	assert.Equal(t, err.Error(), fmt.Sprintf("failed to have the workload/trait unstructured: metadata.name: reference \"%s\" not found", ParameterFieldName))
	ins = &instance{
		v: ctx.CompileString(`
apiVersion: "apps/v1"
kind:       "Deployment"
metadata: name: "abc"
`),
	}
	obj, err := ins.Unstructured()
	assert.Equal(t, err, nil)
	assert.Equal(t, obj, &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": "abc",
			},
		},
	})

	ins = &instance{
		v: ctx.CompileString(`
apiVersion: "source.toolkit.fluxcd.io/v1beta1"
metadata: {
	name: "grafana"
}
kind: "HelmRepository"
spec: {
	url:      string
	interval: *"5m" | string
}`),
	}
	o, err := ins.Unstructured()
	assert.Nil(t, o)
	assert.NotNil(t, err)
}
