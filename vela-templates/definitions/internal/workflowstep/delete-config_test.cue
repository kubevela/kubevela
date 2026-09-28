import "vela/test"

// Configs are deleted through the controller's config factory, which a step
// test does not have, so #DeleteConfig is always mocked.
_deleted: mocks: "vela/config": "#DeleteConfig": {}

"deletes the config in the step's namespace": test.#WorkflowStepExec & _deleted & {
	definition: "delete-config"
	context: namespace: "team-a"
	parameter: name:    "db"
	expect: {
		phase: "succeeded"
		calls: "vela/config": "#DeleteConfig": [{$params: {name: "db", namespace: "team-a"} @exact()}]
	}
}

"deletes the config in the given namespace": test.#WorkflowStepExec & _deleted & {
	definition: "delete-config"
	parameter: {name: "registry", namespace: "vela-system"}
	expect: {
		phase: "succeeded"
		calls: "vela/config": "#DeleteConfig": [{$params: {name: "registry", namespace: "vela-system"}}]
	}
}

"a name is required": test.#WorkflowStepExec & _deleted & {
	definition: "delete-config"
	expect: {
		phase:   "failed"
		message: =~"name: cannot convert non-concrete value"
		calls: "vela/config"?: _|_
	}
}
