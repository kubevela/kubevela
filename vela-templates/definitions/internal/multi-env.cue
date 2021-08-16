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
	component: op.#ApplyEnvBindComponent & {
		env:       parameter.env
		policy:    context.name + "-" + parameter.policy
		component: parameter.component
		// context.namespace indicates the namespace of the app
		namespace: context.namespace
	}

	parameter: {
		// +usage=Declare the name of the component
		component: string
		// +usage=Declare the name of the policy
		policy: string
		// +usage=Declare the name of the env in policy
		env: string
	}
}
