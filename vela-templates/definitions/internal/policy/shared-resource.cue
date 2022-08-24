"shared-resource": {
	annotations: {}
	description: "Configure the resources to be sharable across applications."
	labels: {}
	attributes: {}
	type: "policy"
}

template: {
	#SharedResourcePolicyRule: {
		// +usage=Specify how to select the targets of the rule
		selector: [...#ResourcePolicyRuleSelector]
	}

	#ResourcePolicyRuleSelector: {
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
		// +usage=Specify the list of rules to control shared-resource strategy at resource level.
		// The selected resource will be sharable across applications. (That means multiple applications
		// can all read it without conflict, but only the first one can write it)
		rules?: [...#SharedResourcePolicyRule]
	}
}
