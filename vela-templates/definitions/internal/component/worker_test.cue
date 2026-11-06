import "vela/test"

_worker: {
	definition: "worker"
	context: {name: "queue", appName: "shop"}
}

"renders a Deployment selecting its component, with no Service": test.#ComponentRender & _worker & {
	parameter: image: "shop-worker:1.0"
	expect: {
		output: {
			apiVersion: "apps/v1"
			kind:       "Deployment"
			spec: {
				selector: matchLabels: "app.oam.dev/component": "queue"
				template: {
					metadata: {
						labels: {"app.oam.dev/name": "shop", "app.oam.dev/component": "queue"} @exact()
					}
					spec: {
						containers: [{name: "queue", image: "shop-worker:1.0", ports?: _|_, resources?: _|_, volumeMounts?: _|_}]
						volumes?:          _|_
						imagePullSecrets?: _|_
					}
				}
			}
		}
		outputs: {} @exact()
	}
}

"an image is required": test.#ComponentRender & _worker & {
	expect: error: parameter: [=~"image"]
}

"command, environment and pull policy": test.#ComponentRender & _worker & {
	parameter: {
		image:           "shop-worker:1.0"
		imagePullPolicy: "Always"
		imagePullSecrets: ["registry"]
		cmd: ["/worker", "--queue=orders"]
		env: [
			{name: "MODE", value: "prod"},
			{name: "TOKEN", valueFrom: secretKeyRef: {name: "shop-creds", key: "token"}},
		]
	}
	expect: output: spec: template: spec: {
		imagePullSecrets: [{name: "registry"}]
		containers: [{
			imagePullPolicy: "Always"
			command: ["/worker", "--queue=orders"]
			env: [
				{name: "MODE", value: "prod"},
				{name: "TOKEN", valueFrom: secretKeyRef: {name: "shop-creds", key: "token"}},
			]
		}]
	}
}

"cpu is both request and limit": test.#ComponentRender & _worker & {
	parameter: {image: "shop-worker:1.0", cpu: "500m"}
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: "500m"} @exact()
		limits: {cpu: "500m"} @exact()
	}}]
}

"memory is both request and limit, beside cpu": test.#ComponentRender & _worker & {
	parameter: {image: "shop-worker:1.0", cpu: "500m", memory: "256Mi"}
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: "500m", memory: "256Mi"}
		limits: {cpu: "500m", memory: "256Mi"}
	}}]
}

"volumeMounts mount each kind, a volume shared by name once": test.#ComponentRender & _worker & {
	parameter: {
		image: "shop-worker:1.0"
		volumeMounts: {
			pvc: [{name: "data", mountPath: "/data", claimName: "shop-data"}]
			configMap: [{name: "conf", mountPath: "/etc/shop", cmName: "shop-conf"}]
			secret: [{name: "creds", mountPath: "/etc/creds", secretName: "shop-creds"}]
			emptyDir: [
				{name: "cache", mountPath: "/cache", medium: "Memory"},
				{name: "cache", mountPath: "/tmp/cache"},
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
			{name: "cache", mountPath: "/tmp/cache"},
			{name: "logs", mountPath: "/logs"},
		]}]
		volumes: [
			{name: "data", persistentVolumeClaim: claimName: "shop-data"},
			{name: "conf", configMap: {name: "shop-conf", defaultMode: 420}},
			{name: "creds", secret: {secretName: "shop-creds", defaultMode: 420}},
			{name: "cache", emptyDir: medium: "Memory"},
			{name: "logs", hostPath: path: "/var/log/shop"},
		]
	}
}

"the deprecated volumes mount, defaulting to an emptyDir": test.#ComponentRender & _worker & {
	parameter: {
		image: "shop-worker:1.0"
		volumes: [
			{name: "conf", mountPath: "/etc/shop", type: "configMap", cmName: "shop-conf"},
			{name: "data", mountPath: "/data", type: "pvc", claimName: "shop-data"},
			{name: "scratch", mountPath: "/scratch"},
		]
	}
	expect: output: spec: template: spec: {
		containers: [{volumeMounts: [
			{name: "conf", mountPath: "/etc/shop"},
			{name: "data", mountPath: "/data"},
			{name: "scratch", mountPath: "/scratch"},
		]}]
		volumes: [
			{name: "conf", configMap: name: "shop-conf"},
			{name: "data", persistentVolumeClaim: claimName: "shop-data"},
			{name: "scratch", emptyDir: medium: ""},
		]
	}
}

"volumeMounts win over the deprecated volumes": test.#ComponentRender & _worker & {
	parameter: {
		image: "shop-worker:1.0"
		volumes: [{name: "old", mountPath: "/old"}]
		volumeMounts: emptyDir: [{name: "new", mountPath: "/new"}]
	}
	expect: output: spec: template: spec: {
		containers: [{volumeMounts: [{name: "new", mountPath: "/new"}]}]
		volumes: [{name: "new"}]
	}
}

