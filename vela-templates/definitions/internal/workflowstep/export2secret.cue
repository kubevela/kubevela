import (
	"vela/op"
)

"export2secret": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Export data to Kubernetes Secret in your workflow."
}
template: {
	apply: op.#Apply & {
		value: {
			apiVersion: "v1"
			kind:       "Secret"
			if parameter.type != _|_ {
				type: parameter.type
			}
			metadata: {
				name: parameter.secretName
				if parameter.namespace != _|_ {
					namespace: parameter.namespace
				}
				if parameter.namespace == _|_ {
					namespace: context.namespace
				}
			}
			stringData: parameter.data
		}
		cluster: parameter.cluster
	}
	parameter: {
		// +usage=Specify the name of the secret
		secretName: string
		// +usage=Specify the namespace of the secret
		namespace?: string
		// +usage=Specify the type of the secret
		type?: string
		// +usage=Specify the data of secret
		data: {}
		// +usage=Specify the cluster of the config map
		cluster: *"" | string
	}
}
