resource: {
	type: "trait"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Add resource requests and limits on K8s pod for your workload which follows the pod spec in path 'spec.template.'"
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["*"]
	}
}
template: {
	patch: spec: template: spec: containers: [...{
		resources: {
			if parameter.cpu != _|_ && parameter.memory != _|_ && parameter.requests == _|_ && parameter.limits == _|_ {
				requests: {
					cpu:    parameter.cpu
					memory: parameter.memory
				}
				limits: {
					cpu:    parameter.cpu
					memory: parameter.memory
				}
			}

			if parameter.requests != _|_ {
				requests: {
					cpu:    parameter.requests.cpu
					memory: parameter.requests.memory
				}
			}
			if parameter.limits != _|_ {
				limits: {
					cpu:    parameter.limits.cpu
					memory: parameter.limits.memory
				}
			}
		}
	}]

	parameter: {
		// +usage=Specify the amount of cpu for requests and limits
		cpu?: *1 | number
		// +usage=Specify the amount of memory for requests and limits
		memory?: *"2048Mi" | =~"^([1-9][0-9]{0,63})(E|P|T|G|M|K|Ei|Pi|Ti|Gi|Mi|Ki)$"
		// +usage=Specify the resources in requests
		requests?: {
			// +usage=Specify the amount of cpu for requests
			cpu: *1 | number
			// +usage=Specify the amount of memory for requests
			memory: *"2048Mi" | =~"^([1-9][0-9]{0,63})(E|P|T|G|M|K|Ei|Pi|Ti|Gi|Mi|Ki)$"
		}
		// +usage=Specify the resources in limits
		limits?: {
			// +usage=Specify the amount of cpu for limits
			cpu: *1 | number
			// +usage=Specify the amount of memory for limits
			memory: *"2048Mi" | =~"^([1-9][0-9]{0,63})(E|P|T|G|M|K|Ei|Pi|Ti|Gi|Mi|Ki)$"
		}
	}
}
