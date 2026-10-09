import "vela/test"

_web: {
	definition: "securitycontext"
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

"locks down the component's container, leaving the others": test.#TraitRender & _web & {
	parameter: runAsUser: 1000
	expect: {
		output: spec: template: spec: containers: [
			{
				name:  "web"
				image: "shop:1.0"
				securityContext: {
					runAsUser:                1000
					allowPrivilegeEscalation: false
					readOnlyRootFilesystem:   false
					privileged:               false
					runAsNonRoot:             true
					capabilities: {} @exact()
				} @exact()
			},
			{name: "log", securityContext?: _|_},
		]
		outputs: {} @exact()
	}
}

"applies the locked-down defaults with no parameters at all": test.#TraitRender & _web & {
	expect: output: spec: template: spec: containers: [
		{name: "web", securityContext: {runAsNonRoot: true, privileged: false}},
		_,
	]
}

"group, read-only root and capabilities when given": test.#TraitRender & _web & {
	parameter: {
		runAsGroup:             3000
		readOnlyRootFilesystem: true
		addCapabilities: ["NET_BIND_SERVICE"]
		dropCapabilities: ["ALL"]
	}
	expect: output: spec: template: spec: containers: [{securityContext: {
		runAsGroup:             3000
		readOnlyRootFilesystem: true
		capabilities: {add: ["NET_BIND_SERVICE"], drop: ["ALL"]}
	}}, _]
}

"containerName targets another container": test.#TraitRender & _web & {
	parameter: {containerName: "log", privileged: true, runAsNonRoot: false}
	expect: output: spec: template: spec: containers: [
		{name: "web", securityContext?: _|_},
		{name: "log", securityContext: {privileged: true, runAsNonRoot: false}},
	]
}

"containers set each one its own context": test.#TraitRender & _web & {
	parameter: containers: [
		{containerName: "web", runAsUser: 1000},
		{containerName: "log", allowPrivilegeEscalation: true},
	]
	expect: output: spec: template: spec: containers: [
		{name: "web", securityContext: {runAsUser: 1000, allowPrivilegeEscalation: false}},
		{name: "log", securityContext: {runAsUser?: _|_, allowPrivilegeEscalation: true}},
	]
}

"an unknown container is an error": test.#TraitRender & _web & {
	parameter: containerName: "proxy"
	expect: error: user: ["container proxy not found"]
}

"each of several containers must be named": test.#TraitRender & _web & {
	parameter: containers: [{runAsUser: 1000}]
	expect: error: user: ["containerName must be set for containers"]
}

"privilege escalation can be allowed": test.#TraitRender & _web & {
	parameter: allowPrivilegeEscalation: true
	expect: output: spec: template: spec: containers: [{securityContext: allowPrivilegeEscalation: true}, _]
}

"each of several containers takes every field": test.#TraitRender & _web & {
	parameter: containers: [
		{
			containerName:          "web"
			runAsGroup:             3000
			readOnlyRootFilesystem: true
			addCapabilities: ["NET_BIND_SERVICE"]
			dropCapabilities: ["ALL"]
		},
		{containerName: "log", privileged: true, runAsNonRoot: false},
	]
	expect: output: spec: template: spec: containers: [
		{name: "web", securityContext: {
			runAsGroup:             3000
			readOnlyRootFilesystem: true
			privileged:             false
			runAsNonRoot:           true
			capabilities: {add: ["NET_BIND_SERVICE"], drop: ["ALL"]} @exact()
		}},
		{name: "log", securityContext: {
			privileged:             true
			runAsNonRoot:           false
			readOnlyRootFilesystem: false
			capabilities: {} @exact()
		}},
	]
}
