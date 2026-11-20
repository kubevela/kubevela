import "vela/test"

// Configs are written through the controller's config factory, which a step
// test does not have, so #CreateConfig is always mocked.
_stored: mocks: "vela/config": "#CreateConfig": {}

"creates the config in the step's namespace": test.#WorkflowStepExec & _stored & {
	definition: "create-config"
	context: namespace: "team-a"
	parameter: {name: "db", config: {host: "db.local", port: 5432}}
	expect: {
		phase: "succeeded"
		calls: "vela/config": "#CreateConfig": [{$params: {
			name:      "db"
			namespace: "team-a"
			config: {host: "db.local", port: 5432} @exact()
		} @exact()
		}]
	}
}

"creates the config from a template in the given namespace": test.#WorkflowStepExec & _stored & {
	definition: "create-config"
	parameter: {name: "registry", namespace: "vela-system", template: "image-registry", config: registry: "ghcr.io"}
	expect: {
		phase: "succeeded"
		calls: "vela/config": "#CreateConfig": [{$params: {
			name:      "registry"
			namespace: "vela-system"
			template:  "image-registry"
			config: registry: "ghcr.io"
		}}]
	}
}

"a name is required": test.#WorkflowStepExec & _stored & {
	definition: "create-config"
	parameter: config: host: "db.local"
	expect: {
		phase:   "failed"
		message: =~"name: cannot convert non-concrete value"
		calls: "vela/config"?: _|_
	}
}
