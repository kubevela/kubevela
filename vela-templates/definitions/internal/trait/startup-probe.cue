"startup-probe": {
	type: "trait"
	annotations: {}
	description: "Add startup probe hooks for the specified container of K8s pod for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
	}
}
template: {
	#StartupProbeParams: {
		// +usage=Specify the name of the target container, if not set, use the component name
		containerName: *"" | string
		// +usage=Number of seconds after the container has started before liveness probes are initiated. Minimum value is 0.
		initialDelaySeconds: *0 | int
		// +usage=How often, in seconds, to execute the probe. Minimum value is 1.
		periodSeconds: *10 | int
		// +usage=Number of seconds after which the probe times out. Minimum value is 1.
		timeoutSeconds: *1 | int
		// +usage=Minimum consecutive successes for the probe to be considered successful after having failed.  Minimum value is 1.
		successThreshold: *1 | int
		// +usage=Minimum consecutive failures for the probe to be considered failed after having succeeded. Minimum value is 1.
		failureThreshold: *3 | int
		// +usage=Optional duration in seconds the pod needs to terminate gracefully upon probe failure. Set this value longer than the expected cleanup time for your process.
		terminationGracePeriodSeconds?: int
		// +usage=Instructions for assessing container startup status by executing a command. Either this attribute or the httpGet attribute or the grpc attribute or the tcpSocket attribute MUST be specified. This attribute is mutually exclusive with the httpGet attribute and the tcpSocket attribute and the gRPC attribute.
		exec?: {
			// +usage=A command to be executed inside the container to assess its health. Each space delimited token of the command is a separate array element. Commands exiting 0 are considered to be successful probes, whilst all other exit codes are considered failures.
			command: [...string]
		}
		// +usage=Instructions for assessing container startup status by executing an HTTP GET request. Either this attribute or the exec attribute or the grpc attribute or the tcpSocket attribute MUST be specified. This attribute is mutually exclusive with the exec attribute and the tcpSocket attribute and the gRPC attribute.
		httpGet?: {
			// +usage=The endpoint, relative to the port, to which the HTTP GET request should be directed.
			path?: string
			// +usage=The port numer to access on the host or container.
			port: int
			// +usage=The hostname to connect to, defaults to the pod IP. You probably want to set "Host" in httpHeaders instead.
			host?: string
			// +usage=The Scheme to use for connecting to the host.
			scheme?: *"HTTP" | "HTTPS"
			// +usage=Custom headers to set in the request. HTTP allows repeated headers.
			httpHeaders?: [...{
				// +usage=The header field name
				name: string
				//+usage=The header field value
				value: string
			}]
		}
		// +usage=Instructions for assessing container startup status by probing a gRPC service. Either this attribute or the exec attribute or the grpc attribute or the httpGet attribute MUST be specified. This attribute is mutually exclusive with the exec attribute and the httpGet attribute and the tcpSocket attribute.
		grpc?: {
			// +usage=The port number of the gRPC service.
			port: int
			// +usage=The name of the service to place in the gRPC HealthCheckRequest
			service?: string
		}
		// +usage=Instructions for assessing container startup status by probing a TCP socket. Either this attribute or the exec attribute or the tcpSocket attribute or the httpGet attribute MUST be specified. This attribute is mutually exclusive with the exec attribute and the httpGet attribute and the gRPC attribute.
		tcpSocket?: {
			// +usage=Number or name of the port to access on the container.
			port: string
			// +usage=Host name to connect to, defaults to the pod IP.
			host?: string
		}
	}
	PatchContainer: {
		_params:         #StartupProbeParams
		name:            _params.containerName
		_baseContainers: context.output.spec.template.spec.containers
		_matchContainers_: [ for _container_ in _baseContainers if _container_.name == name {_container_}]
		if len(_matchContainers_) == 0 {
			err: "container \(name) not found"
		}
		if len(_matchContainers_) > 0 {
			startupProbe: {
				if _params.exec != _|_ {
					exec: _params.exec
				}
				if _params.httpGet != _|_ {
					httpGet: _params.httpGet
				}
				if _params.grpc != _|_ {
					grpc: _params.grpc
				}
				if _params.tcpSocket != _|_ {
					tcpSocket: _params.tcpSocket
				}
				if _params.initialDelaySeconds != _|_ {
					initialDelaySeconds: _params.initialDelaySeconds
				}
				if _params.periodSeconds != _|_ {
					periodSeconds: _params.periodSeconds
				}
				if _params.tcpSocket != _|_ {
					tcpSocket: _params.tcpSocket
				}
				if _params.timeoutSeconds != _|_ {
					timeoutSeconds: _params.timeoutSeconds
				}
				if _params.successThreshold != _|_ {
					successThreshold: _params.successThreshold
				}
				if _params.failureThreshold != _|_ {
					failureThreshold: _params.failureThreshold
				}
				if _params.terminationGracePeriodSeconds != _|_ {
					terminationGracePeriodSeconds: _params.terminationGracePeriodSeconds
				}
			}
		}
	}

	patch: spec: template: spec: {
		if parameter.probes == _|_ {
			// +patchKey=name
			containers: [{
				PatchContainer & {_params: {
					if parameter.containerName == "" {
						containerName: context.name
					}
					if parameter.containerName != "" {
						containerName: parameter.containerName
					}
					periodSeconds:                 parameter.periodSeconds
					initialDelaySeconds:           parameter.initialDelaySeconds
					timeoutSeconds:                parameter.timeoutSeconds
					successThreshold:              parameter.successThreshold
					failureThreshold:              parameter.failureThreshold
					terminationGracePeriodSeconds: parameter.terminationGracePeriodSeconds
					if parameter.exec != _|_ {
						exec: parameter.exec
					}
					if parameter.httpGet != _|_ {
						httpGet: parameter.httpGet
					}
					if parameter.grpc != _|_ {
						grpc: parameter.grpc
					}
					if parameter.tcpSocket != _|_ {
						tcpSocket: parameter.grtcpSocketpc
					}
				}}
			}]
		}
		if parameter.probes != _|_ {
			// +patchKey=name
			containers: [ for c in parameter.probes {
				if c.name == "" {
					err: "containerName must be set when specifying startup probe for multiple containers"
				}
				if c.name != "" {
					PatchContainer & {_params: c}
				}
			}]
		}
	}

	parameter: *#StartupProbeParams | close({
		// +usage=Specify the startup probe for multiple containers
		probes: [...#StartupProbeParams]
	})

	errs: [ for c in patch.spec.template.spec.containers if c.err != _|_ {c.err}]

}
