import (
	"vela/op"
)

"deploy-cloud-resource": {
	type: "workflow-step"
	annotations: {
		"category": "Application Delivery"
	}
	labels: {
		"scope": "Application"
	}
	description: "Deploy cloud resource and deliver secret to multi clusters."
}
template: {
	app: op.#DeployCloudResource & {
		env:    parameter.env
		policy: parameter.policy
		// context.namespace indicates the namespace of the app
		namespace: context.namespace
		// context.namespace indicates the name of the app
		name: context.name
	}

	parameter: {
		// +usage=Declare the name of the env-binding policy, if empty, the first env-binding policy will be used
		policy: *"" | string
		// +usage=Declare the name of the env in policy
		env: string
	}
}
