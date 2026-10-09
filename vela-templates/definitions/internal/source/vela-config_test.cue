import "vela/test"

// A Config's storage is the config package's concern, so its read is mocked.
_config: $returns: {
	properties: {url: "https://registry.example.com"}
	template: {name: "image-registry", namespace: "vela-system"}
	output: {apiVersion: "v1", kind: "Secret", name: "registry-auth", namespace: "vela-system"}
	outputs: db: {apiVersion: "v1", kind: "Secret", name: "db"}
}

"returns the Config's properties, template and outputs": test.#SourceExec & {
	definition: "vela-config"
	parameter: name: "registry"
	mocks: "vela/velaconfig": "#Read": _config
	expect: {
		output: {
			properties: url: "https://registry.example.com"
			template: name:  "image-registry"
			output: {kind: "Secret", name: "registry-auth"}
			outputs: db: {kind: "Secret", name: "db"}
		}
		calls: "vela/velaconfig": "#Read": [{$params: {name: "registry", namespace?: _|_}}]
	}
}

"passes a namespace through": test.#SourceExec & {
	definition: "vela-config"
	parameter: {name: "registry", namespace: "team-a"}
	mocks: "vela/velaconfig": "#Read": _config
	expect: calls: "vela/velaconfig": "#Read": [{$params: namespace: "team-a"}]
}

"caches per Config for five minutes": test.#SourceExec & {
	definition: "vela-config"
	parameter: name: "registry"
	mocks: "vela/velaconfig": "#Read": _config
	expect: storage: {ttl: "5m", onStaleFailure: "use-stale", keyInputs: []}
}
