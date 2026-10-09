import "vela/test"

_args: {
	parameter: extra: ["--verbose"]
	expect: output: spec: template: spec: containers: [{args: ["serve", "--verbose"]}]
}

"needs the upgrader": test.#ComponentRender & _args & {
	definition: "legacy"
} @upgrade(reason="args use list addition")

"broken even when upgraded": test.#ComponentRender & {
	definition: "legacy"
	parameter: extra: ["--verbose"]
	expect: output: spec: template: spec: containers: [{args: ["nope"]}]
} @upgrade(reason="args use list addition")

"fixed but still marked": test.#ComponentRender & _args & {
	definition: "modern"
} @upgrade(reason="args used list addition")

"unmarked and clean": test.#ComponentRender & _args & {
	definition: "modern"
}
