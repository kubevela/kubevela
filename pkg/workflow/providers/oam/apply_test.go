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

package oam

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
)

func TestParser(t *testing.T) {

	p := &provider{
		apply: simpleComponentApplyForTest,
	}
	act := &mockAction{}
	v, err := value.NewValue("", nil, "")
	assert.NilError(t, err)
	err = p.ApplyComponent(nil, v, act)
	assert.Error(t, err, "var(path=value) not exist")
	v.FillObject(map[string]interface{}{}, "value")
	err = p.ApplyComponent(nil, v, act)
	assert.NilError(t, err)
	output, err := v.LookupValue("output")
	assert.NilError(t, err)
	outStr, err := output.String()
	assert.NilError(t, err)
	assert.Equal(t, outStr, `apiVersion: "v1"
kind:       "Pod"
metadata: {
	name: "rss-site"
	labels: {
		app: "web"
	}
}
`)

	outputs, err := v.LookupValue("outputs")
	assert.NilError(t, err)
	outsStr, err := outputs.String()
	assert.NilError(t, err)
	assert.Equal(t, outsStr, `service: {
	apiVersion: "v1"
	kind:       "Service"
	metadata: {
		name: "service"
		labels: {
			"trait.oam.dev/resource": "service"
		}
	}
}
`)

	assert.Equal(t, act.phase, "Wait")
	testHealthy = true
	act = &mockAction{}
	_, err = value.NewValue("", nil, "")
	assert.NilError(t, err)
	assert.Equal(t, act.phase, "")
}

func TestLoadComponent(t *testing.T) {
	p := &provider{
		app: &v1beta1.Application{
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "c1",
						Type:       "web",
						Properties: runtime.RawExtension{Raw: []byte(`{"image": "busybox"}`)},
					},
				},
			},
		},
	}
	v, err := value.NewValue(``, nil, "")
	assert.NilError(t, err)
	err = p.LoadComponent(nil, v, nil)
	assert.NilError(t, err)
	s, err := v.String()
	assert.NilError(t, err)
	assert.Equal(t, s, `value: {
	c1: {
		name: *"c1" | _
		type: *"web" | _
		properties: {
			image: *"busybox" | _
		}
	}
}
`)
}

var testHealthy bool

func simpleComponentApplyForTest(comp common.ApplicationComponent, _ *value.Value, _ string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
	workload := new(unstructured.Unstructured)
	workload.UnmarshalJSON([]byte(`{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
               "name": "rss-site",
               "labels": {
                          "app": "web"
                         }
              }
}`))

	trait := new(unstructured.Unstructured)
	trait.UnmarshalJSON([]byte(`{
  "apiVersion": "v1",
  "kind": "Service",
  "metadata": {
               "name": "service",
               "labels": {
                          "trait.oam.dev/resource": "service"
                         }
              }
}`))
	traits := []*unstructured.Unstructured{trait}
	return workload, traits, testHealthy, nil
}

type mockAction struct {
	phase   string
	message string
}

func (act *mockAction) Suspend(message string) {
	act.phase = "Suspend"
	act.message = message
}

func (act *mockAction) Terminate(message string) {
	act.phase = "Terminate"
	act.message = message
}
func (act *mockAction) Wait(message string) {
	act.phase = "Wait"
	act.message = message
}
