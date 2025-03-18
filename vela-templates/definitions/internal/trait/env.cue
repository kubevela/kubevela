env: {
	type: "trait"
	annotations: {}
	description: "Add env on K8s pod for your workload which follows the pod spec in path 'spec.template'"
	attributes: {
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
	}
}
template: {
	#PatchParams: {
		// +usage=Specify the name of the target container, if not set, use the component name
		containerName: *"" | string
		// +usage=Specify if replacing the whole environment settings for the container
		replace: *false | bool
		// +usage=Specify the  environment variables to merge, if key already existing, override its value
		env: [string]: string
		// +usage=Specify which existing environment variables to unset
		unset: *[] | [...string]
	}
	PatchContainer: {
		_params: #PatchParams
		name:    _params.containerName
		_delKeys: {for k in _params.unset {(k): ""}}
		_baseContainers: context.output.spec.template.spec.containers
		_matchContainers_: [ for _container_ in _baseContainers if _container_.name == name {_container_}]
		_baseContainer: *_|_ | {...}
		if len(_matchContainers_) == 0 {
			err: "container \(name) not found"
		}
		if len(_matchContainers_) > 0 {
			_baseContainer: _matchContainers_[0]
			_baseEnv:       _baseContainer.env
			if _baseEnv == _|_ {
				// +patchStrategy=replace
				env: [ for k, v in _params.env if _delKeys[k] == _|_ {
					name:  k
					value: v
				}]
			}
			if _baseEnv != _|_ {
				_baseEnvMap: {for envVar in _baseEnv {(envVar.name): envVar}}
				// +patchStrategy=replace
				env: [ for envVar in _baseEnv if _delKeys[envVar.name] == _|_ && !_params.replace {
					name: envVar.name
					if _params.env[envVar.name] != _|_ {
						value: _params.env[envVar.name]
					}
					if _params.env[envVar.name] == _|_ {
						if envVar.value != _|_ {
							value: envVar.value
						}
						if envVar.valueFrom != _|_ {
							valueFrom: envVar.valueFrom
						}
					}
				}] + [ for k, v in _params.env if _delKeys[k] == _|_ && (_params.replace || _baseEnvMap[k] == _|_) {
					name:  k
					value: v
				}]
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
					replace: parameter.replace
					env:     parameter.env
					unset:   parameter.unset
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

	parameter: *#PatchParams | close({
		// +usage=Specify the environment variables for multiple containers
		containers: [...#PatchParams]
	})

	errs: [ for c in patch.spec.template.spec.containers if c.err != _|_ {c.err}]
}
