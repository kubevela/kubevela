"e2e-widget": {
	type: "component"
	annotations: {}
	labels: {}
	description: "A trivial component that renders a single ConfigMap, used as an e2e module fixture."
	attributes: {
		workload: {
			definition: {
				apiVersion: "v1"
				kind:       "ConfigMap"
			}
		}
	}
}
template: {
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name: context.name
		}
		data: {
			widget: parameter.message
		}
	}
	parameter: {
		// message is the value stored in the ConfigMap.
		message: *"hello from e2e-widget" | string
	}
}
