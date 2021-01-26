package util

import (
	"context"
	"testing"

	"cuelang.org/go/cue"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
)

func TestTemplate(t *testing.T) {
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

	var workloadDefintion = `
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
` + cueTemplate

	// Create mock client
	tclient := test.MockClient{
		MockGet: func(ctx context.Context, key ktypes.NamespacedName, obj runtime.Object) error {
			switch o := obj.(type) {
			case *v1alpha2.WorkloadDefinition:
				wd, err := UnMarshalStringToWorkloadDefinition(workloadDefintion)
				if err != nil {
					return err
				}
				*o = *wd
			}
			return nil
		},
	}

	temp, err := LoadTemplate(&tclient, "worker", types.TypeWorkload)

	if err != nil {
		t.Error(err)
		return
	}
	var r cue.Runtime
	inst, err := r.Compile("-", temp.TemplateStr)
	if err != nil {
		t.Error(err)
		return
	}
	instDest, err := r.Compile("-", cueTemplate)
	if err != nil {
		t.Error(err)
		return
	}
	s1, _ := inst.Value().String()
	s2, _ := instDest.Value().String()
	if s1 != s2 {
		t.Errorf("parsered template is not correct")
	}
}
