import "vela/test"

_web: {
	definition: "webservice"
	context: {name: "web", appName: "shop"}
}

"renders a Deployment selecting its component": test.#ComponentRender & _web & {
	parameter: image: "shop:1.0"
	expect: {
		output: {
			apiVersion: "apps/v1"
			kind:       "Deployment"
			spec: {
				selector: matchLabels: "app.oam.dev/component": "web"
				template: {
					metadata: {
						labels: {"app.oam.dev/name": "shop", "app.oam.dev/component": "web"} @exact()
					}
					spec: containers: [{name: "web", image: "shop:1.0", ports?: _|_, resources?: _|_}]
				}
			}
		}
		outputs: {} @exact()
	}
}

"an image is required": test.#ComponentRender & _web & {
	expect: error: parameter: [=~"image"]
}

"port is the container's only port": test.#ComponentRender & _web & {
	parameter: {image: "shop:1.0", port: 8080}
	expect: {
		output: spec: template: spec: containers: [{ports: [{containerPort: 8080}]}]
		outputs: {} @exact()
	}
}

"ports are named, and exposed ones become a Service": test.#ComponentRender & _web & {
	parameter: {
		image: "shop:1.0"
		ports: [
			{port: 80, containerPort: 8080, expose: true},
			{port: 53, protocol: "UDP", expose: true},
			{port: 9090, name: "metrics"},
		]
	}
	expect: {
		output: spec: template: spec: containers: [{ports: [
			{containerPort: 8080, protocol: "TCP", name: "port-8080"},
			{containerPort: 53, protocol: "UDP", name: "port-53-udp"},
			{containerPort: 9090, name: "metrics"},
		]}]
		outputs: webserviceExpose: {
			apiVersion: "v1"
			kind:       "Service"
			metadata: name: "web"
			spec: {
				selector: "app.oam.dev/component": "web"
				type: "ClusterIP"
				ports: [
					{port: 80, targetPort: 8080, name: "port-8080", protocol: "TCP"},
					{port: 53, targetPort: 53, name: "port-53-udp", protocol: "UDP"},
				]
			}
		}
	}
}

"a nodePort is kept only for a NodePort Service": test.#ComponentRender & _web & {
	parameter: {
		image:      "shop:1.0"
		exposeType: "NodePort"
		ports: [{port: 80, nodePort: 30080, expose: true}]
	}
	expect: outputs: webserviceExpose: spec: {type: "NodePort", ports: [{nodePort: 30080}]}
}

"a nodePort is dropped for a ClusterIP Service": test.#ComponentRender & _web & {
	parameter: {image: "shop:1.0", ports: [{port: 80, nodePort: 30080, expose: true}]}
	expect: outputs: webserviceExpose: spec: ports: [{nodePort?: _|_}]
}

"command, arguments, environment and pull policy": test.#ComponentRender & _web & {
	parameter: {
		image:           "shop:1.0"
		imagePullPolicy: "Always"
		imagePullSecrets: ["registry"]
		cmd: ["/shop"]
		args: ["--verbose"]
		env: [{name: "MODE", value: "prod"}]
	}
	expect: output: spec: template: spec: {
		imagePullSecrets: [{name: "registry"}]
		containers: [{
			imagePullPolicy: "Always"
			command: ["/shop"]
			args: ["--verbose"]
			env: [{name: "MODE", value: "prod"}]
		}]
	}
}

"cpu and memory are both request and limit": test.#ComponentRender & _web & {
	parameter: {image: "shop:1.0", cpu: "500m", memory: "256Mi"}
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: "500m", memory: "256Mi"}
		limits: {cpu: "500m", memory: "256Mi"}
	}}]
}

"a separate limit raises only the limit": test.#ComponentRender & _web & {
	parameter: {image: "shop:1.0", cpu: "500m", memory: "256Mi", limit: {cpu: "1", memory: "512Mi"}}
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: "500m", memory: "256Mi"}
		limits: {cpu: "1", memory: "512Mi"}
	}}]
}

