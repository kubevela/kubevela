import (
	"vela/op"
)

"clean-workflow-run-jobs": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "clean addon enable jobs"
}
template: {

	parameter: {
		labelselector?: {...}
	}

	clean: op.#Delete & {
		value: {
			apiVersion: "batch/v1"
			kind:       "Job"
			metadata: {
				name:      context.name
				namespace: context.namespace
			}
		}
		filter: {
			namespace: context.namespace
			if parameter.labelselector != _|_ {
				matchingLabels: parameter.labelselector
			}
			if parameter.labelselector == _|_ {
				matchingLabels: {
					"workflowrun.oam.dev/name": context.name
				}
			}
		}
	}
}
