output: {
	apiVersion: "v1"
	kind:       "Job"
	spec: {
		parallelism: parameter.count
		completions: parameter.count
		template: spec: {
			containers: [{
				name:  context.name
				image: parameter.image

				if parameter["cmd"] != _|_ {
					command: parameter.cmd
				}
			}]
		}
	}
}
parameter: {
	// +usage=specify number of tasks to run in parallel
	// +short=c
	count: *1 | int

	// +usage=specify app image
	// +short=i
	image: string

	cmd?: [...string]
}
