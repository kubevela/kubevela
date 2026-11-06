import (
	"vela/kube"
)

"clean-jobs": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	annotations: {
		"category": "Resource Management"
	}
	description: "clean applied jobs in the cluster"
}
template: {

	parameter: {
		labelselector?: {...}
		namespace: *context.namespace | string
	}

	// Interpolating resolves the parameter's default before it meets kube.#Delete's own default.
	let targetNamespace = "\(parameter.namespace)"

	cleanJobs: kube.#Delete & {
		$params: {
			value: {
				apiVersion: "batch/v1"
				kind:       "Job"
				metadata: {
					name:      context.name
					namespace: targetNamespace
				}
			}
			filter: {
				namespace: targetNamespace
				if parameter.labelselector != _|_ {
					matchingLabels: parameter.labelselector
				}
				if parameter.labelselector == _|_ {
					matchingLabels: {
						"workflow.oam.dev/name": context.name
					}
				}
			}
		}
	}

	cleanPods: kube.#Delete & {
		$params: {
			value: {
				apiVersion: "v1"
				kind:       "pod"
				metadata: {
					name:      context.name
					namespace: targetNamespace
				}
			}
			filter: {
				namespace: targetNamespace
				if parameter.labelselector != _|_ {
					matchingLabels: parameter.labelselector
				}
				if parameter.labelselector == _|_ {
					matchingLabels: {
						"workflow.oam.dev/name": context.name
					}
				}
			}
		}
	}
}
