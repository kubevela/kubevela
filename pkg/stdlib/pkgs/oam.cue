#ApplyComponent: {
	#provider:   "oam"
	#do:         "component-apply"
	cluster:     *"" | string
	env:         *"" | string
	namespace:   *"" | string
	waitHealthy: *true | bool
	value: {...}
	patch?: {...}
	...
}

#ApplyComponents: {
	#provider:   "oam"
	#do:         "components-apply"
	parallelism: *5 | int
	components: [string]: {
		cluster:     *"" | string
		env:         *"" | string
		namespace:   *"" | string
		waitHealthy: *true | bool
		value: {...}
		patch?: {...}
		...
	}
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
	...
}

#LoadPolicies: {
	#provider: "oam"
	#do:       "load-policies"
	value?: {...}
	...
}

#LoadPoliciesInOrder: {
	#provider: "oam"
	#do:       "load-policies-in-order"
	input?: [...string]
	output?: [...{...}]
	...
}

#LoadComponetsInOrder: {
	#provider: "oam"
	#do:       "load-comps-in-order"
	...
}
