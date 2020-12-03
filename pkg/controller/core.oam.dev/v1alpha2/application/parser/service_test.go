package parser

import (
	"fmt"
	"reflect"
	"testing"

	"cuelang.org/go/cue"
	"gopkg.in/yaml.v3"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/defclient"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/template"
)

func TestParser(t *testing.T) {
	mock := &defclient.MockClient{}
	mock.AddTD(`
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the app"
  name: scaler
spec:
  appliesToWorkloads:
    - webservice
    - worker
  definitionRef:
    name: manualscalertraits.core.oam.dev
  workloadRefPath: spec.workloadRef
  extension:
    template: |-
      output: {
      	apiVersion: "core.oam.dev/v1alpha2"
      	kind:       "ManualScalerTrait"
      	spec: {
      		replicaCount: parameter.replicas
      	}
      }
      parameter: {
      	//+short=r
      	replicas: *1 | int
      }`)
	mock.AddWD(`
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: worker
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  definitionRef:
    name: deployments.apps
  extension:
    template: |
      output: {
      	apiVersion: "apps/v1"
      	kind:       "Deployment"
      	spec: {
      		selector: matchLabels: {
      			"app.oam.dev/component": context.name
      		}
      
      		template: {
      			metadata: labels: {
      				"app.oam.dev/component": context.name
      			}
      
      			spec: {
      				containers: [{
      					name:  context.name
      					image: parameter.image
      
      					if parameter["cmd"] != _|_ {
      						command: parameter.cmd
      					}
      				}]
      			}
      		}
      
      		selector:
      			matchLabels:
      				"app.oam.dev/component": context.name
      	}
      }
      
      parameter: {
      	// +usage=Which image would you like to use for your service
      	// +short=i
      	image: string
      
      	cmd?: [...string]
      }`)

	const appfileYaml = `
services:
   myweb:
     type: worker
     image: "busybox"
     cmd:
     - sleep
     - "1000"
     scaler:
        replicas: 10
`

	o := map[string]interface{}{}
	yaml.Unmarshal([]byte(appfileYaml), &o)

	appfile, err := NewParser(template.GetHanler(mock)).Parse("test", o)
	if err != nil {
		t.Error(err)
		return
	}

	if !equal(TestExceptApp, appfile) {
		t.Error("parser appfile wrong")
	}

}

func TestEval(t *testing.T) {

	traitDef := `
output: {
      	apiVersion: "core.oam.dev/v1alpha2"
      	kind:       "ManualScalerTrait"
      	spec: {
      		replicaCount: 10
      	}
}`
	trait := &Trait{
		template: traitDef,
	}

	trs, err := trait.Eval(&mockRender{})
	if err != nil {
		t.Error(err)
		return
	}

	if len(trs) != 1 {
		t.Errorf("output means there is only one trait")
	}

	workloadDef := `
output: {
      	apiVersion: "apps/v1"
      	kind:       "Deployment"
      	spec: {
      		selector: matchLabels: {
      			"app.oam.dev/component": "test"
      		}
      
      		template: {
      			metadata: labels: {
      				"app.oam.dev/component": "test"
      			}
      
      			spec: {
      				containers: [{
      					name:  "test"
      					image: "parameter.image"
      				}]
      			}
      		}
      
      		selector:
      			matchLabels:
      				"app.oam.dev/component": "test"
      	}
}`
	workload := &Workload{
		template: workloadDef,
	}

	if _, err := workload.Eval(&mockRender{}); err != nil {
		t.Error(err)
	}

}

type mockRender struct {
	body string
}

// WithParams Mock Fill Params
func (mr *mockRender) WithParams(params interface{}) Render {
	return mr
}

// WithTemplate Mock Fill Params
func (mr *mockRender) WithTemplate(raw string) Render {
	mr.body = raw
	return mr
}

// Complete generate cue instance
func (mr *mockRender) Complete() (*cue.Instance, error) {
	var r cue.Runtime
	return r.Compile("-", mr.body)
}

func equal(af, dest *Appfile) bool {
	if af.name != dest.name || len(af.services) != len(dest.services) {
		return false
	}
	for i, wd := range af.Services() {
		destWd := dest.services[i]
		if wd.name != destWd.name || len(wd.traits) != len(destWd.traits) {
			return false
		}
		if !reflect.DeepEqual(wd.params, destWd.params) {
			fmt.Printf("%#v | %#v\n", wd.params, destWd.params)
			return false
		}
		for j, td := range wd.Traits() {
			destTd := destWd.traits[j]
			if td.name != destTd.name {
				return false
			}
			if !reflect.DeepEqual(td.params, destTd.params) {
				fmt.Printf("%#v | %#v\n", td.params, destTd.params)
				return false
			}

		}
	}
	return true
}
