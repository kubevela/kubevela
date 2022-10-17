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
					name:      parameter.addonName + "-enable-job"
					namespace: "vela-system"
					labels: {
						"enable-addon.oam.dev": context.name
					}
				}
				spec: {
					template: {
						metadata: {
							labels: {
								"workflowrun.oam.dev/name": context.name
                "workflowrun.oam.dev/namespace": context.namespace
                "workflowrun.oam.dev/step": context.stepName
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

		wait: op.#ConditionalWait & {
			continue: job.value.status.succeeded == 1
		}

    log: op.#Log & {
      source: {
         resources: [{labelSelector:{
         	  "workflowrun.oam.dev/name": context.name
            "workflowrun.oam.dev/namespace": context.namespace
            "workflowrun.oam.dev/step": context.stepName
         }}]
      }
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