"probes pass through with their defaults": test.#ComponentRender & _worker & {
	parameter: {
		image: "shop-worker:1.0"
		livenessProbe: exec: command: ["cat", "/alive"]
		readinessProbe: {tcpSocket: port: 9090, periodSeconds: 5}
	}
	expect: output: spec: template: spec: containers: [{
		livenessProbe: {
			exec: command: ["cat", "/alive"]
			initialDelaySeconds: 0
			periodSeconds:       10
			timeoutSeconds:      1
			successThreshold:    1
			failureThreshold:    3
		}
		readinessProbe: {tcpSocket: port: 9090, periodSeconds: 5}
	}]
}

"healthy once every replica is updated and ready": test.#ComponentStatus & _worker & {
	parameter: image: "shop-worker:1.0"
	observed: output: {
		spec: replicas: 3
		status: {replicas: 3, updatedReplicas: 3, readyReplicas: 3, observedGeneration: 1}
	}
	expect: {healthy: true, message: "Ready:3/3"}
}

"not healthy while replicas are not ready": test.#ComponentStatus & _worker & {
	parameter: image: "shop-worker:1.0"
	observed: output: {
		spec: replicas: 3
		status: {replicas: 3, updatedReplicas: 3, readyReplicas: 1, observedGeneration: 1}
	}
	expect: {healthy: false, message: "Ready:1/3"}
}

"not healthy mid-rollout": test.#ComponentStatus & _worker & {
	parameter: image: "shop-worker:1.0"
	observed: output: {
		spec: replicas: 2
		status: {replicas: 3, updatedReplicas: 1, readyReplicas: 2, observedGeneration: 1}
	}
	expect: {healthy: false, message: "Ready:2/2"}
}

"not healthy before the controller has seen the change": test.#ComponentStatus & _worker & {
	parameter: image: "shop-worker:1.0"
	observed: output: {
		metadata: generation: 2
		spec: replicas:       1
		status: {replicas: 1, updatedReplicas: 1, readyReplicas: 1, observedGeneration: 1}
	}
	expect: healthy: false
}

"not healthy with no status reported": test.#ComponentStatus & _worker & {
	parameter: image: "shop-worker:1.0"
	observed: output: spec: replicas: 1
	expect: {healthy: false, message: "Ready:0/1"}
}

"every probe field passes through": test.#ComponentRender & _worker & {
	parameter: {
		image: "shop-worker:1.0"
		livenessProbe: {
			httpGet: {path: "/alive", port: 8080}
			initialDelaySeconds: 5
			periodSeconds:       20
			timeoutSeconds:      2
			successThreshold:    1
			failureThreshold:    6
		}
		readinessProbe: {
			exec: command: ["cat", "/ready"]
			initialDelaySeconds: 3
			periodSeconds:       15
			timeoutSeconds:      4
			successThreshold:    2
			failureThreshold:    5
		}
	}
	expect: output: spec: template: spec: containers: [{
		livenessProbe: {
			httpGet: {path: "/alive", port: 8080}
			initialDelaySeconds: 5
			periodSeconds:       20
			timeoutSeconds:      2
			successThreshold:    1
			failureThreshold:    6
		}
		readinessProbe: {
			exec: command: ["cat", "/ready"]
			initialDelaySeconds: 3
			periodSeconds:       15
			timeoutSeconds:      4
			successThreshold:    2
			failureThreshold:    5
		}
	}]
}

"an exec liveness and an httpGet or tcpSocket readiness probe pass through": test.#ComponentRender & _worker & {
	parameter: {
		image: "shop-worker:1.0"
		livenessProbe: exec: command: ["cat", "/alive"]
		readinessProbe: {
			httpGet: {path: "/ready", port: 8080}
			tcpSocket: port: 8080
		}
	}
	expect: output: spec: template: spec: containers: [{
		livenessProbe: exec: command: ["cat", "/alive"]
		readinessProbe: {
			httpGet: {path: "/ready", port: 8080}
			tcpSocket: port: 8080
		}
	}]
}

"a tcpSocket liveness probe passes through": test.#ComponentRender & _worker & {
	parameter: {
		image: "shop-worker:1.0"
		livenessProbe: tcpSocket: port: 8080
	}
	expect: output: spec: template: spec: containers: [{livenessProbe: tcpSocket: port: 8080}]
}

"a deprecated emptyDir volume keeps its medium": test.#ComponentRender & _worker & {
	parameter: {
		image: "shop-worker:1.0"
		volumes: [{name: "cache", mountPath: "/cache", type: "emptyDir", medium: "Memory"}]
	}
	expect: output: spec: template: spec: {
		containers: [{volumeMounts: [{name: "cache", mountPath: "/cache"}]}]
		volumes: [{name: "cache", emptyDir: medium: "Memory"}]
	}
}
