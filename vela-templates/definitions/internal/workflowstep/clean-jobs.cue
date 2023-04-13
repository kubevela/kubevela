import (
	"vela/op"
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
		labelSelector?: {...}
		namespace: *context.namespace | string
	}

	cleanJobs: op.#Delete & {
		value: {
			apiVersion: "batch/v1"
			kind:       "Job"
			metadata: {
				name:      context.name
				namespace: parameter.namespace
			}
		}
		filter: {
			namespace: parameter.namespace
			if parameter.labelSelector != _|_ {
				matchingLabels: parameter.labelSelector
			}
			if parameter.labelSelector == _|_ {
				matchingLabels: {
					"workflow.oam.dev/name": context.name
				}
			}
		}
	}

	cleanPods: op.#Delete & {
		value: {
			apiVersion: "v1"
			kind:       "pod"
			metadata: {
				name:      context.name
				namespace: parameter.namespace
			}
		}
		filter: {
			namespace: parameter.namespace
			if parameter.labelSelector != _|_ {
				matchingLabels: parameter.labelSelector
			}
			if parameter.labelSelector == _|_ {
				matchingLabels: {
					"workflow.oam.dev/name": context.name
				}
			}
		}
	}
}
