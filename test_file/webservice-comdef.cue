// A KubeVela ComponentDefinition for a simple containerized service
containerizedservice: {
	type: "component"
	annotations: {}
	labels: {}
	description: "Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers."
	attributes: {
		workload: {
			definition: {
				apiVersion: "apps/v1"
				kind:       "Deployment"
			}
			type: "deployments.apps"
		}
	}
}

template: {
	output: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: {
			name: context.name
			labels: {
				app: context.name
			}
		}
		spec: {
			replicas: parameter.replicas
			selector: {
				matchLabels: {
					app: context.name
				}
			}
			template: {
				metadata: {
					labels: {
						app: context.name
					}
				}
				spec: {
					containers: [{
						name:  context.name
						image: parameter.image
					}]
				}
			}
		}
	}
	// What users must/provide in their Application
	parameter: {
		image:     string
		replicas?: *1 | int
		paramfortestingonly: *4 | int
	}
}
