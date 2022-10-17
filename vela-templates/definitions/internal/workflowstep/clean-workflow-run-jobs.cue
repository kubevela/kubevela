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

	clean: op.#Delete & {
		value: {
			apiVersion: "batch/v1"
			kind:       "Job"
			metadata: {
				namespace: "vela-system"
			}
		}
		filter: {
			namespace: "vela-system"
			matchingLabels: {
				"workflowrun.oam.dev/name":      context.name
				"workflowrun.oam.dev/namespace": context.namespace
			}
		}
	}
}