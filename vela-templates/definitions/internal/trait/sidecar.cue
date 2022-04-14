sidecar: {
	type: "trait"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Inject a sidecar container to K8s pod for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["*"]
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
			if parameter.args != _|_ {
				args: parameter.args
			}
			if parameter["volumes"] != _|_ {
				volumeMounts: [ for v in parameter.volumes {
					{
						mountPath: v.path
						name:      v.name
					}
				}]
			}
			if parameter["livenessProbe"] != _|_ {
				livenessProbe: parameter.livenessProbe
			}

			if parameter["readinessProbe"] != _|_ {
				readinessProbe: parameter.readinessProbe
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

		// +usage=Specify the args in the sidecar
		args?: [...string]

		// +usage=Specify the shared volume path
		volumes?: [...{
			name: string
			path: string
		}]

		// +usage=Instructions for assessing whether the container is alive.
		livenessProbe?: #HealthProbe

		// +usage=Instructions for assessing whether the container is in a suitable state to serve traffic.
		readinessProbe?: #HealthProbe
	}

	#HealthProbe: {

		// +usage=Instructions for assessing container health by executing a command. Either this attribute or the httpGet attribute or the tcpSocket attribute MUST be specified. This attribute is mutually exclusive with both the httpGet attribute and the tcpSocket attribute.
		exec?: {
			// +usage=A command to be executed inside the container to assess its health. Each space delimited token of the command is a separate array element. Commands exiting 0 are considered to be successful probes, whilst all other exit codes are considered failures.
			command: [...string]
		}

		// +usage=Instructions for assessing container health by executing an HTTP GET request. Either this attribute or the exec attribute or the tcpSocket attribute MUST be specified. This attribute is mutually exclusive with both the exec attribute and the tcpSocket attribute.
		httpGet?: {
			// +usage=The endpoint, relative to the port, to which the HTTP GET request should be directed.
			path: string
			// +usage=The TCP socket within the container to which the HTTP GET request should be directed.
			port: int
			httpHeaders?: [...{
				name:  string
				value: string
			}]
		}

		// +usage=Instructions for assessing container health by probing a TCP socket. Either this attribute or the exec attribute or the httpGet attribute MUST be specified. This attribute is mutually exclusive with both the exec attribute and the httpGet attribute.
		tcpSocket?: {
			// +usage=The TCP socket within the container that should be probed to assess container health.
			port: int
		}

		// +usage=Number of seconds after the container is started before the first probe is initiated.
		initialDelaySeconds: *0 | int

		// +usage=How often, in seconds, to execute the probe.
		periodSeconds: *10 | int

		// +usage=Number of seconds after which the probe times out.
		timeoutSeconds: *1 | int

		// +usage=Minimum consecutive successes for the probe to be considered successful after having failed.
		successThreshold: *1 | int

		// +usage=Number of consecutive failures required to determine the container is not alive (liveness probe) or not ready (readiness probe).
		failureThreshold: *3 | int
	}
}
