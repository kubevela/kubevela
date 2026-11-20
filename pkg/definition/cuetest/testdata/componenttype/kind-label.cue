"kind-label": {
	type: "trait"
	attributes: {
		appliesToWorkloads: ["*"]
		status: customStatus: #"""
			message: "on a \(context.outputs.record.data.type)"
			"""#
	}
}
template: {
	patch: metadata: labels: "example.com/component-type": context.componentType
	outputs: record: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: "\(context.name)-type"
		data: type: context.componentType
	}
	parameter: {}
}
