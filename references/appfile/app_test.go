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

package appfile

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/references/appfile/api"
	"github.com/oam-dev/kubevela/references/appfile/template"
)

const (
	yamlNormal = `name: myapp
services:
  frontend:
    image: inanimate/echo-server
    env:
      PORT: 8080
    autoscaling:
      max: 10
      min: 1
    rollout:
      strategy: canary
      step: 5
  backend:
    type: cloneset
    image: "back:v1"
`
	yamlNoService = `name: myapp`
	yamlNoName    = `services:
  frontend:
    image: inanimate/echo-server
    env:
      PORT: 8080`
	yamlTraitNotMap = `name: myapp
services:
  frontend:
    image: inanimate/echo-server
    env:
      PORT: 8080
    autoscaling: 10`
)

func TestNewApplication(t *testing.T) {
	tm := template.NewFakeTemplateManager()
	app := NewApplication(nil, tm)
	assert.NotNil(t, app)
	assert.Equal(t, tm, app.Tm)
	assert.NotNil(t, app.AppFile)

	appfile := api.NewAppFile()
	appfile.Name = "test-app"
	app = NewApplication(appfile, tm)
	assert.NotNil(t, app)
	assert.Equal(t, "test-app", app.Name)
}

func TestValidate(t *testing.T) {
	testCases := map[string]struct {
		raw     string
		expErr  error
		addFake bool
	}{
		"normal": {
			raw:    yamlNormal,
			expErr: nil,
		},
		"no service": {
			raw:    yamlNoService,
			expErr: errors.New("at least one service is required"),
		},
		"no name": {
			raw:    yamlNoName,
			expErr: errors.New("name is required"),
		},
		"trait not map": {
			raw:     yamlTraitNotMap,
			expErr:  fmt.Errorf("trait autoscaling in 'frontend' must be map"),
			addFake: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			tm := template.NewFakeTemplateManager()
			if tc.addFake {
				tm.Templates["autoscaling"] = &template.Template{
					Captype: types.TypeTrait,
				}
			}
			app := NewApplication(nil, tm)
			err := yaml.Unmarshal([]byte(tc.raw), &app)
			assert.NoError(t, err)
			err = Validate(app)
			assert.Equal(t, tc.expErr, err)
		})
	}
}

func TestGetComponents(t *testing.T) {
	app := &v1beta1.Application{
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{Name: "c"},
				{Name: "a"},
				{Name: "b"},
			},
		},
	}
	comps := GetComponents(app)
	assert.Equal(t, []string{"a", "b", "c"}, comps)
}

func TestGetServiceConfig(t *testing.T) {
	tm := template.NewFakeTemplateManager()
	app := NewApplication(nil, tm)
	err := yaml.Unmarshal([]byte(yamlNormal), &app)
	assert.NoError(t, err)

	tp, cfg := GetServiceConfig(app, "frontend")
	assert.Equal(t, "webservice", tp)
	assert.NotEmpty(t, cfg)
	assert.Contains(t, cfg, "image")

	tp, cfg = GetServiceConfig(app, "backend")
	assert.Equal(t, "cloneset", tp)
	assert.NotEmpty(t, cfg)
	assert.Contains(t, cfg, "image")

	tp, cfg = GetServiceConfig(app, "non-existent")
	assert.Equal(t, "", tp)
	assert.Empty(t, cfg)
}

func TestGetApplicationSettings(t *testing.T) {
	app := &v1beta1.Application{
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name: "comp-1",
					Type: "worker",
					Properties: &runtime.RawExtension{
						Raw: []byte(`{"image":"my-image"}`),
					},
				},
			},
		},
	}

	tp, settings := GetApplicationSettings(app, "comp-1")
	assert.Equal(t, "worker", tp)
	assert.Equal(t, map[string]interface{}{"image": "my-image"}, settings)

	tp, settings = GetApplicationSettings(app, "non-existent")
	assert.Equal(t, "", tp)
	assert.Empty(t, settings)
}

func TestGetWorkload(t *testing.T) {
	tm := template.NewFakeTemplateManager()
	tm.Templates["autoscaling"] = &template.Template{Captype: types.TypeTrait}
	tm.Templates["rollout"] = &template.Template{Captype: types.TypeTrait}

	app := NewApplication(nil, tm)
	err := yaml.Unmarshal([]byte(yamlNormal), &app)
	assert.NoError(t, err)

	testCases := map[string]struct {
		componentName   string
		expWorkloadType string
		expWorkload     map[string]interface{}
	}{
		"frontend": {
			componentName:   "frontend",
			expWorkloadType: "webservice",
			expWorkload: map[string]interface{}{
				"image": "inanimate/echo-server",
				"env": map[string]interface{}{
					"PORT": float64(8080),
				},
			},
		},
		"backend": {
			componentName:   "backend",
			expWorkloadType: "cloneset",
			expWorkload: map[string]interface{}{
				"image": "back:v1",
			},
		},
		"non-existent": {
			componentName:   "non-existent",
			expWorkloadType: "",
			expWorkload:     map[string]interface{}{},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			workloadType, workload := GetWorkload(app, tc.componentName)
			assert.Equal(t, tc.expWorkloadType, workloadType)
			assert.Equal(t, tc.expWorkload, workload)
		})
	}
}

func TestGetTraits(t *testing.T) {
	tm := template.NewFakeTemplateManager()
	tm.Templates["autoscaling"] = &template.Template{Captype: types.TypeTrait}
	tm.Templates["rollout"] = &template.Template{Captype: types.TypeTrait}

	app := NewApplication(nil, tm)
	err := yaml.Unmarshal([]byte(yamlNormal), &app)
	assert.NoError(t, err)

	// Test case with invalid trait format
	invalidTraitApp := NewApplication(nil, tm)
	err = yaml.Unmarshal([]byte(yamlTraitNotMap), &invalidTraitApp)
	assert.NoError(t, err)

	testCases := map[string]struct {
		app      *api.Application
		compName string
		exp      map[string]map[string]interface{}
		expErr   string
	}{
		"frontend traits": {
			app:      app,
			compName: "frontend",
			exp: map[string]map[string]interface{}{
				"autoscaling": {
					"max": float64(10),
					"min": float64(1),
				},
				"rollout": {
					"strategy": "canary",
					"step":     float64(5),
				},
			},
		},
		"backend traits (none)": {
			app:      app,
			compName: "backend",
			exp:      map[string]map[string]interface{}{},
		},
		"non-existent component": {
			app:      app,
			compName: "non-existent",
			exp:      map[string]map[string]interface{}{},
		},
		"invalid trait format": {
			app:      invalidTraitApp,
			compName: "frontend",
			exp:      nil,
			expErr:   "autoscaling is trait, but with invalid format float64, should be map[string]interface{}",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			traits, err := GetTraits(tc.app, tc.compName)
			if tc.expErr != "" {
				assert.EqualError(t, err, tc.expErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.exp, traits)
			}
		})
	}
}
