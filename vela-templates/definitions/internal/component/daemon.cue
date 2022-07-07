import (
	"strconv"
)

daemon: {
	type: "component"
	annotations: {}
	labels: {}
	description: "Describes daemonset services in Kubernetes."
	attributes: {
		workload: {
			definition: {
				apiVersion: "apps/v1"
				kind:       "DaemonSet"
			}
			type: "daemonsets.apps"
		}
		status: {
			customStatus: #"""
				ready: {
					replicas: *0 | int
				} & {
					if context.output.status.numberReady != _|_ {
						replicas: context.output.status.numberReady
					}
				}
				desired: {
					replicas: *0 | int
				} & {
					if context.output.status.desiredNumberScheduled != _|_ {
						replicas: context.output.status.desiredNumberScheduled
					}
				}
				message: "Ready:\(ready.replicas)/\(desired.replicas)"
				"""#
			healthPolicy: #"""
				ready: {
					replicas: *0 | int
				} & {
					if context.output.status.numberReady != _|_ {
						replicas: context.output.status.numberReady
					}
				}
				desired: {
					replicas: *0 | int
				} & {
					if context.output.status.desiredNumberScheduled != _|_ {
						replicas: context.output.status.desiredNumberScheduled
					}
				}
				current: {
					replicas: *0 | int
				} & {
					if context.output.status.currentNumberScheduled != _|_ {
						replicas: context.output.status.currentNumberScheduled
					}
				}
				updated: {
					replicas: *0 | int
				} & {
					if context.output.status.updatedNumberScheduled != _|_ {
						replicas: context.output.status.updatedNumberScheduled
					}
				}
				generation: {
					metadata: context.output.metadata.generation
					observed: *0 | int
				} & {
					if context.output.status.observedGeneration != _|_ {
						observed: context.output.status.observedGeneration
					}
				}
				isHealth: (desired.replicas == ready.replicas) && (desired.replicas == updated.replicas) && (desired.replicas == current.replicas) && (generation.observed == generation.metadata || generation.observed > generation.metadata)
				"""#
		}
	}
}
template: {
	mountsArray: {
		pvc: *[
			for v in parameter.volumeMounts.pvc {
				{
					mountPath: v.mountPath
					name:      v.name
				}
			},
		] | []

		configMap: *[
				for v in parameter.volumeMounts.configMap {
				{
					mountPath: v.mountPath
					name:      v.name
				}
			},
		] | []

		secret: *[
			for v in parameter.volumeMounts.secret {
				{
					mountPath: v.mountPath
					name:      v.name
				}
			},
		] | []

		emptyDir: *[
				for v in parameter.volumeMounts.emptyDir {
				{
					mountPath: v.mountPath
					name:      v.name
				}
			},
		] | []

		hostPath: *[
				for v in parameter.volumeMounts.hostPath {
				{
					mountPath: v.mountPath
					if v.mountPropagation != _|_ {
						mountPropagation: v.mountPropagation
					}
					name: v.name
					if v.readOnly != _|_ {
						readOnly: v.readOnly
					}
				}
			},
		] | []
	}

	volumesArray: {
		pvc: *[
			for v in parameter.volumeMounts.pvc {
				{
					name: v.name
					persistentVolumeClaim: claimName: v.claimName
				}
			},
		] | []

		configMap: *[
				for v in parameter.volumeMounts.configMap {
				{
					name: v.name
					configMap: {
						defaultMode: v.defaultMode
						name:        v.cmName
						if v.items != _|_ {
							items: v.items
						}
					}
				}
			},
		] | []

		secret: *[
			for v in parameter.volumeMounts.secret {
				{
					name: v.name
					secret: {
						defaultMode: v.defaultMode
						secretName:  v.secretName
						if v.items != _|_ {
							items: v.items
						}
					}
				}
			},
		] | []

		emptyDir: *[
				for v in parameter.volumeMounts.emptyDir {
				{
					name: v.name
					emptyDir: medium: v.medium
				}
			},
		] | []

		hostPath: *[
				for v in parameter.volumeMounts.hostPath {
				{
					name: v.name
					hostPath: {
						path: v.path
					}
				}
			},
		] | []
	}

	output: {
		apiVersion: "apps/v1"
		kind:       "DaemonSet"
		spec: {
			selector: matchLabels: {
				"app.oam.dev/component": context.name
			}

			template: {
				metadata: {
					labels: {
						if parameter.labels != _|_ {
							parameter.labels
						}
						if parameter.addRevisionLabel {
							"app.oam.dev/revision": context.revision
						}
						"app.oam.dev/name":      context.appName
						"app.oam.dev/component": context.name
					}
					if parameter.annotations != _|_ {
						annotations: parameter.annotations
					}
				}

				spec: {
					containers: [{
						name:  context.name
						image: parameter.image
						if parameter["port"] != _|_ && parameter["ports"] == _|_ {
							ports: [{
								containerPort: parameter.port
							}]
						}
						if parameter["ports"] != _|_ {
							ports: [ for v in parameter.ports {
								{
									containerPort: v.port
									protocol:      v.protocol
									if v.name != _|_ {
										name: v.name
									}
									if v.name == _|_ {
										name: "port-" + strconv.FormatInt(v.port, 10)
									}
								}}]
						}

						if parameter["imagePullPolicy"] != _|_ {
							imagePullPolicy: parameter.imagePullPolicy
						}

						if parameter["cmd"] != _|_ {
							command: parameter.cmd
						}

						if parameter["env"] != _|_ {
							env: parameter.env
						}

						if context["config"] != _|_ {
							env: context.config
						}

						if parameter["cpu"] != _|_ {
							resources: {
								limits: cpu:   parameter.cpu
								requests: cpu: parameter.cpu
							}
						}

						if parameter["memory"] != _|_ {
							resources: {
								limits: memory:   parameter.memory
								requests: memory: parameter.memory
							}
						}

						if parameter["volumes"] != _|_ && parameter["volumeMounts"] == _|_ {
							volumeMounts: [ for v in parameter.volumes {
								{
									mountPath: v.mountPath
									name:      v.name
								}}]
						}

						if parameter["volumeMounts"] != _|_ {
							volumeMounts: mountsArray.pvc + mountsArray.configMap + mountsArray.secret + mountsArray.emptyDir + mountsArray.hostPath
						}

						if parameter["livenessProbe"] != _|_ {
							livenessProbe: parameter.livenessProbe
						}

						if parameter["readinessProbe"] != _|_ {
							readinessProbe: parameter.readinessProbe
						}

					}]

					if parameter["hostAliases"] != _|_ {
						// +patchKey=ip
						hostAliases: parameter.hostAliases
					}

					if parameter["imagePullSecrets"] != _|_ {
						imagePullSecrets: [ for v in parameter.imagePullSecrets {
							name: v
						},
						]
					}

					if parameter["volumes"] != _|_ && parameter["volumeMounts"] == _|_ {
						volumes: [ for v in parameter.volumes {
							{
								name: v.name
								if v.type == "pvc" {
									persistentVolumeClaim: claimName: v.claimName
								}
								if v.type == "configMap" {
									configMap: {
										defaultMode: v.defaultMode
										name:        v.cmName
										if v.items != _|_ {
											items: v.items
										}
									}
								}
								if v.type == "secret" {
									secret: {
										defaultMode: v.defaultMode
										secretName:  v.secretName
										if v.items != _|_ {
											items: v.items
										}
									}
								}
								if v.type == "emptyDir" {
									emptyDir: medium: v.medium
								}
							}
						}]
					}

					if parameter["volumeMounts"] != _|_ {
						volumes: volumesArray.pvc + volumesArray.configMap + volumesArray.secret + volumesArray.emptyDir + volumesArray.hostPath
					}
				}
			}
		}
	}

	exposePorts: [
		for v in parameter.ports if v.expose == true {
			port:       v.port
			targetPort: v.port
			if v.name != _|_ {
				name: v.name
			}
			if v.name == _|_ {
				name: "port-" + strconv.FormatInt(v.port, 10)
			}
		},
	]

	outputs: {
		if len(exposePorts) != 0 {
			webserviceExpose: {
				apiVersion: "v1"
				kind:       "Service"
				metadata: name: context.name
				spec: {
					selector: "app.oam.dev/component": context.name
					ports: exposePorts
					type:  parameter.exposeType
				}
			}
		}
	}

	parameter: {
		// +usage=Specify the labels in the workload
		labels?: [string]: string

		// +usage=Specify the annotations in the workload
		annotations?: [string]: string

		// +usage=Which image would you like to use for your service
		// +short=i
		image: string

		// +usage=Specify image pull policy for your service
		imagePullPolicy?: "Always" | "Never" | "IfNotPresent"

		// +usage=Specify image pull secrets for your service
		imagePullSecrets?: [...string]

		// +ignore
		// +usage=Deprecated field, please use ports instead
		// +short=p
		port?: int

		// +usage=Which ports do you want customer traffic sent to, defaults to 80
		ports?: [...{
			// +usage=Number of port to expose on the pod's IP address
			port: int
			// +usage=Name of the port
			name?: string
			// +usage=Protocol for port. Must be UDP, TCP, or SCTP
			protocol: *"TCP" | "UDP" | "SCTP"
			// +usage=Specify if the port should be exposed
			expose: *false | bool
		}]

		// +ignore
		// +usage=Specify what kind of Service you want. options: "ClusterIP", "NodePort", "LoadBalancer", "ExternalName"
		exposeType: *"ClusterIP" | "NodePort" | "LoadBalancer" | "ExternalName"

		// +ignore
		// +usage=If addRevisionLabel is true, the revision label will be added to the underlying pods
		addRevisionLabel: *false | bool

		// +usage=Commands to run in the container
		cmd?: [...string]

		// +usage=Define arguments by using environment variables
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

		// +usage=Number of CPU units for the service, like `0.5` (0.5 CPU core), `1` (1 CPU core)
		cpu?: string

		// +usage=Specifies the attributes of the memory resource required for the container.
		memory?: string

		volumeMounts?: {
			// +usage=Mount PVC type volume
			pvc?: [...{
				name:      string
				mountPath: string
				// +usage=The name of the PVC
				claimName: string
			}]
			// +usage=Mount ConfigMap type volume
			configMap?: [...{
				name:        string
				mountPath:   string
				defaultMode: *420 | int
				cmName:      string
				items?: [...{
					key:  string
					path: string
					mode: *511 | int
				}]
			}]
			// +usage=Mount Secret type volume
			secret?: [...{
				name:        string
				mountPath:   string
				defaultMode: *420 | int
				secretName:  string
				items?: [...{
					key:  string
					path: string
					mode: *511 | int
				}]
			}]
			// +usage=Mount EmptyDir type volume
			emptyDir?: [...{
				name:      string
				mountPath: string
				medium:    *"" | "Memory"
			}]
			// +usage=Mount HostPath type volume
			hostPath?: [...{
				name:              string
				mountPath:         string
				mountPropagation?: "None" | "HostToContainer" | "Bidirectional"
				path:              string
				readOnly?:         bool
			}]
		}

		// +usage=Deprecated field, use volumeMounts instead.
		volumes?: [...{
			name:      string
			mountPath: string
			// +usage=Specify volume type, options: "pvc","configMap","secret","emptyDir"
			type: "pvc" | "configMap" | "secret" | "emptyDir"
			if type == "pvc" {
				claimName: string
			}
			if type == "configMap" {
				defaultMode: *420 | int
				cmName:      string
				items?: [...{
					key:  string
					path: string
					mode: *511 | int
				}]
			}
			if type == "secret" {
				defaultMode: *420 | int
				secretName:  string
				items?: [...{
					key:  string
					path: string
					mode: *511 | int
				}]
			}
			if type == "emptyDir" {
				medium: *"" | "Memory"
			}
		}]

		// +usage=Instructions for assessing whether the container is alive.
		livenessProbe?: #HealthProbe

		// +usage=Instructions for assessing whether the container is in a suitable state to serve traffic.
		readinessProbe?: #HealthProbe

		// +usage=Specify the hostAliases to add
		hostAliases?: [...{
			ip: string
			hostnames: [...string]
		}]
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
			port:    int
			host?:   string
			scheme?: *"HTTP" | string
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
