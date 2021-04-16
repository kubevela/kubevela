patch: {
	// +patchKey=name
	spec: template: spec: containers: [parameter]
}
parameter: {
	// +usage=Specify the name of sidecar container
	name: string

	// +usage=Specify the image of sidecar container
	image: string

	// +usage=Specify the commands run in the sidecar
	command?: [...string]
}
