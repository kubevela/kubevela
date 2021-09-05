import (
	"vela/op"
)

// apply component and traits
apply: op.#ApplyComponent & {
	component: parameter.component
}
parameter: {
	// +usage=Declare the name of the component
	component: string
}
