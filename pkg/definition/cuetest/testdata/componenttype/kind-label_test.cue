import "vela/test"

_workload: {apiVersion: "apps/v1", kind: "Deployment"}

"the workload's type defaults to the synthetic component's": test.#TraitRender & {
	definition: "kind-label"
	workload:   _workload
	expect: output: metadata: labels: "example.com/component-type": "cuetest-workload"
}

"context.componentType names the workload's type": test.#TraitRender & {
	definition: "kind-label"
	context: componentType: "webservice"
	workload: _workload
	expect: {
		output: metadata: labels: "example.com/component-type": "webservice"
		context: componentType: "webservice"
	}
}

"a trait's status sees the workload's type": test.#TraitStatus & {
	definition: "kind-label"
	context: componentType: "worker"
	workload: _workload
	expect: message: "on a worker"
}
