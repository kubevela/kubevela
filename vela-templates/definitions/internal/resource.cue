resource: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add resource requests and limits on K8s pod for your workload which follows the pod spec in path 'spec.template.'"
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["*"]
	}
}
template: {
	patch: spec: template: spec: containers: [{
		metadata: annotations: "customized-resource": "cpu:\(parameter.cpu) memory:\(parameter.memory)"
		resources: {
			requests: parameter
			limits:   parameter
		}
	}, ...]
	parameter: {
		// +usage=Specify the amount of cpu to limit
		cpu: *1 | number
		// +usage=Specify the amount of memory to limit
		memory: *"2048Mi" | =~"^([1-9][0-9]{0,63})(E|P|T|G|M|K|Ei|Pi|Ti|Gi|Mi|Ki)$"
	}
}
