package template

import (
	"cuelang.org/go/cue"
	"testing"
)



func TestTemplate(t *testing.T) {

	var (
		mock = &MockManager{}
	)

	cueTemplate := `
      context: {
         name: "test"
      }
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
      }
      `

	if err:=mock.AddWD(`
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
` + cueTemplate);err!=nil{
		t.Error(err)
		return
	}


	temp, kind, err := mock.LoadTemplate("worker")
	if err != nil {
		t.Error(err)
		return
	}
	if kind != WorkloadKind {
		t.Errorf("template.LoadTemplate kind invalid")
		return
	}

	var r cue.Runtime
	inst,err:=r.Compile("-",temp)
	if err!=nil{
		t.Error(err)
		return
	}
	instDest,err:=r.Compile("-",cueTemplate)
	if err!=nil{
		t.Error(err)
		return
	}
	s1,_:=inst.Value().String()
	s2,_:=instDest.Value().String()
	if s1!=s2 {
		t.Errorf("parsered template is not correct")
	}
}
