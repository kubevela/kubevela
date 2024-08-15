import (
	"vela/oam"
)

// apply component and traits
apply: oam.#ApplyComponent & {
	$params: parameter
}

if apply.$returns.output != _|_ {
	output: apply.$returns.output
}

if apply.$returns.outputs != _|_ {
	outputs: apply.$returns.outputs
}

parameter: {
	value: {...}
	cluster:   *"" | string
	namespace: *"" | string
}
