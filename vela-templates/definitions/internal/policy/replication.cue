"replication": {
	annotations: {}
	description: "Describe the configuration to replicate components when deploying resources, it only works with specified `deploy` step in workflow."
	labels: {}
	attributes: {}
	type: "policy"
}

template: {
	parameter: {
		// +usage=Spicify the keys of replication. Every key coresponds to a replication components
		keys: [...string]
		// +usage=Specify the components which will be replicated.
		selector?: [...string]
	}
}
