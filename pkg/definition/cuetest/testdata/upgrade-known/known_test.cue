import "vela/test"

"known upgrader dependency": test.#ComponentRender & {
	definition: "legacy"
	parameter: extra: ["--verbose"]
	expect: output: spec: template: spec: containers: [{args: ["serve", "--verbose"]}]
} @upgrade(reason="args use list addition")
