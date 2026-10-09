import "vela/test"

_web: {
	definition: "startup-probe"
	context: name: "web"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [
			{name: "web", image: "shop:1.0"},
			{name: "log", image: "fluentbit:3"},
		]
	}
}

"probes the component's container with the Kubernetes defaults": test.#TraitRender & _web & {
	parameter: httpGet: {path: "/healthz", port: 8080}
	expect: {
		output: spec: template: spec: containers: [
			{
				name:  "web"
				image: "shop:1.0"
				startupProbe: {
					httpGet: {path: "/healthz", port: 8080}
					initialDelaySeconds: 0
					periodSeconds:       10
					timeoutSeconds:      1
					successThreshold:    1
					failureThreshold:    3
				} @exact()
			},
			{name: "log", startupProbe?: _|_},
		]
		outputs: {} @exact()
	}
}

"exec, gRPC and TCP handlers and a grace period": test.#TraitRender & _web & {
	parameter: {
		exec: command: ["cat", "/tmp/ready"]
		grpc: {port: 9000, service: "health"}
		tcpSocket: port: 8080
		terminationGracePeriodSeconds: 30
		failureThreshold:              30
	}
	expect: output: spec: template: spec: containers: [{startupProbe: {
		exec: command: ["cat", "/tmp/ready"]
		grpc: {port: 9000, service: "health"}
		tcpSocket: port: 8080
		terminationGracePeriodSeconds: 30
		failureThreshold:              30
	}}, _]
}

"containerName targets another container": test.#TraitRender & _web & {
	parameter: {containerName: "log", tcpSocket: port: 2020}
	expect: output: spec: template: spec: containers: [
		{name: "web", startupProbe?: _|_},
		{name: "log", startupProbe: tcpSocket: port: 2020},
	]
}

"probes give each container its own": test.#TraitRender & _web & {
	parameter: probes: [
		{containerName: "web", httpGet: port: 8080},
		{containerName: "log", tcpSocket: port: 2020, periodSeconds: 5},
	]
	expect: output: spec: template: spec: containers: [
		{name: "web", startupProbe: {httpGet: port: 8080, tcpSocket?: _|_}},
		{name: "log", startupProbe: {tcpSocket: port: 2020, periodSeconds: 5}},
	]
} @pending(the probes branch reads each probe as c.name, but a probe only has containerName, so the branch cannot render)

"an unknown container is an error": test.#TraitRender & _web & {
	parameter: {containerName: "proxy", tcpSocket: port: 80}
	expect: error: user: ["container proxy not found"]
}

"each of several probes must name its container": test.#TraitRender & _web & {
	parameter: probes: [{tcpSocket: port: 80}]
	expect: error: user: ["containerName must be set when specifying startup probe for multiple containers"]
} @pending(the probes branch reads each probe as c.name, but a probe only has containerName, so the branch cannot render)
