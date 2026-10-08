"config-store": {
	attributes: workload: definition: {
		apiVersion: "v1"
		kind:       "ConfigMap"
	}
	description: "A ConfigMap holding a single key, used to prove an addon-provided component type is usable."
	labels: {}
	type: "component"
}

template: {
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: context.name
		data: parameter.data
	}

	parameter: {
		// +usage=Keys written into the ConfigMap
		data: [string]: string
	}
}
