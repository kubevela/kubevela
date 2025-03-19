import (
	"strconv"
	"strings"
)

"container-ports": {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Expose on the host and bind the external port to host to enable web traffic for your component."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
	}
}

template: {
	#PatchParams: {
		// +usage=Specify the name of the target container, if not set, use the component name
		containerName: *"" | string
		// +usage=Specify ports you want customer traffic sent to
		ports: *[] | [...{
			// +usage=Number of port to expose on the pod's IP address
			containerPort: int
			// +usage=Protocol for port. Must be UDP, TCP, or SCTP
			protocol: *"TCP" | "UDP" | "SCTP"
			// +usage=Number of port to expose on the host
			hostPort?: int
			// +usage=What host IP to bind the external port to.
			hostIP?: string
		}]
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
			_baseContainer: _matchContainers_[0]
			_basePorts:     _baseContainer.ports
			if _basePorts == _|_ {
				// +patchStrategy=replace
				ports: [ for port in _params.ports {
					containerPort: port.containerPort
					protocol:      port.protocol
					if port.hostPort != _|_ {
						hostPort: port.hostPort
					}
					if port.hostIP != _|_ {
						hostIP: port.hostIP
					}
				}]
			}
			if _basePorts != _|_ {
				_basePortsMap: {for _basePort in _basePorts {(strings.ToLower(_basePort.protocol) + strconv.FormatInt(_basePort.containerPort, 10)): _basePort}}
				_portsMap: {for port in _params.ports {(strings.ToLower(port.protocol) + strconv.FormatInt(port.containerPort, 10)): port}}
				// +patchStrategy=replace
				ports: [ for portVar in _basePorts {
					containerPort: portVar.containerPort
					protocol:      portVar.protocol
					name:          portVar.name
					_uniqueKey:    strings.ToLower(portVar.protocol) + strconv.FormatInt(portVar.containerPort, 10)
					if _portsMap[_uniqueKey] != _|_ {
						if _portsMap[_uniqueKey].hostPort != _|_ {
							hostPort: _portsMap[_uniqueKey].hostPort
						}
						if _portsMap[_uniqueKey].hostIP != _|_ {
							hostIP: _portsMap[_uniqueKey].hostIP
						}
					}
				}] + [ for port in _params.ports if _basePortsMap[strings.ToLower(port.protocol)+strconv.FormatInt(port.containerPort, 10)] == _|_ {
					if port.containerPort != _|_ {
						containerPort: port.containerPort
					}
					if port.protocol != _|_ {
						protocol: port.protocol
					}
					if port.hostPort != _|_ {
						hostPort: port.hostPort
					}
					if port.hostIP != _|_ {
						hostIP: port.hostIP
					}
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
					ports: parameter.ports
				}}
			}]
		}
		if parameter.containers != _|_ {
			// +patchKey=name
			containers: [ for c in parameter.containers {
				if c.containerName == "" {
					err: "container name must be set for containers"
				}
				if c.containerName != "" {
					PatchContainer & {_params: c}
				}
			}]
		}
	}

	parameter: *#PatchParams | close({
		// +usage=Specify the container ports for multiple containers
		containers: [...#PatchParams]
	})

	errs: [ for c in patch.spec.template.spec.containers if c.err != _|_ {c.err}]
}
