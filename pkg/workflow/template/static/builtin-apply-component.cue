import (
	"vela/op"
)

// apply component and traits
apply: op.#ApplyComponent & {
	value:     parameter.value
	cluster:   parameter.cluster
	namespace: parameter.namespace
}

if apply.output != _|_ {
	output: apply.output
}

if apply.outputs != _|_ {
	outputs: apply.outputs
}
parameter: {
	value: {...}
	cluster:   *"" | string
	namespace: *"" | string
}
