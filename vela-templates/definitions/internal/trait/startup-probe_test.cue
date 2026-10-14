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
}

"an unknown container is an error": test.#TraitRender & _web & {
	parameter: {containerName: "proxy", tcpSocket: port: 80}
	expect: error: user: ["container proxy not found"]
}

"each of several probes must name its container": test.#TraitRender & _web & {
	parameter: probes: [{tcpSocket: port: 80}]
	expect: error: user: ["containerName must be set when specifying startup probe for multiple containers"]
}

"an HTTP probe's host, scheme and headers, and every timing": test.#TraitRender & _web & {
	parameter: {
		httpGet: {
			path:   "/healthz"
			port:   8443
			host:   "web.internal"
			scheme: "HTTPS"
			httpHeaders: [{name: "X-Probe", value: "startup"}]
		}
		initialDelaySeconds: 5
		periodSeconds:       20
		timeoutSeconds:      2
		successThreshold:    1
	}
	expect: output: spec: template: spec: containers: [{startupProbe: {
		httpGet: {
			path:   "/healthz"
			port:   8443
			host:   "web.internal"
			scheme: "HTTPS"
			httpHeaders: [{name: "X-Probe", value: "startup"}]
		} @exact()
		initialDelaySeconds: 5
		periodSeconds:       20
		timeoutSeconds:      2
		successThreshold:    1
	}}, _]
}

"a TCP probe can name its host": test.#TraitRender & _web & {
	parameter: tcpSocket: {port: 8080, host: "web.internal"}
	expect: output: spec: template: spec: containers: [{startupProbe: {
		tcpSocket: {port: 8080, host: "web.internal"} @exact()
	}}, _]
}

"each of several probes keeps its own handler and timings": test.#TraitRender & _web & {
	parameter: probes: [
		{
			containerName: "web"
			exec: command: ["cat", "/tmp/ready"]
			initialDelaySeconds:           5
			timeoutSeconds:                2
			successThreshold:              1
			failureThreshold:              30
			terminationGracePeriodSeconds: 20
		},
		{containerName: "log", grpc: {port: 9000, service: "health"}},
	]
	expect: output: spec: template: spec: containers: [
		{name: "web", startupProbe: {
			exec: command: ["cat", "/tmp/ready"]
			initialDelaySeconds:           5
			timeoutSeconds:                2
			successThreshold:              1
			failureThreshold:              30
			terminationGracePeriodSeconds: 20
			grpc?:                         _|_
		}},
		{name: "log", startupProbe: {grpc: {port: 9000, service: "health"}, exec?: _|_}},
	]
}
