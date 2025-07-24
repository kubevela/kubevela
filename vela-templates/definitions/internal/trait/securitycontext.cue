securitycontext: {
	type: "trait"
	annotations: {}
	description: "Adds security context to the container spec in path 'spec.template.spec.containers.[].securityContext'."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
	}
}

template: {
	#PatchParams: {
		// +usage=Specify the name of the target container, if not set, use the component name
		containerName: *"" | string
		addCapabilities?: [...string]
		allowPrivilegeEscalation: *false | bool
		dropCapabilities?: [...string]
		privileged:             *false | bool
		readOnlyRootFilesystem: *false | bool
		runAsNonRoot:           *true | bool
		runAsUser?:             int
		runAsGroup?:            int
	}

	PatchContainer: {
		_params:         #PatchParams
		name:            _params.containerName
		_baseContainers: context.output.spec.template.spec.containers
		_matchContainers_: [for _container_ in _baseContainers if _container_.name == name {_container_}]
		_baseContainer: *_|_ | {...}
		if len(_matchContainers_) == 0 {
			err: "container \(name) not found"
		}
		if len(_matchContainers_) > 0 {
			securityContext: {
				capabilities: {
					if _params.addCapabilities != _|_ {
						add: _params.addCapabilities
					}
					if _params.dropCapabilities != _|_ {
						drop: _params.dropCapabilities
					}
				}
				if _params.runAsUser != _|_ {
					runAsUser: _params.runAsUser
				}
				if _params.runAsGroup != _|_ {
					runAsGroup: _params.runAsGroup
				}
				allowPrivilegeEscalation: _params.allowPrivilegeEscalation
				readOnlyRootFilesystem:   _params.readOnlyRootFilesystem
				privileged:               _params.privileged
				runAsNonRoot:             _params.runAsNonRoot
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
					allowPrivilegeEscalation: parameter.allowPrivilegeEscalation
					readOnlyRootFilesystem:   parameter.readOnlyRootFilesystem
					privileged:               parameter.privileged
					runAsNonRoot:             parameter.runAsNonRoot
					runAsUser:                parameter.runAsUser
					runAsGroup:               parameter.runAsGroup
					addCapabilities:          parameter.addCapabilities
					dropCapabilities:         parameter.dropCapabilities
				}}
			}]
		}

		if parameter.containers != _|_ {
			// +patchKey=name
			containers: [for c in parameter.containers {
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

	errs: [for c in patch.spec.template.spec.containers if c.err != _|_ {c.err}]
}
