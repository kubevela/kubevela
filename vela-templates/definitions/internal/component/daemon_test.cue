import "vela/test"

_daemon: {
	definition: "daemon"
	context: {name: "agent", appName: "shop"}
}

"renders a DaemonSet selecting its component, with no Service": test.#ComponentRender & _daemon & {
	parameter: image: "log-agent:1.0"
	expect: {
		output: {
			apiVersion: "apps/v1"
			kind:       "DaemonSet"
			spec: {
				selector: {
					matchLabels: {"app.oam.dev/component": "agent"} @exact()
				}
				template: {
					metadata: {
						labels: {"app.oam.dev/name": "shop", "app.oam.dev/component": "agent"} @exact()
						annotations?: _|_
					}
					spec: {
						containers: [{name: "agent", image: "log-agent:1.0", ports?: _|_, resources?: _|_, volumeMounts?: _|_}]
						volumes?:     _|_
						hostAliases?: _|_
					}
				}
			}
		}
		outputs: {} @exact()
	}
}

"an image is required": test.#ComponentRender & _daemon & {
	expect: error: parameter: [=~"image"]
}

"port is the container's only port": test.#ComponentRender & _daemon & {
	parameter: {image: "log-agent:1.0", port: 8080}
	expect: {
		output: spec: template: spec: containers: [{ports: [{containerPort: 8080}]}]
		outputs: {} @exact()
	}
}

"ports are named, and exposed ones become a Service": test.#ComponentRender & _daemon & {
	parameter: {
		image: "log-agent:1.0"
		ports: [
			{port: 24224, expose: true},
			{port: 514, protocol: "UDP", name: "syslog", expose: true},
			{port: 9090, name: "metrics"},
		]
	}
	expect: {
		output: spec: template: spec: containers: [{ports: [
			{containerPort: 24224, protocol: "TCP", name: "port-24224"},
			{containerPort: 514, protocol: "UDP", name: "syslog"},
			{containerPort: 9090, protocol: "TCP", name: "metrics"},
		]}]
		outputs: webserviceExpose: {
			apiVersion: "v1"
			kind:       "Service"
			metadata: name: "agent"
			spec: {
				selector: "app.oam.dev/component": "agent"
				type: "ClusterIP"
				ports: [
					{port: 24224, targetPort: 24224, name: "port-24224"},
					{port: 514, targetPort: 514, name: "syslog"},
				]
			}
		}
	}
}

"ports take precedence over port": test.#ComponentRender & _daemon & {
	parameter: {image: "log-agent:1.0", port: 8080, ports: [{port: 9090}]}
	expect: output: spec: template: spec: containers: [{ports: [{containerPort: 9090}]}]
}

"exposeType sets the Service type": test.#ComponentRender & _daemon & {
	parameter: {image: "log-agent:1.0", exposeType: "NodePort", ports: [{port: 24224, expose: true}]}
	expect: outputs: webserviceExpose: spec: type: "NodePort"
}

"command, environment and pull policy": test.#ComponentRender & _daemon & {
	parameter: {
		image:           "log-agent:1.0"
		imagePullPolicy: "IfNotPresent"
		imagePullSecrets: ["registry"]
		cmd: ["/agent"]
		env: [{name: "LEVEL", value: "info"}]
	}
	expect: output: spec: template: spec: {
		imagePullSecrets: [{name: "registry"}]
		containers: [{
			imagePullPolicy: "IfNotPresent"
			command: ["/agent"]
			env: [{name: "LEVEL", value: "info"}]
		}]
	}
}

"cpu and memory are both request and limit": test.#ComponentRender & _daemon & {
	parameter: {image: "log-agent:1.0", cpu: "100m", memory: "64Mi"}
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: "100m", memory: "64Mi"}
		limits: {cpu: "100m", memory: "64Mi"}
	}}]
}

