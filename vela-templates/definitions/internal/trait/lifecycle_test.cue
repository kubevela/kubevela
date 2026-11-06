import "vela/test"

_web: {
	definition: "lifecycle"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [
			{name: "web", image: "shop:1.0"},
			{name: "proxy", image: "envoy:1.30"},
		]
	}
}

"hooks every container": test.#TraitRender & _web & {
	parameter: {
		postStart: exec: command: ["sh", "-c", "echo started"]
		preStop: exec: command: ["sleep", "5"]
	}
	_hooks: {
		postStart: exec: command: ["sh", "-c", "echo started"]
		preStop: exec: command: ["sleep", "5"]
	}
	expect: output: spec: template: spec: containers: [
		{name: "web", image: "shop:1.0", lifecycle: _hooks @exact()},
		{name: "proxy", image: "envoy:1.30", lifecycle: _hooks @exact()},
	]
}

"only the hooks given are set": test.#TraitRender & _web & {
	parameter: preStop: exec: command: ["sleep", "5"]
	expect: output: spec: template: spec: containers: [
		{lifecycle: {postStart?: _|_, preStop: _}},
		{lifecycle: {postStart?: _|_, preStop: _}},
	]
}

"an HTTP hook defaults to plain HTTP": test.#TraitRender & _web & {
	parameter: preStop: httpGet: {path: "/drain", port: 8080}
	expect: output: spec: template: spec: containers: [
		{lifecycle: preStop: httpGet: {path: "/drain", port: 8080, scheme: "HTTP"}},
		{lifecycle: preStop: httpGet: {path: "/drain", port: 8080, scheme: "HTTP"}},
	]
}

"a TCP hook targets a port": test.#TraitRender & _web & {
	parameter: postStart: tcpSocket: {port: 5432, host: "db"}
	expect: output: spec: template: spec: containers: [
		{lifecycle: postStart: tcpSocket: {port: 5432, host: "db"}},
		{lifecycle: postStart: tcpSocket: {port: 5432, host: "db"}},
	]
}

"a port outside 1 to 65535 is rejected": test.#TraitRender & _web & {
	parameter: preStop: httpGet: port: 70000
	expect: error: parameter: [=~"port"]
}

"an HTTP postStart and a TCP preStop hook": test.#TraitRender & _web & {
	parameter: {
		postStart: httpGet: {path: "/warm", port: 8080, host: "localhost", scheme: "HTTPS", httpHeaders: [{name: "X-Warm", value: "1"}]}
		preStop: tcpSocket: port: 8080
	}
	_hooks: {
		postStart: httpGet: {path: "/warm", port: 8080, host: "localhost", scheme: "HTTPS", httpHeaders: [{name: "X-Warm", value: "1"}]}
		preStop: tcpSocket: port: 8080
	}
	expect: output: spec: template: spec: containers: [
		{name: "web", lifecycle: _hooks @exact()},
		{name: "proxy", lifecycle: _hooks @exact()},
	]
}
