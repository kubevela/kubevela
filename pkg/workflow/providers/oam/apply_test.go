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
	"context"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/mock"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestParser(t *testing.T) {
	r := require.New(t)
	p := &provider{
		apply: simpleComponentApplyForTest,
	}
	act := &mock.Action{}
	v, err := value.NewValue("", nil, "")
	r.NoError(err)
	err = p.ApplyComponent(nil, nil, v, act)
	r.Equal(err.Error(), "failed to lookup value: var(path=value) not exist")
	v.FillObject(map[string]interface{}{}, "value")
	err = p.ApplyComponent(nil, nil, v, act)
	r.NoError(err)
	output, err := v.LookupValue("output")
	r.NoError(err)
	outStr, err := output.String()
	r.NoError(err)
	r.Equal(outStr, `apiVersion: "v1"
kind:       "Pod"
metadata: {
	labels: {
		app: "web"
	}
	name: "rss-site"
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
		labels: {
			"trait.oam.dev/resource": "service"
		}
		name: "service"
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
	err = p.LoadComponent(nil, nil, v, nil)
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
	err = p.LoadComponent(nil, nil, overrideValue, nil)
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
	err = p.LoadComponentInOrder(nil, nil, v, nil)
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

var testHealthy bool

func simpleComponentApplyForTest(_ context.Context, comp common.ApplicationComponent, _ *value.Value, _, _, _ string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
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
