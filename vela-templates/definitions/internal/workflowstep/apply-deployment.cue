import (
	"vela/kube"
	"vela/builtin"
)

"apply-deployment": {
	alias: ""
	annotations: {}
	attributes: {}
	description: "Apply deployment with specified image and cmd."
	annotations: {
		"category": "Resource Management"
	}
	labels: {}
	type: "workflow-step"
}

template: {
	output: kube.#Apply & {
		$params: {
			cluster: parameter.cluster
			value: {
				apiVersion: "apps/v1"
				kind:       "Deployment"
				metadata: {
					name:      context.stepName
					namespace: context.namespace
				}
				spec: {
					selector: matchLabels: "workflow.oam.dev/step-name": "\(context.name)-\(context.stepName)"
					replicas: parameter.replicas
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
	}
	// Kubernetes omits readyReplicas while it is zero.
	let readyReplicas = [if output.$returns.value.status.readyReplicas != _|_ {output.$returns.value.status.readyReplicas}, 0][0]
	wait: builtin.#ConditionalWait & {
		$params: continue: readyReplicas == parameter.replicas
	}
	parameter: {
		image:    string
		replicas: *1 | int
		cluster:  *"" | string
		cmd?: [...string]
	}
}
