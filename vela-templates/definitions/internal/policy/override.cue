"override": {
	annotations: {}
	description: "Override configuration when deploying resources"
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
		properties?: {...}
		traits?: [...{
			type: string
			properties?: {...}
			// +usage=Specify if the trait shoued be remove, default false
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
