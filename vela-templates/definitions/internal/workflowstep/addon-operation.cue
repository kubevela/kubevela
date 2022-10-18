import (
	"vela/op"
)

"addon-operation": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Enable a KubeVela addon"
}
template: {

	job: op.#Apply & {
		value: {
			apiVersion: "batch/v1"
			kind:       "Job"
			metadata: {
				name:      context.name + "-" + context.stepName + "-" + context.stepSessionID + "-enable-addon-job"
				namespace: context.namespace
				labels: {
					"enable-addon.oam.dev": context.name
				}
			}
			spec: {
				backoffLimit: 0
				template: {
					metadata: {
						labels: {
							"workflowrun.oam.dev/name":    context.name
							"workflowrun.oam.dev/step":    context.stepName
							"workflowrun.oam.dev/session": context.stepSessionID
						}
					}
					spec: {
						containers: [
							{
								name:  parameter.addonName + "-enable-job"
								image: parameter.image

								if parameter.args == _|_ {
									command: ["vela", "addon", parameter.operation, parameter.addonName]
								}

								if parameter.args != _|_ {
									command: ["vela", "addon", parameter.operation, parameter.addonName] + parameter.args
								}
							},
						]
						restartPolicy:  "Never"
						serviceAccount: parameter.serviceAccountName
					}
				}
			}
		}
	}

	log: op.#Log & {
		source: {
			resources: [{labelSelector: {
				"workflowrun.oam.dev/name":    context.name
				"workflowrun.oam.dev/step":    context.stepName
				"workflowrun.oam.dev/session": context.stepSessionID
			}}]
		}
	}

	fail: op.#Steps & {
		if job.value.status.failed != _|_ {
			if job.value.status.failed > 0 {
				breakWorkflow: op.#Fail & {
					message: "enable addon failed"
				}
			}
		}
	}

	wait: op.#ConditionalWait & {
		continue: job.value.status.succeeded != _|_ && job.value.status.succeeded > 0
	}

	parameter: {
		// +usage=Specify the name of the addon.
		addonName: string
		// +usage=Specify addon enable args.
		args?: [...string]
		// +usage=Specify the image
		image: *"oamdev/vela-cli:v1.6.0-alpha.4" | string
		// +usage=operation for the addon
		operation: *"enable" | "upgrade" | "disable"
		// +usage=specify serviceAccountName want to use
		serviceAccountName: *"kubevela-vela-core" | string
	}
}
