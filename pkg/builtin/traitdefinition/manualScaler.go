package traitdefinition

var ManualScaler = `apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata: 
  annotations: 
    oam.appengine.info/apiVersion: core.oam.dev/v1alpha2
    oam.appengine.info/kind: ManualScalerTrait
  name: manualscalertraits.core.oam.dev
spec: 
  appliesToWorkloads: 
    - core.oam.dev/v1alpha2.ContainerizedWorkload
    - apps/v1.Deployment
  definitionRef: 
    name: manualscalertraits.core.oam.dev
  extension: 
    template: |-
        #Template: {
        	apiVersion: "core.oam.dev/v1alpha2"
        	kind:       "ManualScalerTrait"
        	spec: {
        		replicaCount: scale.replica
        	}
        }
        scale: {
        	//+short=r
        	replica: *2 | int
        }
  workloadRefPath: spec.workloadRef
`
