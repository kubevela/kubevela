import "vela/builtin"

waits: {
	type:        "workflow-step"
	description: "Suspends until resumed"
}
template: {
	wait: builtin.#Suspend & {$params: {}}
	parameter: {}
}
