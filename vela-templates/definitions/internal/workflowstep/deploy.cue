import (
	"vela/op"
)

"deploy": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Deploy components with policies."
}
template: {
	deploy: op.#Deploy & {
		policies:    parameter.policies
		parallelism: parameter.parallelism
	}
	parameter: {
		auto: *true | bool
		policies?: [...string]
		parallelism: *5 | int
	}
}
