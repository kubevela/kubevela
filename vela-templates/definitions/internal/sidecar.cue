sidecar: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Inject a sidecar container to the component."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps"]
	}
}
template: {
	patch: {
		// +patchKey=name
		spec: template: spec: containers: [{
			name:  parameter.name
			image: parameter.image
			if parameter.cmd != _|_ {
				command: parameter.cmd
			}
			if parameter["volumes"] != _|_ {
				volumeMounts: [ for v in parameter.volumes {
					{
						mountPath: v.path
						name:      v.name
					}
				}]
			}
		}]
	}
	parameter: {
		// +usage=Specify the name of sidecar container
		name: string

		// +usage=Specify the image of sidecar container
		image: string

		// +usage=Specify the commands run in the sidecar
		cmd?: [...string]

		// +usage=Specify the shared volume path
		volumes?: [...{
			name: string
			path: string
		}]
	}
}
