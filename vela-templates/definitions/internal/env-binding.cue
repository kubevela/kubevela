"env-binding": {
	type: "policy"
	annotations: {}
	labels: {}
	description: "Provides differentiated configuration and environment scheduling policies for application."
}
template: {
	output: {
		apiVersion: "core.oam.dev/v1alpha1"
		kind:       "EnvBinding"
		spec: {
			engine: parameter.engine
			appTemplate: {
				apiVersion: "core.oam.dev/v1beta1"
				kind:       "Application"
				metadata: {
					name:      context.appName
					namespace: context.namespace
				}
				spec: {
					components: context.components
				}
			}
			envs: parameter.envs
			if !parameter.created {
				outputResourcesTo: {
					name:      context.name
					namespace: context.namespace
				}
			}
		}
	}
	#Env: {
		name: string
		patch: components: [...{
			name: string
			type: string
			properties: {...}
			traits?: {
				type: string
				properties: {...}
			}
		}]
		placement: {
			clusterSelector?: {
				labels?: [string]: string
				name?: string
			}
			namespaceSelector?: {
				labels?: [string]: string
				name?: string
			}
		}
	}
	parameter: {
		engine: *"local" | string
		envs: [...#Env]
		created: *true | bool
	}
}
