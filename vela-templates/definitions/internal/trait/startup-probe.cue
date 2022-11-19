"startup-probe": {
	type: "trait"
	annotations: {}
	description: "Add startup probe hooks for every container of K8s pod for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
	}
}
template: {
	patch: spec: template: spec: containers: [...{
		startupProbe: {
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
				tcpSocket: parameter.tcpSocket
			}
			if parameter.initialDelaySeconds != _|_ {
				initialDelaySeconds: parameter.initialDelaySeconds
			}
			if parameter.periodSeconds != _|_ {
				periodSeconds: parameter.periodSeconds
			}
			if parameter.tcpSocket != _|_ {
				tcpSocket: parameter.tcpSocket
			}
			if parameter.timeoutSeconds != _|_ {
				timeoutSeconds: parameter.timeoutSeconds
			}
			if parameter.successThreshold != _|_ {
				successThreshold: parameter.successThreshold
			}
			if parameter.failureThreshold != _|_ {
				failureThreshold: parameter.failureThreshold
			}
			if parameter.terminationGracePeriodSeconds != _|_ {
				terminationGracePeriodSeconds: parameter.terminationGracePeriodSeconds
			}
		}
	}]

	parameter: {
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

		// +usage=Number of seconds after the container has started before liveness probes are initiated.
		initialDelaySeconds: *0 | int

		// +usage=How often, in seconds, to execute the probe.
		periodSeconds: *10 | int

		// +usage=Number of seconds after which the probe times out.
		timeoutSeconds: *1 | int

		// +usage=Minimum consecutive successes for the probe to be considered successful after having failed.
		successThreshold: *1 | int

		// +usage=Minimum consecutive failures for the probe to be considered failed after having succeeded. Defaults to 3. Minimum value is 1.
		failureThreshold: *3 | int

		// +usage=Optional duration in seconds the pod needs to terminate gracefully upon probe failure. Set this value longer than the expected cleanup time for your process. 
		terminationGracePeriodSeconds?: int
	}
}
