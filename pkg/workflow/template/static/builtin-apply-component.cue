import (
	"vela/oam"
)

// apply component and traits
apply: oam.#ApplyComponent & {
	$params: $parameter
}

if apply.$returns.output != _|_ {
	output: apply.output
}

if apply.$returns.outputs != _|_ {
	outputs: apply.outputs
}

parameter: {
	value: {...}
	cluster:   *"" | string
	namespace: *"" | string
}
