import (
	"vela/op"
)

oam: op.oam
// apply component and traits
apply: oam.#ApplyComponent & {
	value:   parameter.value
	cluster: parameter.cluster
}

if apply.output != _|_ {
	output: apply.output
}

if apply.outputs != _|_ {
	outputs: apply.outputs
}
parameter: {
	value: {...}
	cluster: *"" | string
}
