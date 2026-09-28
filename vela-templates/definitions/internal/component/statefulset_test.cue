import "vela/test"

_sts: {
	definition: "statefulset"
	context: {name: "db", appName: "shop"}
}

"renders a StatefulSet selecting its component, with no Service": test.#ComponentRender & _sts & {
	parameter: image: "postgres:16"
	expect: {
		output: {
			apiVersion: "apps/v1"
			kind:       "StatefulSet"
			spec: {
				selector: {
					matchLabels: {"app.oam.dev/component": "db"} @exact()
				}
				template: {
					metadata: {
						labels: {"app.oam.dev/name": "shop", "app.oam.dev/component": "db"} @exact()
						annotations?: _|_
					}
					spec: {
						containers: [{name: "db", image: "postgres:16", ports?: _|_, resources?: _|_, volumeMounts?: _|_}]
						volumes?: _|_
					}
				}
			}
		}
		outputs: {} @exact()
	}
}

"an image is required": test.#ComponentRender & _sts & {
	expect: error: parameter: [=~"image"]
}

"port is the container's only port": test.#ComponentRender & _sts & {
	parameter: {image: "postgres:16", port: 5432}
	expect: {
		output: spec: template: spec: containers: [{ports: [{containerPort: 5432}]}]
		outputs: {} @exact()
	}
}

"ports are named, and exposed ones become a Service": test.#ComponentRender & _sts & {
	parameter: {
		image: "postgres:16"
		ports: [
			{port: 5432, expose: true},
			{port: 80, containerPort: 8080, expose: true},
			{port: 53, protocol: "UDP", expose: true},
			{port: 9187, name: "metrics"},
		]
	}
	expect: {
		output: spec: template: spec: containers: [{ports: [
			{containerPort: 5432, protocol: "TCP", name: "port-5432"},
			{containerPort: 8080, protocol: "TCP", name: "port-8080"},
			{containerPort: 53, protocol: "UDP", name: "port-53-udp"},
			{containerPort: 9187, name: "metrics"},
		]}]
		outputs: statefulsetsExpose: {
			apiVersion: "v1"
			kind:       "Service"
			metadata: name: "db"
			spec: {
				selector: "app.oam.dev/component": "db"
				type: "ClusterIP"
				ports: [
					{port: 5432, targetPort: 5432, name: "port-5432", protocol: "TCP"},
					{port: 80, targetPort: 8080, name: "port-8080", protocol: "TCP"},
					{port: 53, targetPort: 53, name: "port-53-udp", protocol: "UDP"},
				]
			}
		}
	}
}

"a nodePort is kept only for a NodePort Service": test.#ComponentRender & _sts & {
	parameter: {
		image:      "postgres:16"
		exposeType: "NodePort"
		ports: [{port: 5432, nodePort: 30432, expose: true}]
	}
	expect: outputs: statefulsetsExpose: spec: {type: "NodePort", ports: [{nodePort: 30432}]}
}

"a nodePort is dropped for a ClusterIP Service": test.#ComponentRender & _sts & {
	parameter: {image: "postgres:16", ports: [{port: 5432, nodePort: 30432, expose: true}]}
	expect: outputs: statefulsetsExpose: spec: ports: [{nodePort?: _|_}]
}

"command, arguments, environment and pull policy": test.#ComponentRender & _sts & {
	parameter: {
		image:           "postgres:16"
		imagePullPolicy: "Always"
		imagePullSecrets: ["registry"]
		cmd: ["postgres"]
		args: ["-c", "max_connections=200"]
		env: [{name: "PGDATA", value: "/data/pg"}]
	}
	expect: output: spec: template: spec: {
		imagePullSecrets: [{name: "registry"}]
		containers: [{
			imagePullPolicy: "Always"
			command: ["postgres"]
			args: ["-c", "max_connections=200"]
			env: [{name: "PGDATA", value: "/data/pg"}]
		}]
	}
}

"cpu and memory are both request and limit": test.#ComponentRender & _sts & {
	parameter: {image: "postgres:16", cpu: "1", memory: "1Gi"}
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: "1", memory: "1Gi"}
		limits: {cpu: "1", memory: "1Gi"}
	}}]
}

