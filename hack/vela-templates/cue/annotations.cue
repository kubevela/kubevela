patch: {
	spec: template: metadata: annotations: {
		for k, v in parameter {
			"\(k)": v
		}
	}
}
parameter: [string]: string
