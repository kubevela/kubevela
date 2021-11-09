import (
	"vela/op"
)

"export2secret": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Export data to secret for your workflow steps"
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
	}
}
