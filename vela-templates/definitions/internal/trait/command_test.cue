import "vela/test"

_web: {
	definition: "command"
	context: name: "web"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [
			{name: "web", image: "shop:1.0", command: ["shop"], args: ["--port", "80", "--debug"]},
			{name: "proxy", image: "envoy:1.30", args: ["--log-level", "info"]},
		]
	}
}

"replaces the command of the container named after the component": test.#TraitRender & _web & {
	parameter: command: ["shop-server", "serve"]
	expect: output: spec: template: spec: containers: [
		{name: "web", image: "shop:1.0", command: ["shop-server", "serve"], args: ["--port", "80", "--debug"]},
		{name: "proxy", image: "envoy:1.30", command?: _|_, args: ["--log-level", "info"]},
	]
}

"no parameters leave the container as it was": test.#TraitRender & _web & {
	expect: output: spec: template: spec: containers: [
		{name: "web", command: ["shop"], args: ["--port", "80", "--debug"]},
		{name: "proxy", args: ["--log-level", "info"]},
	]
}

"args replace the existing args": test.#TraitRender & _web & {
	parameter: args: ["--port", "8080"]
	expect: output: spec: template: spec: containers: [
		{name: "web", command: ["shop"], args: ["--port", "8080"]},
		{name: "proxy", args: ["--log-level", "info"]},
	]
}

"addArgs appends only args not already there": test.#TraitRender & _web & {
	parameter: addArgs: ["--debug", "--trace"]
	expect: output: spec: template: spec: containers: [{name: "web", args: ["--port", "80", "--debug", "--trace"]}, ...]
}

"delArgs removes existing args": test.#TraitRender & _web & {
	parameter: delArgs: ["--debug"]
	expect: output: spec: template: spec: containers: [{name: "web", args: ["--port", "80"]}, ...]
}

"an arg both added and deleted is left out": test.#TraitRender & _web & {
	parameter: {addArgs: ["--trace"], delArgs: ["--trace", "--debug"]}
	expect: output: spec: template: spec: containers: [{name: "web", args: ["--port", "80"]}, ...]
}

"containerName targets another container": test.#TraitRender & _web & {
	parameter: {containerName: "proxy", command: ["envoy"], addArgs: ["--concurrency", "2"]}
	expect: output: spec: template: spec: containers: [
		{name: "web", command: ["shop"], args: ["--port", "80", "--debug"]},
		{name: "proxy", command: ["envoy"], args: ["--log-level", "info", "--concurrency", "2"]},
	]
}

"containers patches each named container": test.#TraitRender & _web & {
	parameter: containers: [
		{containerName: "web", args: ["--port", "9090"]},
		{containerName: "proxy", delArgs: ["--log-level", "info"]},
	]
	expect: output: spec: template: spec: containers: [
		{name: "web", command: ["shop"], args: ["--port", "9090"]},
		{name: "proxy", args: []},
	]
}

"args cannot be combined with addArgs": test.#TraitRender & _web & {
	parameter: {args: ["--port", "8080"], addArgs: ["--trace"]}
	expect: error: user: [=~"cannot set addArgs/delArgs and args at the same time"]
}

"a container that does not exist is an error": test.#TraitRender & _web & {
	parameter: {containerName: "worker", command: ["work"]}
	expect: error: user: [=~"container worker not found"]
}

"each of several containers must be named": test.#TraitRender & _web & {
	parameter: containers: [{command: ["work"]}]
	expect: error: user: [=~"container name must be set for containers"]
}
