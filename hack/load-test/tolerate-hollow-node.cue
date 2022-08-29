"tolerate-hollow-node": {
	annotations: {}
	attributes: {}
	description: "Tolerate hollow nodes"
	labels: {}
	type: "trait"
}

template: {
	patch: spec: template: spec: {
		tolerations: [{
			key:      "node.kubernetes.io/network-unavailable"
			operator: "Exists"
			effect:   "NoSchedule"
		}]
	}
	parameter: {}
}