"volumeMounts mount each kind, a volume shared by name once": test.#ComponentRender & _daemon & {
	parameter: {
		image: "log-agent:1.0"
		volumeMounts: {
			pvc: [{name: "data", mountPath: "/data", claimName: "agent-data"}]
			configMap: [{name: "conf", mountPath: "/etc/agent", cmName: "agent-conf"}]
			secret: [{name: "creds", mountPath: "/etc/creds", secretName: "agent-creds"}]
			emptyDir: [{name: "buffer", mountPath: "/buffer"}]
			hostPath: [
				{name: "varlog", mountPath: "/var/log", path: "/var/log"},
				{name: "varlog", mountPath: "/host/log", path: "/var/log"},
			]
		}
	}
	expect: output: spec: template: spec: {
		containers: [{volumeMounts: [
			{name: "data", mountPath: "/data"},
			{name: "conf", mountPath: "/etc/agent"},
			{name: "creds", mountPath: "/etc/creds"},
			{name: "buffer", mountPath: "/buffer"},
			{name: "varlog", mountPath: "/var/log"},
			{name: "varlog", mountPath: "/host/log"},
		]}]
		volumes: [
			{name: "data", persistentVolumeClaim: claimName: "agent-data"},
			{name: "conf", configMap: {name: "agent-conf", defaultMode: 420}},
			{name: "creds", secret: {secretName: "agent-creds", defaultMode: 420}},
			{name: "buffer", emptyDir: medium: ""},
			{name: "varlog", hostPath: path: "/var/log"},
		]
	}
}

"a hostPath mount is read-only and propagated when asked": test.#ComponentRender & _daemon & {
	parameter: {
		image: "log-agent:1.0"
		volumeMounts: hostPath: [{name: "varlog", mountPath: "/var/log", path: "/var/log", readOnly: true, mountPropagation: "HostToContainer"}]
	}
	expect: output: spec: template: spec: containers: [{volumeMounts: [
		{name: "varlog", mountPath: "/var/log", readOnly: true, mountPropagation: "HostToContainer"},
	]}]
}

"the deprecated volumes still mount": test.#ComponentRender & _daemon & {
	parameter: {
		image: "log-agent:1.0"
		volumes: [{name: "creds", mountPath: "/etc/creds", type: "secret", secretName: "agent-creds"}]
	}
	expect: output: spec: template: spec: {
		containers: [{volumeMounts: [{name: "creds", mountPath: "/etc/creds"}]}]
		volumes: [{name: "creds", secret: secretName: "agent-creds"}]
	}
}

"probes and host aliases pass through": test.#ComponentRender & _daemon & {
	parameter: {
		image: "log-agent:1.0"
		livenessProbe: httpGet: {path: "/healthz", port: 8080}
		readinessProbe: tcpSocket: port: 24224
		hostAliases: [{ip: "10.0.0.1", hostnames: ["collector.local"]}]
	}
	expect: output: spec: template: spec: {
		hostAliases: [{ip: "10.0.0.1", hostnames: ["collector.local"]}]
		containers: [{
			livenessProbe: httpGet: {path: "/healthz", port: 8080}
			readinessProbe: tcpSocket: port: 24224
		}]
	}
}

"pod labels and annotations, and the revision label when asked": test.#ComponentRender & _daemon & {
	context: revision: "agent-v2"
	parameter: {
		image: "log-agent:1.0"
		labels: team:       "platform"
		annotations: owner: "platform"
		addRevisionLabel: true
	}
	expect: output: spec: template: metadata: {
		labels: {team: "platform", "app.oam.dev/revision": "agent-v2", "app.oam.dev/component": "agent"}
		annotations: owner: "platform"
	}
}

"healthy once every scheduled pod is current, updated and ready": test.#ComponentStatus & _daemon & {
	parameter: image: "log-agent:1.0"
	observed: output: status: {desiredNumberScheduled: 3, currentNumberScheduled: 3, updatedNumberScheduled: 3, numberReady: 3, observedGeneration: 1}
	expect: {healthy: true, message: "Ready:3/3"}
}

