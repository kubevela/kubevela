import (
	"strconv"
	"strings"
)

testcomp: {
	type: "component"
	annotations: {}
	labels: {}
	description: "Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers."
	attributes: {
		workload: {
			definition: {
				apiVersion: "apps/v1"
				kind:       "Deployment"
			}
			type: "deployments.apps"
		}
		status: {
			customStatus: #"""
				ready: {
					readyReplicas: *0 | int
				} & {
					if context.output.status.readyReplicas != _|_ {
						readyReplicas: context.output.status.readyReplicas
					}
				}
				message: "Ready:\(ready.readyReplicas)/\(context.output.spec.replicas)"
				"""#
			healthPolicy: #"""
				ready: {
					updatedReplicas:    *0 | int
					readyReplicas:      *0 | int
					replicas:           *0 | int
					observedGeneration: *0 | int
				} & {
					if context.output.status.updatedReplicas != _|_ {
						updatedReplicas: context.output.status.updatedReplicas
					}
					if context.output.status.readyReplicas != _|_ {
						readyReplicas: context.output.status.readyReplicas
					}
					if context.output.status.replicas != _|_ {
						replicas: context.output.status.replicas
					}
					if context.output.status.observedGeneration != _|_ {
						observedGeneration: context.output.status.observedGeneration
					}
				}
				_isHealth: (context.output.spec.replicas == ready.readyReplicas) && (context.output.spec.replicas == ready.updatedReplicas) && (context.output.spec.replicas == ready.replicas) && (ready.observedGeneration == context.output.metadata.generation || ready.observedGeneration > context.output.metadata.generation)
				isHealth: *_isHealth | bool
				if context.output.metadata.annotations != _|_ {
					if context.output.metadata.annotations["app.oam.dev/disable-health-check"] != _|_ {
						isHealth: true
					}
				}
				"""#
		}
	}
}
template: {


	output: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
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
							ports: [for v in parameter.ports {
								{
									containerPort: {
										if v.containerPort != _|_ {v.containerPort}
										if v.containerPort == _|_ {v.port}
									}
									protocol: v.protocol
									if v.name != _|_ {
										name: v.name
									}
									if v.name == _|_ {
										_name: {
											if v.containerPort != _|_ {"port-" + strconv.FormatInt(v.containerPort, 10)}
											if v.containerPort == _|_ {"port-" + strconv.FormatInt(v.port, 10)}
										}
										name: *_name | string
										if v.protocol != "TCP" {
											name: _name + "-" + strings.ToLower(v.protocol)
										}
									}
								}}]
						}

						if parameter["imagePullPolicy"] != _|_ {
							imagePullPolicy: parameter.imagePullPolicy
						}

						if parameter["cmd"] != _|_ {
							command: parameter.cmd
						}

						if parameter["args"] != _|_ {
							args: parameter.args
						}

						if parameter["env"] != _|_ {
							env: parameter.env
						}

						if context["config"] != _|_ {
							env: context.config
						}

						if parameter["cpu"] != _|_ {
							if (parameter.limit.cpu != _|_) {
								resources: {
									requests: cpu: parameter.cpu
									limits: cpu:   parameter.limit.cpu
								}
							}
							if (parameter.limit.cpu == _|_) {
								resources: {
									limits: cpu:   parameter.cpu
									requests: cpu: parameter.cpu
								}
							}
						}

						if parameter["memory"] != _|_ {
							if (parameter.limit.memory != _|_) {
								resources: {
									limits: memory:   parameter.limit.memory
									requests: memory: parameter.memory
								}
							}
							if (parameter.limit.memory == _|_) {
								resources: {
									limits: memory:   parameter.memory
									requests: memory: parameter.memory
								}
							}
						}

						if parameter["volumes"] != _|_ && parameter["volumeMounts"] == _|_ {
							volumeMounts: [for v in parameter.volumes {
								{
									mountPath: v.mountPath
									name:      v.name
								}}]
						}

						if parameter["volumeMounts"] != _|_ {
							volumeMounts: mountsArray
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
						imagePullSecrets: [for v in parameter.imagePullSecrets {
							name: v
						},
						]
					}

					if parameter["volumes"] != _|_ && parameter["volumeMounts"] == _|_ {
						volumes: [for v in parameter.volumes {
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
						volumes: deDupVolumesArray
					}
				}
			}
		}
	}



	outputs: {
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

	outputs: {
			test: {
				apiVersion: "v1"
				kind:       "Pod1"
				metadata: name: context.name
				spec: {
					selector: "app.oam.dev/component": context.name
					ports: exposePorts
					type:  parameter.exposeType
				}
			}
	}


}