"volumeMounts mount each kind, a volume shared by name once": test.#ComponentRender & _web & {
	parameter: {
		image: "shop:1.0"
		volumeMounts: {
			pvc: [{name: "data", mountPath: "/data", claimName: "shop-data"}]
			configMap: [{name: "conf", mountPath: "/etc/shop", cmName: "shop-conf"}]
			secret: [{name: "creds", mountPath: "/etc/creds", secretName: "shop-creds"}]
			emptyDir: [
				{name: "cache", mountPath: "/cache"},
				{name: "cache", mountPath: "/tmp/cache", subPath: "tmp"},
			]
			hostPath: [{name: "logs", mountPath: "/logs", path: "/var/log/shop"}]
		}
	}
	expect: output: spec: template: spec: {
		containers: [{volumeMounts: [
			{name: "data", mountPath: "/data"},
			{name: "conf", mountPath: "/etc/shop"},
			{name: "creds", mountPath: "/etc/creds"},
			{name: "cache", mountPath: "/cache"},
			{name: "cache", mountPath: "/tmp/cache", subPath: "tmp"},
			{name: "logs", mountPath: "/logs"},
		]}]
		volumes: [
			{name: "data", persistentVolumeClaim: claimName: "shop-data"},
			{name: "conf", configMap: name: "shop-conf"},
			{name: "creds", secret: secretName: "shop-creds"},
			{name: "cache", emptyDir: _},
			{name: "logs", hostPath: path: "/var/log/shop"},
		]
	}
}

"the deprecated volumes still mount": test.#ComponentRender & _web & {
	parameter: {
		image: "shop:1.0"
		volumes: [{name: "conf", mountPath: "/etc/shop", type: "configMap", cmName: "shop-conf"}]
	}
	expect: output: spec: template: spec: {
		containers: [{volumeMounts: [{name: "conf", mountPath: "/etc/shop"}]}]
		volumes: [{name: "conf", configMap: name: "shop-conf"}]
	}
}

"probes and host aliases pass through": test.#ComponentRender & _web & {
	parameter: {
		image: "shop:1.0"
		livenessProbe: httpGet: {path: "/healthz", port: 8080}
		readinessProbe: exec: command: ["cat", "/ready"]
		hostAliases: [{ip: "10.0.0.1", hostnames: ["db.local"]}]
	}
	expect: output: spec: template: spec: {
		hostAliases: [{ip: "10.0.0.1", hostnames: ["db.local"]}]
		containers: [{
			livenessProbe: httpGet: {path: "/healthz", port: 8080}
			readinessProbe: exec: command: ["cat", "/ready"]
		}]
	}
}

"pod labels and annotations, and the revision label when asked": test.#ComponentRender & _web & {
	context: revision: "web-v4"
	parameter: {
		image: "shop:1.0"
		labels: team:       "payments"
		annotations: owner: "payments"
		addRevisionLabel: true
	}
	expect: output: spec: template: metadata: {
		labels: {team: "payments", "app.oam.dev/revision": "web-v4", "app.oam.dev/component": "web"}
		annotations: owner: "payments"
	}
}

"healthy once every replica is updated and ready": test.#ComponentStatus & _web & {
	parameter: image: "shop:1.0"
	observed: output: {
		spec: replicas: 2
		status: {replicas: 2, updatedReplicas: 2, readyReplicas: 2, observedGeneration: 1}
	}
	expect: {healthy: true, message: "Ready:2/2"}
}

"not healthy mid-rollout": test.#ComponentStatus & _web & {
	parameter: image: "shop:1.0"
	observed: output: {
		spec: replicas: 2
		status: {replicas: 2, updatedReplicas: 1, readyReplicas: 1, observedGeneration: 1}
	}
	expect: {healthy: false, message: "Ready:1/2"}
}

"not healthy before the controller has seen the change": test.#ComponentStatus & _web & {
	parameter: image: "shop:1.0"
	observed: output: {
		metadata: generation: 2
		spec: replicas:       1
		status: {replicas: 1, updatedReplicas: 1, readyReplicas: 1, observedGeneration: 1}
	}
	expect: healthy: false
}

"the disable-health-check annotation makes it healthy": test.#ComponentStatus & _web & {
	parameter: image: "shop:1.0"
	observed: output: {
		metadata: annotations: "app.oam.dev/disable-health-check": "true"
		spec: replicas: 2
	}
	expect: {healthy: true, message: "Ready:0/2"}
}
