import (
	"vela/test"
	"vela/kube"
	"vela/config"
)

// A platform config: made in vela-system from a system template, so any namespace may read it.
registry: {
	namespace: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "Namespace", metadata: name: "vela-system"}}
	template: kube.#Apply & {
		$params: value: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {
				name:      "config-template-registry"
				namespace: "vela-system"
				labels: "config.oam.dev/catalog": "velacore-config"
			}
			data: template: """
				metadata: {name: "registry", scope: "system"}
				template: parameter: url: string
				"""
		}
	}
	store: config.#CreateConfig & {
		$params: {
			name:      "registry"
			namespace: "vela-system"
			template:  "registry"
			config: url: "https://registry.example.com"
		}
	}
} @before()

"vela-config reads a Config for real": test.#SourceExec & {
	definition: "../../../../../vela-templates/definitions/internal/source/vela-config"
	parameter: name: "registry"
	expect: output: {
		properties: url: "https://registry.example.com"
		output: {kind: "Secret", name: "registry", namespace: "vela-system"}
	}
}
