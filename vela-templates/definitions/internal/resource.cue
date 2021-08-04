resource: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add resource requests and limits to workloads."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps"]
	}
}
template: {
	patch: {
		spec: template: spec: containers: [{
			metadata: annotations: "customized-resource": "cpu:\(parameter.cpu) memory:\(parameter.memory)"
			resources: {
				requests: parameter
				limits:   parameter
			}
		}, ...]
	}
	parameter: {
		// +usage=Specify the amount of cpu to limit
		cpu: *1 | number
		// +usage=Specify the amount of memory to limit
		memory: *"2048Mi" | =~"^([1-9][0-9]{0,63})(E|P|T|G|M|K|Ei|Pi|Ti|Gi|Mi|Ki)$"
	}
}
