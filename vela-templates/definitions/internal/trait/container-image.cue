"container-image": {
	type: "trait"
	annotations: {}
	description: "Set the image of the container."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
	}
}
template: {
	#PatchParams: {
		// +usage=Specify the name of the target container, if not set, use the component name
		containerName: *"" | string
		// +usage=Specify the image of the container
		image: string
		// +usage=Specify the image pull policy of the container
		imagePullPolicy: *"" | "IfNotPresent" | "Always" | "Never"
	}
	PatchContainer: {
		_params:         #PatchParams
		name:            _params.containerName
		_baseContainers: context.output.spec.template.spec.containers
		_matchContainers_: [ for _container_ in _baseContainers if _container_.name == name {_container_}]
		_baseContainer: *_|_ | {...}
		if len(_matchContainers_) == 0 {
			err: "container \(name) not found"
		}
		if len(_matchContainers_) > 0 {
			// +patchStrategy=retainKeys
			image: _params.image

			if _params.imagePullPolicy != "" {
				// +patchStrategy=retainKeys
				imagePullPolicy: _params.imagePullPolicy
			}
		}
	}
	patch: spec: template: spec: {
		if parameter.containers == _|_ {
			// +patchKey=name
			containers: [{
				PatchContainer & {_params: {
					if parameter.containerName == "" {
						containerName: context.name
					}
					if parameter.containerName != "" {
						containerName: parameter.containerName
					}
					image:           parameter.image
					imagePullPolicy: parameter.imagePullPolicy
				}}
			}]
		}
		if parameter.containers != _|_ {
			// +patchKey=name
			containers: [ for c in parameter.containers {
				if c.containerName == "" {
					err: "containerName must be set for containers"
				}
				if c.containerName != "" {
					PatchContainer & {_params: c}
				}
			}]
		}
	}

	parameter: #PatchParams | close({
		// +usage=Specify the container image for multiple containers
		containers: [...#PatchParams]
	})

	errs: [ for c in patch.spec.template.spec.containers if c.err != _|_ {c.err}]
}
