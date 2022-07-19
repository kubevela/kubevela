"garbage-collect": {
	annotations: {}
	description: "Configure the garbage collect behaviour for the application."
	labels: {}
	attributes: {}
	type: "policy"
}

template: {
	#GarbageCollectPolicyRule: {
		// +usage=Specify how to select the targets of the rule
		selector: [...#ResourcePolicyRuleSelector]
		// +usage=Specify the strategy for target resource to recycle
		strategy: *"onAppUpdate" | "onAppDelete" | "never"
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
		// +usage=If is set, outdated versioned resourcetracker will not be recycled automatically, outdated resources will be kept until resourcetracker be deleted manually
		keepLegacyResource: *false | bool
		// +usage=Specify the list of rules to control gc strategy at resource level, if one resource is controlled by multiple rules, first rule will be used
		rules?: [...#GarbageCollectPolicyRule]
	}
}
