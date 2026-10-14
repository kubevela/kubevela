import "vela/test"

_web: {
	definition: "container-ports"
	context: name: "web"
	_bare: *false | bool
	_webPorts: [{containerPort: 8080, if !_bare {protocol: "TCP", name: "http"}}]
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [
			{name: "web", image: "shop:1.0", ports: _webPorts},
			{name: "proxy", image: "envoy:1.30"},
		]
	}
}

"a container without ports gets those given, TCP by default": test.#TraitRender & _web & {
	parameter: {containerName: "proxy", ports: [{containerPort: 9901}, {containerPort: 53, protocol: "UDP", hostPort: 5353, hostIP: "127.0.0.1"}]}
	expect: output: spec: template: spec: containers: [
		{name: "web", ports: [{containerPort: 8080, name: "http", hostPort?: _|_}]},
		{name: "proxy", ports: [
			{containerPort: 9901, protocol: "TCP", hostPort?: _|_, hostIP?: _|_},
			{containerPort: 53, protocol: "UDP", hostPort: 5353, hostIP: "127.0.0.1"},
		]},
	]
}

"a known port gains its host binding and keeps its name": test.#TraitRender & _web & {
	parameter: ports: [{containerPort: 8080, hostPort: 80, hostIP: "0.0.0.0"}]
	expect: output: spec: template: spec: containers: [
		{name: "web", ports: [{containerPort: 8080, protocol: "TCP", name: "http", hostPort: 80, hostIP: "0.0.0.0"}]},
		{name: "proxy", ports?: _|_},
	]
}

"new ports are appended after the existing ones": test.#TraitRender & _web & {
	parameter: ports: [{containerPort: 9090, hostPort: 9090}]
	expect: output: spec: template: spec: containers: [{name: "web", ports: [
		{containerPort: 8080, name: "http", hostPort?: _|_},
		{containerPort: 9090, protocol: "TCP", hostPort: 9090},
	]}, ...]
}

"the same port number under another protocol is a new port": test.#TraitRender & _web & {
	parameter: ports: [{containerPort: 8080, protocol: "UDP", hostPort: 8080}]
	expect: output: spec: template: spec: containers: [{name: "web", ports: [
		{containerPort: 8080, protocol: "TCP", hostPort?: _|_},
		{containerPort: 8080, protocol: "UDP", hostPort: 8080},
	]}, ...]
}

// A Deployment written by hand, rather than rendered by webservice, leaves a
// port's protocol and name to the API server's defaults.
"an existing port without a protocol or name is kept": test.#TraitRender & _web & {
	_bare: true
	parameter: ports: [{containerPort: 9090}]
	expect: output: spec: template: spec: containers: [{name: "web", ports: [
		{containerPort: 8080},
		{containerPort: 9090, protocol: "TCP"},
	]}, ...]
}

"containers sets the ports of each named container": test.#TraitRender & _web & {
	parameter: containers: [
		{containerName: "web", ports: [{containerPort: 8080, hostPort: 80}]},
		{containerName: "proxy", ports: [{containerPort: 9901}]},
	]
	expect: output: spec: template: spec: containers: [
		{name: "web", ports: [{containerPort: 8080, hostPort: 80}]},
		{name: "proxy", ports: [{containerPort: 9901, protocol: "TCP"}]},
	]
}

"a protocol must be TCP, UDP or SCTP": test.#TraitRender & _web & {
	parameter: ports: [{containerPort: 8080, protocol: "HTTP"}]
	expect: error: =~"protocol"
}

"a container that does not exist is an error": test.#TraitRender & _web & {
	parameter: {containerName: "worker", ports: [{containerPort: 8080}]}
	expect: error: user: [=~"container worker not found"]
}

"each of several containers must be named": test.#TraitRender & _web & {
	parameter: containers: [{ports: [{containerPort: 8080}]}]
	expect: error: user: [=~"container name must be set for containers"]
}

"an existing port without a protocol is matched as TCP, not duplicated": test.#TraitRender & _web & {
	_bare: true
	parameter: ports: [{containerPort: 8080, hostPort: 8080}]
	expect: output: spec: template: spec: containers: [{name: "web", ports: [
		{containerPort: 8080, hostPort: 8080, protocol?: _|_},
	]}, ...]
}
