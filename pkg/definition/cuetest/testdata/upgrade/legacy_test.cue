import "vela/test"

// List addition was removed from CUE; only KubeVela's upgrader rewrites it.
"appends extra args": test.#ComponentRender & {
	definition: "legacy"
	parameter: extra: ["--verbose"]
	expect: output: spec: template: spec: containers: [{args: ["serve", "--verbose"]}]
}
