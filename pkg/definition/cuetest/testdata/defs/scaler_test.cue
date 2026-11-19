import "vela/test"

_workload: {apiVersion: "apps/v1", kind: "Deployment", spec: paused: true}

"patches replicas": test.#TraitRender & {
	definition: "scaler"
	parameter: replicas: 3
	workload: _workload
	expect: output: spec: {replicas: 3, paused: true}
}

"healthy once scaled": test.#TraitStatus & {
	definition: "scaler"
	parameter: replicas: 3
	workload: _workload
	observed: outputs: hpa: status: currentReplicas: 3
	expect: healthy: true
}

"unhealthy below target": test.#TraitStatus & {
	definition: "scaler"
	parameter: replicas: 3
	workload: _workload
	observed: outputs: hpa: status: currentReplicas: 1
	expect: healthy: false
}

"pending on purpose": test.#TraitRender & {
	definition: "scaler"
	parameter: replicas: 3
	workload: _workload
	expect: output: spec: replicas: 99
} @pending(would fail if it ran)
