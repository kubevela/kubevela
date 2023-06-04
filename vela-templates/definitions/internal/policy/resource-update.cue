"resource-update": {
	annotations: {}
	description: "Configure the update strategy for selected resources."
	labels: {}
	attributes: {}
	type: "policy"
}

template: {
	#PolicyRule: {
		// +usage=Specify how to select the targets of the rule
		selector: #RuleSelector
		// +usage=The update strategy for the target resources
		strategy: #Strategy
	}

	#Strategy: {
		// +usage=Specify the op for updating target resources
		op: *"patch" | "replace"
		// +usage=Specify which fields would trigger recreation when updated
		recreateFields?: [...string]
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
		// +usage=Specify the list of rules to control resource update strategy at resource level.
		rules?: [...#PolicyRule]
	}
}
