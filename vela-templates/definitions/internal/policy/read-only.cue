"read-only": {
	annotations: {}
	description: "Configure the resources to be read-only in the application (no update / state-keep)."
	labels: {}
	attributes: {}
	type: "policy"
}

template: {
	#PolicyRule: {
		// +usage=Specify how to select the targets of the rule
		selector: #RuleSelector
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
		// +usage=Specify the list of rules to control read only strategy at resource level.
		// The selected resource will be read-only to the current application. If the target resource does
		// not exist, error will be raised.
		rules?: [...#PolicyRule]
	}
}
