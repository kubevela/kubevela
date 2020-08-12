package application

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
)

func TestApplication(t *testing.T) {
	yaml1 := `name: myapp
components:
  frontend:
    deployment:
      image: inanimate/echo-server
      env:
        PORT: 8080
    traits:
      autoscaling:
        max: 10
        min: 1
      rollout:
        strategy: canary
        step: 5
  backend:
    cloneset:
      image: "back:v1"
`
	yaml2 := `name: myapp`
	yaml3 := `components:
  frontend:
    deployment:
      image: inanimate/echo-server
      env:
        PORT: 8080`
	yaml4 := `name: myapp
components:
  frontend:
    deployment:
      image: inanimate/echo-server
    scopes:
      public-scope: true
appScopes:
  public-scope:
    networkPolicy: public`
	yaml5 := `name: myapp
components:
  frontend:
    traits:
      rollout:
        strategy: canary
        step: 5
  backend:
    cloneset:
      image: "back:v1"
`
	yaml6 := `name: myapp
components:
  frontend:
    deployment:
      image: inanimate/echo-server
    traits:
      autoscaling: 10`

	cases := map[string]struct {
		raw             string
		InValid         bool
		InvalidReason   error
		ExpName         string
		ExpComponents   []string
		WantWorkload    string
		ExpWorklaod     map[string]interface{}
		ExpWorkloadType string
		ExpTraits       map[string]map[string]interface{}
	}{
		"normal case backend": {
			raw:           yaml1,
			ExpName:       "myapp",
			ExpComponents: []string{"backend", "frontend"},
			WantWorkload:  "backend",
			ExpWorklaod: map[string]interface{}{
				"image": "back:v1",
			},
			ExpWorkloadType: "cloneset",
			ExpTraits:       map[string]map[string]interface{}{},
		},
		"normal case frontend": {
			raw:           yaml1,
			ExpName:       "myapp",
			ExpComponents: []string{"backend", "frontend"},
			WantWorkload:  "frontend",
			ExpWorklaod: map[string]interface{}{
				"image": "inanimate/echo-server",
				"env": map[string]interface{}{
					"PORT": float64(8080),
				},
			},
			ExpWorkloadType: "deployment",
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
			raw:           yaml2,
			ExpName:       "myapp",
			InValid:       true,
			InvalidReason: errors.New("at least one component is required"),
		},
		"no name": {
			raw:           yaml3,
			ExpName:       "",
			InValid:       true,
			InvalidReason: errors.New("name is required"),
		},
		"scopes not array": {
			raw:           yaml4,
			ExpName:       "myapp",
			InValid:       true,
			InvalidReason: fmt.Errorf("format of scopes in frontend must be string array"),
		},
		"workload not exist": {
			raw:           yaml5,
			ExpName:       "myapp",
			InValid:       true,
			InvalidReason: fmt.Errorf("you must have only one workload in component frontend"),
		},
		"trait must be map": {
			raw:           yaml6,
			ExpName:       "myapp",
			InValid:       true,
			InvalidReason: fmt.Errorf("trait autoscaling in frontend must be map"),
		},
	}
	for caseName, c := range cases {
		var app Application
		err := yaml.Unmarshal([]byte(c.raw), &app)
		assert.NoError(t, err, caseName)
		err = app.Validate()
		if c.InValid {
			assert.Equal(t, c.InvalidReason, err)
			continue
		}
		assert.Equal(t, c.ExpName, app.Name, caseName)
		assert.Equal(t, c.ExpComponents, app.GetComponents(), caseName)
		workloadType, workload, err := app.GetWorkload(c.WantWorkload)
		assert.NoError(t, err, caseName)
		assert.Equal(t, c.ExpWorklaod, workload, caseName)
		assert.Equal(t, c.ExpWorkloadType, workloadType, caseName)
		traits, err := app.GetTraits(c.WantWorkload)
		assert.NoError(t, err, caseName)
		assert.Equal(t, c.ExpTraits, traits, caseName)
	}
}
