"demo-deployment": {
	type:        "component"
	annotations: {}
	labels:      {}
	description: "Demo nginx Deployment with typed parameters for source-driven name, replica count, and labels."
	attributes: workload: {
		definition: {
			apiVersion: "apps/v1"
			kind:       "Deployment"
		}
		type: "deployments.apps"
	}
}
template: {
	parameter: {
		name:        string
		replicas:    int
		region:      string
		zone:        string
		department:  string
		tenant:      string
		environment: string
	}

	output: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: {
			name: parameter.name
			labels: {
				"example.com/region":      parameter.region
				"example.com/zone":        parameter.zone
				"example.com/department":  parameter.department
				"example.com/tenant":      parameter.tenant
				"example.com/environment": parameter.environment
			}
		}
		spec: {
			replicas: parameter.replicas
			selector: matchLabels: app: context.name
			template: {
				metadata: labels: {
					app:                   context.name
					"example.com/tenant":  parameter.tenant
				}
				spec: containers: [{
					name:  "app"
					image: "nginx:1.27-alpine"
				}]
			}
		}
	}
}
