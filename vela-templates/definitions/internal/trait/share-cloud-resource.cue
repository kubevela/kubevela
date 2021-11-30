import (
	"vela/op"
)

"share-cloud-resource": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Sync secrets created by terraform component to runtime clusters so that runtime clusters can share the created cloud resource."
}
template: {
	app: op.#ShareCloudResource & {
		env:        parameter.env
		policy:     parameter.policy
		placements: parameter.placements
		// context.namespace indicates the namespace of the app
		namespace: context.namespace
		// context.namespace indicates the name of the app
		name: context.name
	}

	parameter: {
		env: string
		// +usage=Declare the location to bind
		placements: [...{
			namespace?: string
			cluster?:   string
		}]
		// +usage=Declare the name of the env-binding policy, if empty, the first env-binding policy will be used
		policy: *"" | string
		// +usage=Declare the name of the env in policy
		env: string
	}
}
