package workloaddefinition

var Deployment = `apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: deployments.apps
  annotations:
    definition.oam.dev/apiVersion: "apps/v1"
    definition.oam.dev/kind: "Deployment"
spec:
  definitionRef:
    name: deployments.apps
  extension:
    template: |
      #Template: {
      	apiVersion: "apps/v1"
      	kind:       "Deployment"
      	metadata: name: deployment.name
      	spec: {
      		selector:
      			matchLabels:
      				app: deployment.name
      		template: {
      			metadata:
      				labels:
      					app: deployment.name
      			spec: containers: [{
      				image: deployment.image
      				name:  deployment.name
      				env:   deployment.env
      				ports: [{
      					containerPort: deployment.port
      					protocol:      "TCP"
      					name:          "default"
      				}]
      			}]
      		}
      	}
      }

      deployment: {
      	name: string
      	// +usage=specify app image
      	// +short=i
      	image: string
      	// +usage=specify port for container
      	// +short=p
      	port: *8080 | int
      	env: [...{
      		name:  string
      		value: string
      	}]
      }`
