import "vela/test"

_web: {
	definition: "service-binding"
	context: name: "web"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [
			{name: "web", image: "shop:1.0", env: [{name: "MODE", value: "prod"}]},
			{name: "log", image: "fluentbit:3"},
		]
	}
}

"maps each variable to a Secret key of the same name": test.#TraitRender & _web & {
	parameter: envMappings: DB_PASSWORD: secret: "db-conn"
	expect: {
		output: spec: template: spec: containers: [
			{name: "web", env: [
				{name: "MODE", value: "prod"},
				{name: "DB_PASSWORD", valueFrom: secretKeyRef: {name: "db-conn", key: "DB_PASSWORD"}},
			] @contains()
			},
			{name: "log", env?: _|_},
		]
		outputs: {} @exact()
	}
}

"a key reads a differently named Secret key": test.#TraitRender & _web & {
	parameter: envMappings: DB_HOST: {secret: "db-conn", key: "host"}
	expect: output: spec: template: spec: containers: [
		{env: [{name: "DB_HOST", valueFrom: secretKeyRef: {name: "db-conn", key: "host"}}] @contains()},
		_,
	]
}

"a mapping needs its Secret": test.#TraitRender & _web & {
	parameter: envMappings: DB_HOST: key: "host"
	expect: error: template: [=~"secretKeyRef.name: incomplete value"]
}
