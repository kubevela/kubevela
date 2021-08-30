"init-container": {
	type: "trait"
	annotations: {}
	labels: {}
	description: "add an init container and use shared volume with pod"
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps"]
	}
}
template: {
	patch: spec: template: spec: {
		// +patchKey=name
		containers: [{
			name: context.name
			// +patchKey=name
			volumeMounts: [{
				name:      parameter.mountName
				mountPath: parameter.appMountPath
			}]
		}]
		initContainers: [{
			name:    parameter.name
			image:   parameter.image
			command: parameter.command
			// +patchKey=name
			volumeMounts: [{
				name:      parameter.mountName
				mountPath: parameter.initMountPath
			}]
		}]
		// +patchKey=name
		volumes: [{
			name: parameter.mountName
			emptyDir: {}
		}]
	}
	parameter: {
		name:  string
		image: string
		command?: [...string]
		mountName:     *"workdir" | string
		appMountPath:  string
		initMountPath: string
	}
}
