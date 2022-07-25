"override": {
	annotations: {}
	description: "Describe the configuration to override when deploying resources, it only works with specified `deploy` step in workflow."
	labels: {}
	attributes: {}
	type: "policy"
}

template: {

	#PatchParams: {
		// +usage=Specify the name of the patch component, if empty, all components will be merged
		name?: string
		// +usage=Specify the type of the patch component.
		type?: string
		// +usage=Specify the properties to override.
		properties?: {...}
		// +usage=Specify the traits to override.
		traits?: [...{
			// +usage=Specify the type of the trait to be patched.
			type: string
			// +usage=Specify the properties to override.
			properties?: {...}
			// +usage=Specify if the trait should be remove, default false
			disable: *false | bool
		}]
	}

	parameter: {
		// +usage=Specify the overridden component configuration.
		components: [...#PatchParams]
		// +usage=Specify a list of component names to use, if empty, all components will be selected.
		selector?: [...string]
	}
}
