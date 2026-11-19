import (
	"vela/test"
	"vela/kube"
	"vela/config"
)

registry: {
	namespace: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "Namespace", metadata: name: "vela-system"}}
	store: config.#CreateConfig & {
		$params: {
			name:      "registry"
			namespace: "vela-system"
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
