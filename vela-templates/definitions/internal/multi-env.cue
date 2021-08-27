import (
	"vela/op"
)

"multi-env": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Apply env binding component"
}
template: {
	app: op.#ApplyEnvBindApp & {
		env:    parameter.env
		policy: parameter.policy
		app:    context.name
		// context.namespace indicates the namespace of the app
		namespace: context.namespace
	}

	parameter: {
		// +usage=Declare the name of the policy
		policy: string
		// +usage=Declare the name of the env in policy
		env: string
	}
}
