import "vela/test"

_web: {
	definition: "env"
	context: name: "web"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [
			{name: "web", image: "shop:1.0", env: [
				{name: "LOG_LEVEL", value: "info"},
				{name: "DB_PASSWORD", valueFrom: secretKeyRef: {name: "db", key: "password"}},
			]},
			{name: "proxy", image: "envoy:1.30"},
		]
	}
}

"merges into the env of the container named after the component": test.#TraitRender & _web & {
	parameter: env: {LOG_LEVEL: "debug", REGION: "eu-west-1"}
	expect: output: spec: template: spec: containers: [
		{name: "web", env: [
			{name: "LOG_LEVEL", value: "debug"},
			{name: "DB_PASSWORD", valueFrom: secretKeyRef: {name: "db", key: "password"}, value?: _|_},
			{name: "REGION", value: "eu-west-1"},
		]},
		{name: "proxy", env?: _|_},
	]
}

"a container without env gets the variables given": test.#TraitRender & _web & {
	parameter: {containerName: "proxy", env: {REGION: "eu-west-1", ZONE: "a"}}
	expect: output: spec: template: spec: containers: [
		{name: "web", env: [{name: "LOG_LEVEL", value: "info"}, {name: "DB_PASSWORD"}]},
		{name: "proxy", env: [{name: "REGION", value: "eu-west-1"}, {name: "ZONE", value: "a"}]},
	]
}

"replace drops the variables not given": test.#TraitRender & _web & {
	parameter: {replace: true, env: REGION: "eu-west-1"}
	expect: output: spec: template: spec: containers: [{name: "web", env: [{name: "REGION", value: "eu-west-1"}]}, ...]
}

"unset removes existing variables and wins over env": test.#TraitRender & _web & {
	parameter: {unset: ["LOG_LEVEL", "REGION"], env: {REGION: "eu-west-1", ZONE: "a"}}
	expect: output: spec: template: spec: containers: [{name: "web", env: [
		{name: "DB_PASSWORD", valueFrom: secretKeyRef: {name: "db", key: "password"}},
		{name: "ZONE", value: "a"},
	]}, ...]
}

"containers sets the env of each named container": test.#TraitRender & _web & {
	parameter: containers: [
		{containerName: "web", env: LOG_LEVEL: "warn"},
		{containerName: "proxy", env: ENVOY_UID: "0"},
	]
	expect: output: spec: template: spec: containers: [
		{name: "web", env: [{name: "LOG_LEVEL", value: "warn"}, {name: "DB_PASSWORD", valueFrom: secretKeyRef: {name: "db", key: "password"}}]},
		{name: "proxy", env: [{name: "ENVOY_UID", value: "0"}]},
	]
}

"a value must be a string": test.#TraitRender & _web & {
	parameter: env: REPLICAS: 3
	expect: error: {
		parameter: [=~"mismatched types int and string"] @contains()
	}
}

"a container that does not exist is an error": test.#TraitRender & _web & {
	parameter: {containerName: "worker", env: REGION: "eu-west-1"}
	expect: error: user: [=~"container worker not found"]
}

"each of several containers must be named": test.#TraitRender & _web & {
	parameter: containers: [{env: REGION: "eu-west-1"}]
	expect: error: user: [=~"containerName must be set for containers"]
}

"a container can unset its own variables": test.#TraitRender & _web & {
	parameter: containers: [
		{containerName: "web", unset: ["LOG_LEVEL"], env: REGION: "eu-west-1"},
		{containerName: "proxy", env: ENVOY_UID: "0"},
	]
	expect: output: spec: template: spec: containers: [
		{name: "web", env: [
			{name: "DB_PASSWORD", valueFrom: secretKeyRef: {name: "db", key: "password"}},
			{name: "REGION", value: "eu-west-1"},
		]},
		{name: "proxy", env: [{name: "ENVOY_UID", value: "0"}]},
	]
}

"a container can replace its env, dropping what it had": test.#TraitRender & _web & {
	parameter: containers: [
		{containerName: "web", replace: true, env: REGION: "eu-west-1"},
		{containerName: "proxy", env: ENVOY_UID: "0"},
	]
	expect: output: spec: template: spec: containers: [
		{name: "web", env: [{name: "REGION", value: "eu-west-1"}]},
		{name: "proxy", env: [{name: "ENVOY_UID", value: "0"}]},
	]
}
