import (
	"vela/op"
)

"deploy2env": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Deploy env binding component to target env"
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
