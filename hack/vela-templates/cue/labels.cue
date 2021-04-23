patch: {
	spec: template: metadata: labels: {
		for k, v in parameter.labels {
			"\(k)": v
		}
	}
}
parameter: {
	labels: [string]: string
}
