"init-container": {
	type: "trait"
	annotations: {}
	description: "add an init container and use shared volume with pod"
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
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
		// +patchKey=name
		initContainers: [{
			name:            parameter.name
			image:           parameter.image
			imagePullPolicy: parameter.imagePullPolicy
			if parameter.cmd != _|_ {
				command: parameter.cmd
			}
			if parameter.args != _|_ {
				args: parameter.args
			}
			if parameter["env"] != _|_ {
				env: parameter.env
			}

			// +patchKey=name
			volumeMounts: [{
				name:      parameter.mountName
				mountPath: parameter.initMountPath
			}] + parameter.extraVolumeMounts
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

		// +usage=Specify image pull policy for your service
		imagePullPolicy: *"IfNotPresent" | "Always" | "Never"

		// +usage=Specify the commands run in the init container
		cmd?: [...string]

		// +usage=Specify the args run in the init container
		args?: [...string]

		// +usage=Specify the env run in the init container
		env?: [...{
			// +usage=Environment variable name
			name: string
			// +usage=The value of the environment variable
			value?: string
			// +usage=Specifies a source the value of this var should come from
			valueFrom?: {
				// +usage=Selects a key of a secret in the pod's namespace
				secretKeyRef?: {
					// +usage=The name of the secret in the pod's namespace to select from
					name: string
					// +usage=The key of the secret to select from. Must be a valid secret key
					key: string
				}
				// +usage=Selects a key of a config map in the pod's namespace
				configMapKeyRef?: {
					// +usage=The name of the config map in the pod's namespace to select from
					name: string
					// +usage=The key of the config map to select from. Must be a valid secret key
					key: string
				}
			}
		}]

		// +usage=Specify the mount name of shared volume
		mountName: *"workdir" | string

		// +usage=Specify the mount path of app container
		appMountPath: string

		// +usage=Specify the mount path of init container
		initMountPath: string

		// +usage=Specify the extra volume mounts for the init container
		extraVolumeMounts: [...{
			// +usage=The name of the volume to be mounted
			name: string
			// +usage=The mountPath for mount in the init container
			mountPath: string
		}]
	}
}
