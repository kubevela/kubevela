import (
	"vela/op"
)

"deploy2env": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden":  "true"
		"deprecated": "true"
		"scope":      "Application"
	}
	description: "Deploy env binding component to target env"
}
template: {
	app: op.#ApplyEnvBindApp & {
		env:      parameter.env
		policy:   parameter.policy
		parallel: parameter.parallel
		app:      context.name
		// context.namespace indicates the namespace of the app
		namespace: context.namespace
	}

	parameter: {
		// +usage=Declare the name of the env-binding policy, if empty, the first env-binding policy will be used
		policy: *"" | string
		// +usage=Declare the name of the env in policy
		env: string
		// +usage=components are applied in parallel
		parallel: *false | bool
	}
}
