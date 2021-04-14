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

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/references/appfile/template"
)

func TestApplication(t *testing.T) {
	yamlNormal := `name: myapp
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
	yamlNoService := `name: myapp`
	yamlNoName := `services:
  frontend:
    image: inanimate/echo-server
    env:
      PORT: 8080`
	yamlTraitNotMap := `name: myapp
services:
  frontend:
    image: inanimate/echo-server
    env:
      PORT: 8080
    autoscaling: 10`

	cases := map[string]struct {
		raw             string
		InValid         bool
		InvalidReason   error
		ExpName         string
		ExpComponents   []string
		WantWorkload    string
		ExpWorkload     map[string]interface{}
		ExpWorkloadType string
		ExpTraits       map[string]map[string]interface{}
	}{
		"normal case backend": {
			raw:           yamlNormal,
			ExpName:       "myapp",
			ExpComponents: []string{"backend", "frontend"},
			WantWorkload:  "backend",
			ExpWorkload: map[string]interface{}{
				"image": "back:v1",
			},
			ExpWorkloadType: "cloneset",
			ExpTraits:       map[string]map[string]interface{}{},
		},
		"normal case frontend": {
			raw:           yamlNormal,
			ExpName:       "myapp",
			ExpComponents: []string{"backend", "frontend"},
			WantWorkload:  "frontend",
			ExpWorkload: map[string]interface{}{
				"image": "inanimate/echo-server",
				"env": map[string]interface{}{
					"PORT": float64(8080),
				},
			},
			ExpWorkloadType: "webservice",
			ExpTraits: map[string]map[string]interface{}{
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
		"no component": {
			raw:           yamlNoService,
			ExpName:       "myapp",
			InValid:       true,
			InvalidReason: errors.New("at least one service is required"),
		},
		"no name": {
			raw:           yamlNoName,
			ExpName:       "",
			InValid:       true,
			InvalidReason: errors.New("name is required"),
		},
		"trait must be map": {
			raw: yamlTraitNotMap,
			ExpTraits: map[string]map[string]interface{}{
				"autoscaling": {},
			},
			ExpName:       "myapp",
			InValid:       true,
			InvalidReason: fmt.Errorf("trait autoscaling in 'frontend' must be map"),
		},
	}

	for caseName, c := range cases {
		tm := template.NewFakeTemplateManager()
		for k := range c.ExpTraits {
			tm.Templates[k] = &template.Template{
				Captype: types.TypeTrait,
			}
		}
		app := NewApplication(nil, tm)
		err := yaml.Unmarshal([]byte(c.raw), &app)
		assert.NoError(t, err, caseName)
		err = Validate(app)
		if c.InValid {
			assert.Equal(t, c.InvalidReason, err)
			continue
		}
		assert.Equal(t, c.ExpName, app.Name, caseName)
		workloadType, workload := GetWorkload(app, c.WantWorkload)
		assert.Equal(t, c.ExpWorkload, workload, caseName)
		assert.Equal(t, c.ExpWorkloadType, workloadType, caseName)
		traits, err := GetTraits(app, c.WantWorkload)
		assert.NoError(t, err, caseName)
		assert.Equal(t, c.ExpTraits, traits, caseName)
	}
}
