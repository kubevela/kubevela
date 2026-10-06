import "vela/test"

// #Deploy dispatches the Application's components, which a step test does not
// have, so it is always mocked.
_deployed: mocks: "vela/multicluster": "#Deploy": $returns: {}

"deploys every component by default": test.#WorkflowStepExec & _deployed & {
	definition: "deploy"
	expect: {
		phase: "succeeded"
		calls: "vela/multicluster": "#Deploy": [{$params: {
			policies: []
			parallelism:              5
			ignoreTerraformComponent: true
		}}]
		calls: "vela/builtin"?: _|_
	}
}

"deploys with the given policies and parallelism": test.#WorkflowStepExec & _deployed & {
	definition: "deploy"
	parameter: {policies: ["topology-eu", "override-prod"], parallelism: 2, ignoreTerraformComponent: false}
	expect: calls: "vela/multicluster": "#Deploy": [{$params: {
		policies: ["topology-eu", "override-prod"]
		parallelism:              2
		ignoreTerraformComponent: false
	}}]
}

"waits for approval when not automatic": test.#WorkflowStepExec & _deployed & {
	definition: "deploy"
	parameter: auto: false
	expect: phase:   "suspending"
}

"says what it waits for": test.#WorkflowStepExec & _deployed & {
	definition: "deploy"
	context: stepName: "deploy-prod"
	parameter: auto:   false
	expect: message:   "Waiting approval to the deploy step \"deploy-prod\""
} @pending(kubevela/workflow builtin.#Suspend ignores its message parameter)

"deploying needs an Application, so a test mocks it": test.#WorkflowStepExec & {
	definition: "deploy"
	expect: {
		phase:   "failed"
		message: =~"needs an Application"
	}
}