"not healthy while pods are not ready": test.#ComponentStatus & _daemon & {
	parameter: image: "log-agent:1.0"
	observed: output: status: {desiredNumberScheduled: 3, currentNumberScheduled: 3, updatedNumberScheduled: 3, numberReady: 2, observedGeneration: 1}
	expect: {healthy: false, message: "Ready:2/3"}
}

"not healthy mid-rollout": test.#ComponentStatus & _daemon & {
	parameter: image: "log-agent:1.0"
	observed: output: status: {desiredNumberScheduled: 3, currentNumberScheduled: 3, updatedNumberScheduled: 1, numberReady: 3, observedGeneration: 1}
	expect: {healthy: false, message: "Ready:3/3"}
}

"not healthy while pods are still being scheduled": test.#ComponentStatus & _daemon & {
	parameter: image: "log-agent:1.0"
	observed: output: status: {desiredNumberScheduled: 3, currentNumberScheduled: 2, updatedNumberScheduled: 3, numberReady: 3, observedGeneration: 1}
	expect: healthy: false
}

"not healthy before the controller has seen the change": test.#ComponentStatus & _daemon & {
	parameter: image: "log-agent:1.0"
	observed: output: {
		metadata: generation: 2
		status: {desiredNumberScheduled: 1, currentNumberScheduled: 1, updatedNumberScheduled: 1, numberReady: 1, observedGeneration: 1}
	}
	expect: healthy: false
}

"every probe field passes through": test.#ComponentRender & _daemon & {
	parameter: {
		image: "log-agent:1.0"
		livenessProbe: {
			exec: command: ["cat", "/alive"]
			initialDelaySeconds: 5
			periodSeconds:       20
			timeoutSeconds:      2
			successThreshold:    1
			failureThreshold:    6
		}
		readinessProbe: {
			httpGet: {path: "/ready", port: 8080}
			initialDelaySeconds: 3
			periodSeconds:       15
			timeoutSeconds:      4
			successThreshold:    2
			failureThreshold:    5
		}
	}
	expect: output: spec: template: spec: containers: [{
		livenessProbe: {
			exec: command: ["cat", "/alive"]
			initialDelaySeconds: 5
			periodSeconds:       20
			timeoutSeconds:      2
			successThreshold:    1
			failureThreshold:    6
		}
		readinessProbe: {
			httpGet: {path: "/ready", port: 8080}
			initialDelaySeconds: 3
			periodSeconds:       15
			timeoutSeconds:      4
			successThreshold:    2
			failureThreshold:    5
		}
	}]
}

"a tcpSocket liveness and an exec readiness probe pass through": test.#ComponentRender & _daemon & {
	parameter: {
		image: "log-agent:1.0"
		livenessProbe: tcpSocket: port: 8080
		readinessProbe: exec: command: ["cat", "/alive"]
	}
	expect: output: spec: template: spec: containers: [{
		livenessProbe: tcpSocket: port: 8080
		readinessProbe: exec: command: ["cat", "/alive"]
	}]
}

"environment can come from a Secret or a ConfigMap": test.#ComponentRender & _daemon & {
	parameter: {
		image: "log-agent:1.0"
		env: [
			{name: "PASSWORD", valueFrom: secretKeyRef: {name: "shop-db", key: "password"}},
			{name: "MODE", valueFrom: configMapKeyRef: {name: "shop-conf", key: "mode"}},
		]
	}
	expect: output: spec: template: spec: containers: [{env: [
		{name: "PASSWORD", valueFrom: secretKeyRef: {name: "shop-db", key: "password"}},
		{name: "MODE", valueFrom: configMapKeyRef: {name: "shop-conf", key: "mode"}},
	]}]
}

"a deprecated emptyDir volume keeps its medium": test.#ComponentRender & _daemon & {
	parameter: {
		image: "log-agent:1.0"
		volumes: [{name: "cache", mountPath: "/cache", type: "emptyDir", medium: "Memory"}]
	}
	expect: output: spec: template: spec: {
		containers: [{volumeMounts: [{name: "cache", mountPath: "/cache"}]}]
		volumes: [{name: "cache", emptyDir: medium: "Memory"}]
	}
}
