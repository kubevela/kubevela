import (
	"strconv"
	"strings"
	"vela/op"
)

"apply-deployment": {
	alias: ""
	annotations: {}
	attributes: {}
	description: "Apply deployment with specified image and cmd."
	labels: {}
	type: "workflow-step"
}

template: {
	output: op.#Apply & {
		value: {
			apiVersion: "apps/v1"
			kind:       "Deployment"
			metadata: {
				name:      context.stepName
				namespace: context.namespace
			}
			spec: {
				selector: matchLabels: "workflow.oam.dev/step-name": "\(context.name)-\(context.stepName)"
				template: {
					metadata: labels: "workflow.oam.dev/step-name": "\(context.name)-\(context.stepName)"
					spec: containers: [{
						name:  context.stepName
						image: parameter.image
						if parameter["cmd"] != _|_ {
							command: parameter.cmd
						}
					}]
				}
			}
		}
	}
	wait: op.#ConditionalWait & {
		continue: output.value.status.readyReplicas == 1
	}
	parameter: {
		image: string
		cmd?: [...string]
	}
}
