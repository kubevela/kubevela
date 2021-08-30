import (
	"vela/op"
	"encoding/yaml"
)

"depends-on-app": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "check or install depends-on Application"
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
			}
			apply: op.#Apply & {
				value: {
					yaml.Unmarshal(configMap.value.data[parameter.name])
				}
			}
		}
		if dependsOn.err == _|_ {
			apply: op.#Apply & {
				value: {
					dependsOn.value
				}
			}
		}
	}

	phase: load.apply.value.status.status

	wait: op.#ConditionalWait & {
		continue: phase == "running"
	}

	parameter: {
		// +usage=Specify the name of the dependent Application
		name: string
		// +usage=Specify the namespace of the dependent Application
		namespace: string
	}
}
