"apply-once": {
	annotations: {}
	description: "Allow configuration drift for applied resources, delivery the resource without continuously reconciliation."
	labels: {}
	attributes: {}
	type: "policy"
}

template: {
	#ApplyOnceStrategy: {
		// +usage=When the strategy takes effect,e.g. onUpdate、onStateKeep
		affect?: string
		// +usage=Specify the path of the resource that allow configuration drift
		path: [...string]
	}

	#ApplyOncePolicyRule: {
		// +usage=Specify how to select the targets of the rule
		selector?: #ResourcePolicyRuleSelector
		// +usage=Specify the strategy for configuring the resource level configuration drift behaviour
		strategy: #ApplyOnceStrategy
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
		// +usage=Whether to enable apply-once for the whole application
		enable: *false | bool
		// +usage=Specify the rules for configuring apply-once policy in resource level
		rules?: [...#ApplyOncePolicyRule]
	}
}
