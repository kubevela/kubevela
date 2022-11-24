"take-over": {
	annotations: {}
	description: "Configure the resources to be able to take over when it belongs to no application."
	labels: {}
	attributes: {}
	type: "policy"
}

template: {
	#PolicyRule: {
		// +usage=Specify how to select the targets of the rule
		selector: [...#RuleSelector]
	}

	#RuleSelector: {
		// +usage=Select resources by component names
		componentNames?: [...string]
		// +usage=Select resources by component types
		componentTypes?: [...string]
		// +usage=Select resources by oamTypes (COMPONENT or TRAIT)
		oamTypes?: [...string]
		// +usage=Select resources by trait types
		traitTypes?: [...string]
		// +usage=Select resources by resource types (like Deployment)
		resourceTypes?: [...string]
		// +usage=Select resources by their names
		resourceNames?: [...string]
	}

	parameter: {
		// +usage=Specify the list of rules to control take over strategy at resource level.
		// The selected resource will be able to be taken over by the current application when the resource belongs to no
		// one.
		rules?: [...#PolicyRule]
	}
}
