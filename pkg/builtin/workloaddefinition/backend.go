package workloaddefinition

var BackendService = `apiVersion: standard.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: backend-service
  annotations:
    definition.oam.dev/apiVersion: "standard.oam.dev/v1alpha2"
    definition.oam.dev/kind: "Containerized"
spec:
  definitionRef:
    name: containerizeds.core.oam.dev
  childResourceKinds:
    - apiVersion: apps/v1
      kind: Deployment
    - apiVersion: v1
      kind: Service
  extension:
	defaultTraits:
        - scale
        - route
        - monitor
    template: |
      #Template: {
      	apiVersion: "core.oam.dev/v1alpha2"
      	kind:       "Containerized"
      	metadata: name: backend-service
      	spec: {
            replicas: containerized.replicas
			podSpec:
				containers: [{
					image: containerized.image
					name:  containerized.name
					ports: [{
						containerPort: containerized.port
						protocol:      "TCP"
						name:          "default"
					}]
				}]
      	}
      }
      containerized: {
		// +usage=specify replicas
      	// +short=r
      	replicas: *1 | int
      	name: string
      	// +usage=specify app image
      	// +short=i
      	image: string
      	// +usage=specify port for container
      	// +short=p
      	port: *6379 | int
      }
`
