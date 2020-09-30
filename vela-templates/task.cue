data: {
	apiVersion: "batch/v1"
	kind:       "Job"
	metadata: name: parameter.name
	spec: {
		parallelism: parameter.count
		completions: parameter.count
		template:
			spec:
				containers: [
					for _, c in parameter.containers {
						image: c.image
						name:  c.name
					}]
	}
}
#task: {
	name: string
	// +usage=specify number of tasks to run in parallel
	// +short=c
	count: *1 | int
	containers: [ ...{
		name: string
		// +usage=specify app image
		// +short=i
		image: string
	}]
}
parameter: #task
// below is a sample value
parameter: {
	name: "container-component"
	containers: [
		{
			name:  "c1"
			image: "image1"
		},
		{
			name:  "c2"
			image: "image2"
		},
	]
}
