import (
	"strconv"
)

statefulservice: {
	type: "component"
	annotations: {}
	labels: {}
	description: "A long-running, scalable application with guarantees about the ordering and uniqueness of the replicas."
	attributes: {
		workload: {
			definition: {
				apiVersion: "apps/v1"
				kind:       "StatefulSet"
			}
			type: "statefulsets.apps"
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
				isHealth: (context.output.spec.replicas == ready.readyReplicas) && (context.output.spec.replicas == ready.updatedReplicas) && (context.output.spec.replicas == ready.replicas) && (ready.observedGeneration == context.output.metadata.generation || ready.observedGeneration > context.output.metadata.generation)
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
					name:      v.name
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
					size: v.size
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
					hostPath: path: v.path
				}
			},
		] | []
	}
	output: {
		apiVersion: "apps/v1"
		kind:       "StatefulSet"
		metadata: {
			name: context.name
			labels: {
				app:                     context.appName
				component:               context.name
				"app.oam.dev/component": context.name
				"app.oam.dev/name":      context.appName

				if parameter.labels != _|_ {
					parameter.labels
				}
				if parameter.addRevisionLabel {
					"app.oam.dev/revision": context.revision
				}
			}
			if parameter.annotations != _|_ {
				annotations: parameter.annotations
			}
		}
		spec: {
			selector: matchLabels: app: context.name
			serviceName: context.name
			replicas:    parameter.replicas
			template: {
				metadata: {
					labels: {
						app:                     context.name
						"app.oam.dev/name":      context.appName
						"app.oam.dev/component": context.name
					}
				}
				spec: {
					containers: [{
						name:  context.name
						image: parameter.image

						if parameter["imagePullPolicy"] != _|_ {
							imagePullPolicy: parameter.imagePullPolicy
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

						if parameter["env"] != _|_ {
							env: parameter.env
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
						if parameter["volumeMounts"] != _|_ {
							volumeMounts: mountsArray.pvc + mountsArray.configMap + mountsArray.secret + mountsArray.emptyDir + mountsArray.hostPath
						}
					}]

					if parameter["imagePullSecrets"] != _|_ {
						imagePullSecrets: [ for v in parameter.imagePullSecrets {
							name: v
						},
						]
					}

					if parameter["volumeMounts"] != _|_ {
						volumes: volumesArray.configMap + volumesArray.secret + volumesArray.emptyDir + volumesArray.hostPath
					}
				}
			}
			if len(volumesArray.pvc) != 0 {
				volumeClaimTemplates: [ for v in volumesArray.pvc {
					{
						metadata: name: v.name
						spec: {
							accessModes: [ "ReadWriteOnce"]
							resources: requests: storage: v.size
						}
					}}]
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
			headless: {
				apiVersion: "v1"
				kind:       "Service"
				metadata: name: context.name + "-headless"
				spec: {
					selector: app: context.name
					ports:     exposePorts
					clusterIP: "None"
				}
			}
			service: {
				apiVersion: "v1"
				kind:       "Service"
				metadata: name: context.name
				spec: {
					selector: app: context.name
					ports: exposePorts
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

		// +usage=Define the required replicas
		replicas: int

		// +usage=Which ports do you want customer traffic sent to,
		ports?: [...{
			port:     int
			name?:    string
			protocol: *"TCP" | "UDP" | "SCTP"
		}]

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

		// +ignore
		// +usage=If addRevisionLabel is true, the revision label will be added to the underlying pods
		addRevisionLabel: *false | bool

		volumeMounts?: {
			// +usage=Mount PVC type volume
			pvc?: [...{
				name:      string
				mountPath: string
				// +usage=The name of the PVC
				claimName?: string
				size:       string
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
				name:      string
				mountPath: string
				path:      string
			}]
		}

		// +usage=Number of CPU units for the service, like `0.5` (0.5 CPU core), `1` (1 CPU core)
		cpu?: string

		// +usage=Specifies the attributes of the memory resource required for the container.
		memory?: string

		// +usage=Specify image pull policy for your service
		imagePullPolicy?: "Always" | "Never" | "IfNotPresent"

		// +usage=Specify image pull secrets for your service
		imagePullSecrets?: [...string]
	}
}
