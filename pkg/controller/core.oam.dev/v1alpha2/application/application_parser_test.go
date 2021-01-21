package application

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/ghodss/yaml"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var expectedExceptApp = &Appfile{
	Name: "test",
	Workloads: []*Workload{
		{
			Name: "myweb",
			Type: "worker",
			Params: map[string]interface{}{
				"image": "busybox",
				"cmd":   []interface{}{"sleep", "1000"},
			},
			Template: `
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
      }`,
			Traits: []*Trait{
				{
					Name: "scaler",
					Params: map[string]interface{}{
						"replicas": float64(10),
					},
					Template: `
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
      }
`,
				},
			},
		},
	},
}

const traitDefinition = `
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
      }`

const workloadDefinition = `
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
      }`

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

var _ = Describe("Test application parser", func() {
	It("Test we can parse an application to an appFile", func() {
		o := v1alpha2.Application{}
		err := yaml.Unmarshal([]byte(appfileYaml), &o)
		Expect(err).ShouldNot(HaveOccurred())

		// Create mock client
		tclient := test.MockClient{
			MockGet: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				switch o := obj.(type) {
				case *v1alpha2.WorkloadDefinition:
					wd, err := util.UnMarshalStringToWorkloadDefinition(workloadDefinition)
					if err != nil {
						return err
					}
					*o = *wd
				case *v1alpha2.TraitDefinition:
					td, err := util.UnMarshalStringToTraitDefinition(traitDefinition)
					if err != nil {
						return err
					}
					*o = *td
				}
				return nil
			},
		}

		appfile, err := NewApplicationParser(&tclient, nil).GenerateAppFile("test", &o)
		Expect(err).ShouldNot(HaveOccurred())

		Expect(equal(expectedExceptApp, appfile)).Should(BeTrue())
	})
})

func equal(af, dest *Appfile) bool {
	if af.Name != dest.Name || len(af.Workloads) != len(dest.Workloads) {
		return false
	}
	for i, wd := range af.Workloads {
		destWd := dest.Workloads[i]
		if wd.Name != destWd.Name || len(wd.Traits) != len(destWd.Traits) {
			return false
		}
		if !reflect.DeepEqual(wd.Params, destWd.Params) {
			fmt.Printf("%#v | %#v\n", wd.Params, destWd.Params)
			return false
		}
		for j, td := range wd.Traits {
			destTd := destWd.Traits[j]
			if td.Name != destTd.Name {
				return false
			}
			if !reflect.DeepEqual(td.Params, destTd.Params) {
				fmt.Printf("%#v | %#v\n", td.Params, destTd.Params)
				return false
			}

		}
	}
	return true
}
