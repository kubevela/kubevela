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
	// An Application has no status.status until it is first reconciled, and the step waits for it.
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
			template: configMap.$returns.value.data["application"]
			apply: kube.#Apply & {
				$params: value: yaml.Unmarshal(template)
			}
			wait: builtin.#ConditionalWait & {
				$params: continue: [if apply.$returns.value.status.status != _|_ {apply.$returns.value.status.status == "running"}, false][0]
			}
		}

		if dependsOn.$returns.err == _|_ {
			wait: builtin.#ConditionalWait & {
				$params: continue: [if dependsOn.$returns.value.status.status != _|_ {dependsOn.$returns.value.status.status == "running"}, false][0]
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
