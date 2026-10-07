import "vela/test"

_task: {
	definition: "task"
	context: {name: "migrate", appName: "shop"}
}

"renders a Job named for the app and component, run once": test.#ComponentRender & _task & {
	parameter: image: "shop-migrate:1.0"
	expect: {
		output: {
			apiVersion: "batch/v1"
			kind:       "Job"
			metadata: name: "shop-migrate"
			spec: {
				parallelism: 1
				completions: 1
				template: {
					metadata: {
						labels: {"app.oam.dev/name": "shop", "app.oam.dev/component": "migrate"} @exact()
						annotations?: _|_
					}
					spec: {
						restartPolicy: "Never"
						containers: [{name: "migrate", image: "shop-migrate:1.0", resources?: _|_, volumeMounts?: _|_}]
						volumes?:          _|_
						imagePullSecrets?: _|_
					}
				}
			}
		}
		outputs: {} @exact()
	}
}

"an image is required": test.#ComponentRender & _task & {
	expect: error: parameter: [=~"image"]
}

"count sets both parallelism and completions": test.#ComponentRender & _task & {
	parameter: {image: "shop-migrate:1.0", count: 3}
	expect: output: spec: {parallelism: 3, completions: 3}
}

"restart sets the pod's restart policy": test.#ComponentRender & _task & {
	parameter: {image: "shop-migrate:1.0", restart: "OnFailure"}
	expect: output: spec: template: spec: restartPolicy: "OnFailure"
}

"command, environment and pull policy": test.#ComponentRender & _task & {
	parameter: {
		image:           "shop-migrate:1.0"
		imagePullPolicy: "Always"
		imagePullSecrets: ["registry"]
		cmd: ["/migrate", "up"]
		env: [{name: "DB_URL", valueFrom: secretKeyRef: {name: "db", key: "url"}}]
	}
	expect: output: spec: template: spec: {
		imagePullSecrets: [{name: "registry"}]
		containers: [{
			imagePullPolicy: "Always"
			command: ["/migrate", "up"]
			env: [{name: "DB_URL", valueFrom: secretKeyRef: {name: "db", key: "url"}}]
		}]
	}
}

"cpu and memory are both request and limit": test.#ComponentRender & _task & {
	parameter: {image: "shop-migrate:1.0", cpu: "250m", memory: "128Mi"}
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: "250m", memory: "128Mi"}
		limits: {cpu: "250m", memory: "128Mi"}
	}}]
}

"pod labels and annotations": test.#ComponentRender & _task & {
	parameter: {
		image: "shop-migrate:1.0"
		labels: team:       "payments"
		annotations: owner: "payments"
	}
	expect: output: spec: template: metadata: {
		labels: {team: "payments", "app.oam.dev/name": "shop", "app.oam.dev/component": "migrate"} @exact()
		annotations: owner: "payments"
	}
}

"volumes mount each type, defaulting to an emptyDir": test.#ComponentRender & _task & {
	parameter: {
		image: "shop-migrate:1.0"
		volumes: [
			{name: "data", mountPath: "/data", type: "pvc", claimName: "shop-data"},
			{name: "conf", mountPath: "/etc/shop", type: "configMap", cmName: "shop-conf"},
			{name: "creds", mountPath: "/etc/creds", type: "secret", secretName: "shop-creds"},
			{name: "scratch", mountPath: "/scratch"},
		]
	}
	expect: output: spec: template: spec: {
		containers: [{volumeMounts: [
			{name: "data", mountPath: "/data"},
			{name: "conf", mountPath: "/etc/shop"},
			{name: "creds", mountPath: "/etc/creds"},
			{name: "scratch", mountPath: "/scratch"},
		]}]
		volumes: [
			{name: "data", persistentVolumeClaim: claimName: "shop-data"},
			{name: "conf", configMap: {name: "shop-conf", defaultMode: 420}},
			{name: "creds", secret: {secretName: "shop-creds", defaultMode: 420}},
			{name: "scratch", emptyDir: medium: ""},
		]
	}
}

"probes pass through": test.#ComponentRender & _task & {
	parameter: {
		image: "shop-migrate:1.0"
		livenessProbe: exec: command: ["cat", "/alive"]
		readinessProbe: tcpSocket: port: 8080
	}
	expect: output: spec: template: spec: containers: [{
		livenessProbe: exec: command: ["cat", "/alive"]
		readinessProbe: tcpSocket: port: 8080
	}]
} @pending(livenessProbe and readinessProbe are accepted as parameters but never rendered into the container)

"healthy once every parallel pod has succeeded": test.#ComponentStatus & _task & {
	parameter: {image: "shop-migrate:1.0", count: 2}
	observed: output: status: {succeeded: 2}
	expect: {healthy: true, message: "Active/Failed/Succeeded:0/0/2"}
}

"not healthy while pods are still running": test.#ComponentStatus & _task & {
	parameter: {image: "shop-migrate:1.0", count: 2}
	observed: output: status: {active: 1, succeeded: 1}
	expect: {healthy: false, message: "Active/Failed/Succeeded:1/0/1"}
}

"not healthy after a failure, which the message counts": test.#ComponentStatus & _task & {
	parameter: image: "shop-migrate:1.0"
	observed: output: status: {failed: 1}
	expect: {healthy: false, message: "Active/Failed/Succeeded:0/1/0"}
}

"not healthy with no status reported": test.#ComponentStatus & _task & {
	parameter: image: "shop-migrate:1.0"
	expect: {healthy: false, message: "Active/Failed/Succeeded:0/0/0"}
}
