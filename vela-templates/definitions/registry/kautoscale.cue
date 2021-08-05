import "encoding/json"

kautoscale: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Specify auto scale by annotation"
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps"]
	}
}
template: {
	patch: metadata: annotations: "my.autoscale.ann": json.Marshal({
		minReplicas: parameter.min
		maxReplicas: parameter.max
	})
	parameter: {
		min: *1 | int
		max: *3 | int
	}
}
