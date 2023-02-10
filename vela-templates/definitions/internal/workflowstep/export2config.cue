import (
	"vela/op"
)

"export2config": {
	type: "workflow-step"
	annotations: {
		"category": "Resource Management"
	}
	description: "Export data to specified Kubernetes ConfigMap in your workflow."
}
template: {
	apply: op.#Apply & {
		value: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {
				name: parameter.configName
				if parameter.namespace != _|_ {
					namespace: parameter.namespace
				}
				if parameter.namespace == _|_ {
					namespace: context.namespace
				}
			}
			data: parameter.data
		}
		cluster: parameter.cluster
	}
	parameter: {
		// +usage=Specify the name of the config map
		configName: string
		// +usage=Specify the namespace of the config map
		namespace?: string
		// +usage=Specify the data of config map
		data: {}
		// +usage=Specify the cluster of the config map
		cluster: *"" | string
	}
}
