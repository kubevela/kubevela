import (
	"vela/op"
)

"deploy": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Deploy components with policies."
}
template: {
	deploy: op.#Deploy & {
		policies:                 parameter.policies
		parallelism:              parameter.parallelism
		ignoreTerraformComponent: parameter.ignoreTerraformComponent
	}
	parameter: {
		//+usage=If set false, the workflow will be suspend before this step.
		auto: *true | bool
		//+usage=Declare the policies used for this step.
		policies?: [...string]
		//+usage=Maximum number of concurrent delivered components.
		parallelism: *5 | int
		//+usage=If set false, this step will apply the components with the terraform workload.
		ignoreTerraformComponent: *true | bool
	}
}
