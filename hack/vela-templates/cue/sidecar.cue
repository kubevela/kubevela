patch: {
	// +patchKey=name
	spec: template: spec: containers: [parameter]
}
parameter: {
	name:  string
	image: string
	command?: [...string]
}
