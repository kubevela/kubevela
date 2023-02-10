import (
	"vela/op"
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
	dependsOn: op.#Read & {
		value: {
			apiVersion: "core.oam.dev/v1beta1"
			kind:       "Application"
			metadata: {
				name:      parameter.name
				namespace: parameter.namespace
			}
		}
	}
	load: op.#Steps & {
		if dependsOn.err != _|_ {
			configMap: op.#Read & {
				value: {
					apiVersion: "v1"
					kind:       "ConfigMap"
					metadata: {
						name:      parameter.name
						namespace: parameter.namespace
					}
				}
			}         @step(1)
			template: configMap.value.data["application"]
			apply:    op.#Apply & {
				value: yaml.Unmarshal(template)
			}     @step(2)
			wait: op.#ConditionalWait & {
				continue: apply.value.status.status == "running"
			} @step(3)
		}

		if dependsOn.err == _|_ {
			wait: op.#ConditionalWait & {
				continue: dependsOn.value.status.status == "running"
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
