import (
	"vela/op"
)

"deploy": {
	type: "workflow-step"
	annotations: {
		"category": "Application Delivery"
	}
	labels: {
		"scope": "Application"
	}
	description: "A powerful and unified deploy step for components multi-cluster delivery with policies."
}
template: {
	if parameter.auto == false {
		suspend: op.#Suspend & {message: "Waiting approval to the deploy step \"\(context.stepName)\""}
	}
	deploy: op.#Deploy & {
		policies:                 parameter.policies
		parallelism:              parameter.parallelism
		ignoreTerraformComponent: parameter.ignoreTerraformComponent
	}
	parameter: {
		//+usage=If set to false, the workflow will suspend automatically before this step, default to be true.
		auto: *true | bool
		//+usage=Declare the policies that used for this deployment. If not specified, the components will be deployed to the hub cluster.
		policies: *[] | [...string]
		//+usage=Maximum number of concurrent delivered components.
		parallelism: *5 | int
		//+usage=If set false, this step will apply the components with the terraform workload.
		ignoreTerraformComponent: *true | bool
	}
}
