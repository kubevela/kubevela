"init-container": {
	type: "trait"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
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
			name:  parameter.name
			image: parameter.image
			if parameter.cmd != _|_ {
				command: parameter.cmd
			}
			if parameter.args != _|_ {
				args: parameter.args
			}

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
		// +usage=Specify the name of init container
		name: string

		// +usage=Specify the image of init container
		image: string

		// +usage=Specify the commands run in the init container
		cmd?: [...string]

		// +usage=Specify the args run in the init container
		args?: [...string]

		// +usage=Specify the mount name of shared volume
		mountName: *"workdir" | string

		// +usage=Specify the mount path of app container
		appMountPath: string

		// +usage=Specify the mount path of init container
		initMountPath: string
	}
}
