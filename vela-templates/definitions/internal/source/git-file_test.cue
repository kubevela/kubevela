import "vela/test"

_read: {
	definition: "git-file"
	parameter: {registry: "platform", path: "shop/config.yaml"}
}

"decodes a YAML file": test.#SourceExec & _read & {
	mocks: "vela/registry": "#ReadFile": $returns: {found: true, content: "tier: gold\n"}
	expect: {
		output: {found: true, content: tier: "gold"}
		calls: "vela/registry": "#ReadFile": [{$params: {registry: "platform", path: "shop/config.yaml", ref?: _|_}}]
	}
}

"passes a ref through": test.#SourceExec & _read & {
	parameter: ref: "v2"
	mocks: "vela/registry": "#ReadFile": $returns: {found: true, content: "tier: gold\n"}
	expect: calls: "vela/registry": "#ReadFile": [{$params: ref: "v2"}]
}

"decodes a JSON file": test.#SourceExec & {
	definition: "git-file"
	parameter: {registry: "platform", path: "shop/config.json"}
	mocks: "vela/registry": "#ReadFile": $returns: {found: true, content: "{\"tier\": \"gold\"}"}
	expect: output: content: tier: "gold"
}

"keeps any other file as a string": test.#SourceExec & {
	definition: "git-file"
	parameter: {registry: "platform", path: "shop/VERSION"}
	mocks: "vela/registry": "#ReadFile": $returns: {found: true, content: "1.4.2"}
	expect: output: content: "1.4.2"
}

"a missing required file fails the source, saying how to allow it": test.#SourceExec & _read & {
	mocks: "vela/registry": "#ReadFile": $returns: {found: false, content: ""}
	expect: error: =~"set required: false"
}

"a missing optional file reads as null": test.#SourceExec & _read & {
	parameter: required: false
	mocks: "vela/registry": "#ReadFile": $returns: {found: false, content: ""}
	expect: output: {found: false, content: null}
}

"a missing optional file is not found": test.#SourceExec & _read & {
	parameter: required: false
	mocks: "vela/registry": "#ReadFile": $returns: {found: false, content: ""}
	expect: output: found: false
}

"the registry is outside the cluster, so a test mocks it": test.#SourceExec & _read & {
	expect: error: message: =~"reaches outside the cluster"
}

"caches per file for half an hour": test.#SourceExec & _read & {
	mocks: "vela/registry": "#ReadFile": $returns: {found: true, content: "tier: gold\n"}
	expect: storage: {ttl: "30m", onStaleFailure: "use-stale", keyInputs: []}
}
