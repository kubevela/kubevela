import (
	"vela/kube"
	"vela/builtin"
	"encoding/yaml"
)

"depends-on-app": {
	type: "workflow-step"
	annotations: {
		"category": "Application Delivery"
	}
	labels: {}
	description: "Wait for the specified Application to complete."
}

template: {
	dependsOn: kube.#Read & {
		$params: {
			value: {
				apiVersion: "core.oam.dev/v1beta1"
				kind:       "Application"
				metadata: {
					name:      parameter.name
					namespace: parameter.namespace
				}
			}
		}
	}
	load: {
		if dependsOn.$returns.err != _|_ {
			configMap: kube.#Read & {
				$params: {
					value: {
						apiVersion: "v1"
						kind:       "ConfigMap"
						metadata: {
							name:      parameter.name
							namespace: parameter.namespace
						}
					}
				}
			}
<<<<<<< HEAD
			template: configMap.$returns.value.data["application"]
			apply:    kube.#Apply & {
				$params: value: yaml.Unmarshal(template)
||||||| parent of c9243126b (removes the unnecessary file changes and update component validation webhook logic)
			template: configMap.value.data["application"]
			apply: op.#Apply & {
				value: yaml.Unmarshal(template)
=======
			template: configMap.value.data["application"]
			apply:    op.#Apply & {
				value: yaml.Unmarshal(template)
>>>>>>> c9243126b (removes the unnecessary file changes and update component validation webhook logic)
			}
			wait: builtin.#ConditionalWait & {
				$params: continue: apply.$returns.value.status.status == "running"
			}
		}

		if dependsOn.$returns.err == _|_ {
			wait: builtin.#ConditionalWait & {
				$params: continue: dependsOn.$returns.value.status.status == "running"
			}
		}
	}
	parameter: {
		// +usage=Specify the name of the dependent Application
		name: string
		// +usage=Specify the namespace of the dependent Application
		namespace: string
	}
}
