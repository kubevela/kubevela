package application

import (
	"errors"
	"fmt"
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/appfile"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
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
		app := newApplication(nil, tm)
		err := yaml.Unmarshal([]byte(c.raw), &app)
		assert.NoError(t, err, caseName)
		err = app.Validate()
		if c.InValid {
			assert.Equal(t, c.InvalidReason, err)
			continue
		}
		assert.Equal(t, c.ExpName, app.Name, caseName)
		assert.Equal(t, c.ExpComponents, app.GetComponents(), caseName)
		workloadType, workload := app.GetWorkload(c.WantWorkload)
		assert.Equal(t, c.ExpWorkload, workload, caseName)
		assert.Equal(t, c.ExpWorkloadType, workloadType, caseName)
		traits, err := app.GetTraits(c.WantWorkload)
		assert.NoError(t, err, caseName)
		assert.Equal(t, c.ExpTraits, traits, caseName)
	}
}

func TestAddWorkloadTypeLabel(t *testing.T) {
	tests := map[string]struct {
		comps    []*v1alpha2.Component
		services map[string]appfile.Service
		expect   []*v1alpha2.Component
	}{
		"empty case": {
			comps:    []*v1alpha2.Component{},
			services: map[string]appfile.Service{},
			expect:   []*v1alpha2.Component{},
		},
		"add type to labels normal case": {
			comps: []*v1alpha2.Component{
				{
					ObjectMeta: v1.ObjectMeta{Name: "mycomp"},
					Spec:       v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &unstructured.Unstructured{Object: map[string]interface{}{}}}},
				},
			},
			services: map[string]appfile.Service{
				"mycomp": {"type": "kubewatch"},
			},
			expect: []*v1alpha2.Component{
				{
					ObjectMeta: v1.ObjectMeta{Name: "mycomp"},
					Spec: v1alpha2.ComponentSpec{
						Workload: runtime.RawExtension{
							Object: &unstructured.Unstructured{Object: map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"workload.oam.dev/type": "kubewatch",
									}}}}},
					},
				},
			},
		},
	}
	for key, ca := range tests {
		addWorkloadTypeLabel(ca.comps, ca.services)
		assert.Equal(t, ca.expect, ca.comps, key)
	}
}
