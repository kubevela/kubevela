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
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/mock"
)

func TestParser(t *testing.T) {
	r := require.New(t)
	p := &provider{
		apply: simpleComponentApplyForTest,
	}
	act := &mock.Action{}
	v, err := value.NewValue("", nil, "")
	r.NoError(err)
	err = p.ApplyComponent(nil, v, act)
	r.Equal(err.Error(), "var(path=value) not exist")
	v.FillObject(map[string]interface{}{}, "value")
	err = p.ApplyComponent(nil, v, act)
	r.NoError(err)
	output, err := v.LookupValue("output")
	r.NoError(err)
	outStr, err := output.String()
	r.NoError(err)
	r.Equal(outStr, `apiVersion: "v1"
kind:       "Pod"
metadata: {
	name: "rss-site"
	labels: {
		app: "web"
	}
}
`)

	outputs, err := v.LookupValue("outputs")
	r.NoError(err)
	outsStr, err := outputs.String()
	r.NoError(err)
	r.Equal(outsStr, `service: {
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

	r.Equal(act.Phase, "Wait")
	testHealthy = true
	act = &mock.Action{}
	_, err = value.NewValue("", nil, "")
	r.NoError(err)
	r.Equal(act.Phase, "")
}

func TestRenderComponent(t *testing.T) {
	r := require.New(t)
	p := &provider{
		render: func(comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, _ string) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
					},
				}, []*unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "core.oam.dev/v1alpha2",
							"kind":       "ManualScalerTrait",
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"trait.oam.dev/resource": "scaler",
								},
							},
							"spec": map[string]interface{}{"replicaCount": int64(10)},
						},
					},
				}, nil
		},
	}
	v, err := value.NewValue(`value: {}`, nil, "")
	r.NoError(err)
	err = p.RenderComponent(nil, v, nil)
	r.NoError(err)
	s, err := v.String()
	r.NoError(err)
	r.Equal(s, `value: {}
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
}
outputs: {
	scaler: {
		apiVersion: "core.oam.dev/v1alpha2"
		kind:       "ManualScalerTrait"
		metadata: {
			labels: {
				"trait.oam.dev/resource": "scaler"
			}
		}
		spec: {
			replicaCount: 10
		}
	}
}
`)
}

func TestApplyComponents(t *testing.T) {
	r := require.New(t)
	testcases := map[string]struct {
		Input string
		Error string
	}{
		"normal": {
			Input: `{components:{first:{value:{name:"first"}},second:{value:{name:"second"}}},parallelism:5}`,
		},
		"no-components": {
			Input: `{}`,
			Error: "var(path=components) not exist",
		},
		"no-parallelism": {
			Input: `{components:{first:{value:{name:"first"}},second:{value:{name:"second"}}}}`,
			Error: "var(path=parallelism) not exist",
		},
		"invalid-parallelism": {
			Input: `{components:{first:{value:{name:"first"}},second:{value:{name:"second"}}},parallelism:-1}`,
			Error: "parallelism cannot be smaller than 1",
		},
		"bad-component": {
			Input: `{components:{first:{value:{name:"error-first"}},second:{value:{name:"error-second"}},third:{value:{name:"third"}}},parallelism:5}`,
			Error: "failed to apply component",
		},
	}
	p := &provider{apply: simpleComponentApplyForTest}
	for name, tt := range testcases {
		t.Run(name, func(t *testing.T) {
			act := &mock.Action{}
			v, err := value.NewValue("", nil, "")
			r.NoError(err)
			r.NoError(v.FillRaw(tt.Input))
			err = p.ApplyComponents(nil, v, act)
			if tt.Error != "" {
				r.NotNil(err)
				r.Contains(err.Error(), tt.Error)
			} else {
				r.NoError(err)
			}
		})
	}
}

func TestLoadComponent(t *testing.T) {
	r := require.New(t)
	p := &provider{
		app: &v1beta1.Application{
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "c1",
						Type:       "web",
						Properties: &runtime.RawExtension{Raw: []byte(`{"image": "busybox"}`)},
					},
				},
			},
		},
	}
	v, err := value.NewValue(``, nil, "")
	r.NoError(err)
	err = p.LoadComponent(nil, v, nil)
	r.NoError(err)
	s, err := v.String()
	r.NoError(err)
	r.Equal(s, `value: {
	c1: {
		name: *"c1" | _
		type: *"web" | _
		properties: {
			image: *"busybox" | _
		}
	}
}
`)
	overrideApp := `app: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	metadata: {
		name:      "test"
		namespace: "default"
	}
	spec: {
		components: [{
			name: "c2"
			type: "web"
			properties: {
				image: "busybox"
			}
		}]
	}
}
`
	overrideValue, err := value.NewValue(overrideApp, nil, "")
	r.NoError(err)
	err = p.LoadComponent(nil, overrideValue, nil)
	r.NoError(err)
	_, err = overrideValue.LookupValue("value", "c2")
	r.NoError(err)
}

func TestLoadComponentInOrder(t *testing.T) {
	r := require.New(t)
	p := &provider{
		app: &v1beta1.Application{
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "c1",
						Type:       "web",
						Properties: &runtime.RawExtension{Raw: []byte(`{"image": "busybox"}`)},
					},
					{
						Name:       "c2",
						Type:       "web2",
						Properties: &runtime.RawExtension{Raw: []byte(`{"image": "busybox"}`)},
					},
				},
			},
		},
	}
	v, err := value.NewValue(``, nil, "")
	r.NoError(err)
	err = p.LoadComponentInOrder(nil, v, nil)
	r.NoError(err)
	s, err := v.String()
	r.NoError(err)
	r.Equal(s, `value: [{
	name: "c1"
	type: "web"
	properties: {
		image: "busybox"
	}
}, {
	name: "c2"
	type: "web2"
	properties: {
		image: "busybox"
	}
}]
`)
}

func TestLoadPolicyInOrder(t *testing.T) {
	r := require.New(t)
	p := &provider{af: &appfile.Appfile{
		Policies: []v1beta1.AppPolicy{{Name: "policy-1"}, {Name: "policy-2"}, {Name: "policy-3"}},
	}, app: &v1beta1.Application{
		Spec: v1beta1.ApplicationSpec{Policies: []v1beta1.AppPolicy{{Name: "policy-1"}, {Name: "policy-2"}}},
	}}
	testcases := map[string]struct {
		Input  string
		Output []v1beta1.AppPolicy
		Error  string
	}{
		"normal": {
			Input:  `{input:["policy-3","policy-1"]}`,
			Output: []v1beta1.AppPolicy{{Name: "policy-3"}, {Name: "policy-1"}},
		},
		"empty-input": {
			Input:  `{}`,
			Output: []v1beta1.AppPolicy{{Name: "policy-1"}, {Name: "policy-2"}},
		},
		"invalid-input": {
			Input: `{input:{"name":"policy"}}`,
			Error: "failed to parse specified policy name",
		},
		"policy-not-found": {
			Input: `{input:["policy-4","policy-1"]}`,
			Error: "not found",
		},
	}
	for name, tt := range testcases {
		t.Run(name, func(t *testing.T) {
			act := &mock.Action{}
			v, err := value.NewValue("", nil, "")
			r.NoError(err)
			r.NoError(v.FillRaw(tt.Input))
			err = p.LoadPoliciesInOrder(nil, v, act)
			if tt.Error != "" {
				r.NotNil(err)
				r.Contains(err.Error(), tt.Error)
			} else {
				r.NoError(err)
				v, err = v.LookupValue("output")
				r.NoError(err)
				var outputPolicies []v1beta1.AppPolicy
				r.NoError(v.UnmarshalTo(&outputPolicies))
				r.Equal(tt.Output, outputPolicies)
			}
		})
	}
}

var testHealthy bool

func simpleComponentApplyForTest(comp common.ApplicationComponent, _ *value.Value, _ string, _ string, _ string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
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
	if comp.Name != "" {
		workload.SetName(comp.Name)
		if strings.Contains(comp.Name, "error") {
			return nil, nil, false, errors.Errorf("bad component")
		}
	}
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
	if comp.Name != "" {
		trait.SetName(comp.Name)
	}
	traits := []*unstructured.Unstructured{trait}
	return workload, traits, testHealthy, nil
}
