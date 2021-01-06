package parser

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/ghodss/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
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
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: application-sample
spec:
  components:
    - name: myweb
      type: worker
      settings:
        image: "busybox"
        cmd:
        - sleep
        - "1000"
      traits:
        - name: scaler
          properties:
            replicas: 10
`

	o := v1alpha2.Application{}
	yaml.Unmarshal([]byte(appfileYaml), &o)

	appfile, err := NewParser(template.GetHandler(mock)).Parse("test", &o)
	if err != nil {
		t.Error(err)
		return
	}
	if !equal(TestExceptApp, appfile) {
		t.Error("parser appfile wrong")
	}

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
