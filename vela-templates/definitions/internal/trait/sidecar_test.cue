import "vela/test"

_web: {
	definition: "sidecar"
	context: name: "web"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [{name: "web", image: "shop:1.0"}]
	}
}

"adds a container beside the workload's own": test.#TraitRender & _web & {
	parameter: {name: "proxy", image: "envoy:1.30"}
	expect: {
		output: spec: template: spec: containers: [
			{name: "web", image: "shop:1.0"},
			{name: "proxy", image: "envoy:1.30", command?: _|_, args?: _|_, env?: _|_, ports?: _|_, volumeMounts?: _|_, livenessProbe?: _|_, readinessProbe?: _|_},
		]
		outputs: {} @exact()
	}
}

// Without a name the patch has no key to add by, so it lands on the first container.
"without a name the sidecar collides with the workload's container": test.#TraitRender & _web & {
	parameter: image: "envoy:1.30"
	expect: error: template: [=~"containers.0.image: conflicting values"]
}

"command, arguments, environment and ports": test.#TraitRender & _web & {
	parameter: {
		name:  "proxy"
		image: "envoy:1.30"
		cmd: ["envoy"]
		args: ["-c", "/etc/envoy.yaml"]
		env: [{name: "POD", valueFrom: fieldRef: fieldPath: "metadata.name"}]
		ports: [{containerPort: 9901, name: "admin"}]
	}
	expect: output: spec: template: spec: containers: [_, {
		command: ["envoy"]
		args: ["-c", "/etc/envoy.yaml"]
		env: [{name: "POD", valueFrom: fieldRef: fieldPath: "metadata.name"}]
		ports: [{containerPort: 9901, name: "admin", protocol: "TCP"}]
	}]
}

"volumes become mounts, sharing the workload's volumes by name": test.#TraitRender & _web & {
	parameter: {name: "proxy", image: "envoy:1.30", volumes: [{name: "logs", path: "/var/log/envoy"}]}
	expect: output: spec: template: spec: {
		volumes?: _|_
		containers: [_, {volumeMounts: [{name: "logs", mountPath: "/var/log/envoy"}]}]
	}
}

"probes take the Kubernetes defaults": test.#TraitRender & _web & {
	parameter: {
		name:  "proxy"
		image: "envoy:1.30"
		livenessProbe: tcpSocket: port: 9901
		readinessProbe: httpGet: {path: "/ready", port: 9901}
	}
	expect: output: spec: template: spec: containers: [_, {
		livenessProbe: {
			tcpSocket: port: 9901
			initialDelaySeconds: 0
			periodSeconds:       10
			timeoutSeconds:      1
			successThreshold:    1
			failureThreshold:    3
		}
		readinessProbe: httpGet: {path: "/ready", port: 9901}
	}]
}

"a port outside the valid range is rejected": test.#TraitRender & _web & {
	parameter: {name: "proxy", image: "envoy:1.30", ports: [{containerPort: 70000}]}
	expect: error: parameter: [=~"containerPort"]
}

"every probe field passes through": test.#TraitRender & _web & {
	parameter: {
		name:  "proxy"
		image: "envoy:1.30"
		livenessProbe: {
			exec: command: ["envoy", "--health"]
			httpGet: {path: "/alive", port: 9901}
			initialDelaySeconds: 5
			periodSeconds:       20
			timeoutSeconds:      2
			successThreshold:    1
			failureThreshold:    6
		}
		readinessProbe: {
			exec: command: ["envoy", "--ready"]
			tcpSocket: port: 9901
			initialDelaySeconds: 3
			periodSeconds:       15
			timeoutSeconds:      4
			successThreshold:    2
			failureThreshold:    5
		}
	}
	expect: output: spec: template: spec: containers: [_, {
		livenessProbe: {
			exec: command: ["envoy", "--health"]
			httpGet: {path: "/alive", port: 9901}
			initialDelaySeconds: 5
			periodSeconds:       20
			timeoutSeconds:      2
			successThreshold:    1
			failureThreshold:    6
		}
		readinessProbe: {
			exec: command: ["envoy", "--ready"]
			tcpSocket: port: 9901
			initialDelaySeconds: 3
			periodSeconds:       15
			timeoutSeconds:      4
			successThreshold:    2
			failureThreshold:    5
		}
	}]
}

"a plain env value, and a port's protocol and hostPort": test.#TraitRender & _web & {
	parameter: {
		name:  "proxy"
		image: "envoy:1.30"
		env: [{name: "LOG_LEVEL", value: "debug"}]
		ports: [{containerPort: 5353, protocol: "UDP", hostPort: 53}]
	}
	expect: output: spec: template: spec: containers: [_, {
		env: [{name: "LOG_LEVEL", value: "debug"}]
		ports: [{containerPort: 5353, protocol: "UDP", hostPort: 53}]
	}]
}
