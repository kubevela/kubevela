package util

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"cuelang.org/go/cue"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
)

func TestLoadWorkloadTemplate(t *testing.T) {
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

func TestLoadTraitTemplate(t *testing.T) {
	cueTemplate := `
      parameter: {
      	domain: string
      	http: [string]: int
      }
      context: {
         name: "test"
      }
      // trait template can have multiple outputs in one trait
      outputs: service: {
      	apiVersion: "v1"
      	kind:       "Service"
      	metadata:
      		name: context.name
      	spec: {
      		selector:
      			"app.oam.dev/component": context.name
      		ports: [
      			for k, v in parameter.http {
      				port:       v
      				targetPort: v
      			},
      		]
      	}
      }
      
      outputs: ingress: {
      	apiVersion: "networking.k8s.io/v1beta1"
      	kind:       "Ingress"
      	metadata:
      		name: context.name
      	spec: {
      		rules: [{
      			host: parameter.domain
      			http: {
      				paths: [
      					for k, v in parameter.http {
      						path: k
      						backend: {
      							serviceName: context.name
      							servicePort: v
      						}
      					},
      				]
      			}
      		}]
      	}
      }
      `

	var traitDefintion = `
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Configures K8s ingress and service to enable web traffic for your service.
    Please use route trait in cap center for advanced usage."
  name: ingress
spec:
  status:
    customStatus: |-
      if len(context.outputs.ingress.status.loadBalancer.ingress) > 0 {
      	message: "Visiting URL: " + context.outputs.ingress.spec.rules[0].host + ", IP: " + context.outputs.ingress.status.loadBalancer.ingress[0].ip
      }
      if len(context.outputs.ingress.status.loadBalancer.ingress) == 0 {
      	message: "No loadBalancer found, visiting by using 'vela port-forward " + context.appName + " --route'\n"
      }
    healthPolicy: |
      isHealth: len(context.outputs.service.spec.clusterIP) > 0
  appliesToWorkloads:
    - webservice
    - worker
  template: |
` + cueTemplate

	// Create mock client
	tclient := test.MockClient{
		MockGet: func(ctx context.Context, key ktypes.NamespacedName, obj runtime.Object) error {
			switch o := obj.(type) {
			case *v1alpha2.TraitDefinition:
				wd, err := UnMarshalStringToTraitDefinition(traitDefintion)
				if err != nil {
					return err
				}
				*o = *wd
			}
			return nil
		},
	}

	temp, err := LoadTemplate(&tclient, "ingress", types.TypeTrait)

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

func TestNewTemplate(t *testing.T) {
	testCases := map[string]struct {
		tmp    string
		status *v1alpha2.Status
		ext    *runtime.RawExtension
		exp    *Template
	}{
		"only tmp": {
			tmp: "t1",
			exp: &Template{
				TemplateStr: "t1",
			},
		},
		"no tmp,but has extension": {
			ext: &runtime.RawExtension{Raw: []byte(`{"template":"t1"}`)},
			exp: &Template{
				TemplateStr: "t1",
			},
		},
		"no tmp,but has extension without temp": {
			ext: &runtime.RawExtension{Raw: []byte(`{"template":{"t1":"t2"}}`)},
			exp: &Template{
				TemplateStr: "",
			},
		},
		"tmp with status": {
			tmp: "t1",
			status: &v1alpha2.Status{
				CustomStatus: "s1",
				HealthPolicy: "h1",
			},
			exp: &Template{
				TemplateStr:  "t1",
				CustomStatus: "s1",
				Health:       "h1",
			},
		},
		"no tmp only status": {
			status: &v1alpha2.Status{
				CustomStatus: "s1",
				HealthPolicy: "h1",
			},
			exp: &Template{
				CustomStatus: "s1",
				Health:       "h1",
			},
		},
	}
	for reason, casei := range testCases {
		gtmp, err := NewTemplate(casei.tmp, casei.status, casei.ext)
		assert.NoError(t, err, reason)
		assert.Equal(t, gtmp, casei.exp, reason)
	}
}
