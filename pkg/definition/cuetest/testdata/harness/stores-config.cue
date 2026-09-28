import "vela/config"

"stores-config": {
	type:        "workflow-step"
	description: "Stores a KubeVela Config through the config provider"
}
template: {
	store: config.#CreateConfig & {
		$params: {
			name:      parameter.name
			namespace: context.namespace
			config: url: "https://registry.example.com"
		}
	}
	parameter: name: string
}
