#ApplyComponent: {
	#provider: "oam"
	#do:       "component-apply"

	// +usage=The cluster to use
	cluster: *"" | string
	// +usage=The env to use
	env: *"" | string
	// +usage=The namespace to apply
	namespace: *"" | string
	// +usage=Whether to wait healthy of the applied component
	waitHealthy: *true | bool
	// +usage=The value of the component resource
	value: {...}
	// +usage=The patcher that will be applied to the resource, you can define the strategy of list merge through comments. Reference doc here: https://kubevela.io/docs/platform-engineers/traits/patch-trait#patch-in-workflow-step
	patch?: {...}
	...
}

#RenderComponent: {
	#provider: "oam"
	#do:       "component-render"
	cluster:   *"" | string
	env:       *"" | string
	namespace: *"" | string
	value: {...}
	patch?: {...}
	output?: {...}
	outputs?: {...}
	...
}

#LoadComponets: {
	#provider: "oam"
	#do:       "load"

	// +usage=If specify `app`, use specified application to load its component resources otherwise use current application
	app?: string
	// +usage=The value of the components will be filled in this field after the action is executed, you can use value[componentName] to refer a specified component
	value?: {...}
	...
}

#LoadPolicies: {
	#provider: "oam"
	#do:       "load-policies"
	value?: {...}
	...
}

#LoadComponetsInOrder: {
	#provider: "oam"
	#do:       "load-comps-in-order"
	...
}
