import "vela/test"

"the config provider stores through the test cluster": test.#WorkflowStepExec & {
	definition: "stores-config"
	parameter: name: "registry"
	expect: {
		phase: "succeeded"
		resources: [{apiVersion: "v1", kind: "Secret", metadata: name: "registry"}]
	}
}

"placements need an Application, so a test mocks them": test.#WorkflowStepExec & {
	definition: "places"
	expect: {
		phase:   "failed"
		message: =~"needs an Application"
	}
}

"a legacy function's mock answers beside its parameters": test.#WorkflowStepExec & {
	definition: "counts-policies"
	mocks: "vela/op": "#LoadPolicies": $returns: value: {topology: {type: "topology"}, override: {type: "override"}}
	expect: {
		phase: "succeeded"
		calls: "vela/op": "#LoadPolicies": [{$returns: value: topology: type: "topology"}]
		resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: name: "policy-count", data: count: "2"}]
	}
}

"builtin calls are recorded": test.#WorkflowStepExec & {
	definition: "waits"
	expect: {
		phase: "suspending"
		calls: "vela/builtin": "#Suspend": [_]
	}
}

_job: {
	apiVersion: "batch/v1"
	kind:       "Job"
	metadata: {name: "migrate", namespace: "jobs"}
	spec: template: spec: {
		restartPolicy: "Never"
		containers: [{name: "migrate", image: "shop-migrate:1.0"}]
	}
}

"a seeded Job is cleaned up after its case": test.#WorkflowStepExec & {
	definition: "waits"
	resources: [_job]
	expect: phase: "suspending"
}

"so the next case can seed it again": test.#WorkflowStepExec & {
	definition: "waits"
	resources: [_job]
	expect: phase: "suspending"
}