"volumeMounts mount each kind with a subPath, a volume shared by name once": test.#ComponentRender & _sts & {
	parameter: {
		image: "postgres:16"
		volumeMounts: {
			pvc: [{name: "data", mountPath: "/data", subPath: "pg", claimName: "db-data"}]
			configMap: [{name: "conf", mountPath: "/etc/postgresql", cmName: "db-conf", items: [{key: "pg.conf", path: "postgresql.conf"}]}]
			secret: [{name: "creds", mountPath: "/etc/creds", secretName: "db-creds", defaultMode: 256}]
			emptyDir: [
				{name: "shm", mountPath: "/dev/shm", medium: "Memory"},
				{name: "shm", mountPath: "/tmp/shm", subPath: "tmp"},
			]
			hostPath: [{name: "logs", mountPath: "/logs", path: "/var/log/db"}]
		}
	}
	expect: output: spec: template: spec: {
		containers: [{volumeMounts: [
			{name: "data", mountPath: "/data", subPath: "pg"},
			{name: "conf", mountPath: "/etc/postgresql", subPath?: _|_},
			{name: "creds", mountPath: "/etc/creds"},
			{name: "shm", mountPath: "/dev/shm"},
			{name: "shm", mountPath: "/tmp/shm", subPath: "tmp"},
			{name: "logs", mountPath: "/logs"},
		]}]
		volumes: [
			{name: "data", persistentVolumeClaim: claimName: "db-data"},
			{name: "conf", configMap: {name: "db-conf", defaultMode: 420, items: [{key: "pg.conf", path: "postgresql.conf", mode: 511}]}},
			{name: "creds", secret: {secretName: "db-creds", defaultMode: 256}},
			{name: "shm", emptyDir: medium: "Memory"},
			{name: "logs", hostPath: path: "/var/log/db"},
		]
	}
}

"the deprecated volumes still mount": test.#ComponentRender & _sts & {
	parameter: {
		image: "postgres:16"
		volumes: [{name: "data", mountPath: "/data", type: "pvc", claimName: "db-data"}]
	}
	expect: output: spec: template: spec: {
		containers: [{volumeMounts: [{name: "data", mountPath: "/data"}]}]
		volumes: [{name: "data", persistentVolumeClaim: claimName: "db-data"}]
	}
}

"probes and host aliases pass through": test.#ComponentRender & _sts & {
	parameter: {
		image: "postgres:16"
		livenessProbe: tcpSocket: port: 5432
		readinessProbe: exec: command: ["pg_isready"]
		hostAliases: [{ip: "10.0.0.1", hostnames: ["primary.local"]}]
	}
	expect: output: spec: template: spec: {
		hostAliases: [{ip: "10.0.0.1", hostnames: ["primary.local"]}]
		containers: [{
			livenessProbe: tcpSocket: port: 5432
			readinessProbe: exec: command: ["pg_isready"]
		}]
	}
}

"pod labels and annotations, and the revision label when asked": test.#ComponentRender & _sts & {
	context: revision: "db-v3"
	parameter: {
		image: "postgres:16"
		labels: tier:       "data"
		annotations: owner: "dba"
		addRevisionLabel: true
	}
	expect: output: spec: template: metadata: {
		labels: {tier: "data", "app.oam.dev/revision": "db-v3", "app.oam.dev/component": "db"}
		annotations: owner: "dba"
	}
}

"healthy once every replica is updated and ready": test.#ComponentStatus & _sts & {
	parameter: image: "postgres:16"
	observed: output: {
		spec: replicas: 3
		status: {replicas: 3, updatedReplicas: 3, readyReplicas: 3, observedGeneration: 1}
	}
	expect: {healthy: true, message: "Ready:3/3"}
}

"not healthy mid-rollout": test.#ComponentStatus & _sts & {
	parameter: image: "postgres:16"
	observed: output: {
		spec: replicas: 3
		status: {replicas: 3, updatedReplicas: 2, readyReplicas: 3, observedGeneration: 1}
	}
	expect: {healthy: false, message: "Ready:3/3"}
}

"not healthy while replicas are not ready": test.#ComponentStatus & _sts & {
	parameter: image: "postgres:16"
	observed: output: {
		spec: replicas: 3
		status: {replicas: 3, updatedReplicas: 3, readyReplicas: 1, observedGeneration: 1}
	}
	expect: {healthy: false, message: "Ready:1/3"}
}

"not healthy before the controller has seen the change": test.#ComponentStatus & _sts & {
	parameter: image: "postgres:16"
	observed: output: {
		metadata: generation: 2
		spec: replicas:       1
		status: {replicas: 1, updatedReplicas: 1, readyReplicas: 1, observedGeneration: 1}
	}
	expect: healthy: false
}

"the disable-health-check annotation makes it healthy": test.#ComponentStatus & _sts & {
	parameter: image: "postgres:16"
	observed: output: {
		metadata: annotations: "app.oam.dev/disable-health-check": "true"
		spec: replicas: 2
	}
	expect: {healthy: true, message: "Ready:0/2"}
}
