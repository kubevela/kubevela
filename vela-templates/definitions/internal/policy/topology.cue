"topology": {
	annotations: {}
	description: "Describe the destination where components should be deployed to."
	labels: {}
	attributes: {}
	type: "policy"
}

template: {
	parameter: {
		// +usage=Specify the names of the clusters to select.
		clusters?: [...string]
		// +usage=Specify the label selector for clusters
		clusterLabelSelector?: [string]: string
		// +usage=Ignore empty cluster error
		allowEmpty?: bool
		// +usage=Custom provider to find clusters
		customProvider?: #CustomProvider
		// +usage=Deprecated: Use clusterLabelSelector instead.
		clusterSelector?: [string]: string
		// +usage=Specify the target namespace to deploy in the selected clusters, default inherit the original namespace.
		namespace?: string
	}

	#CustomProvider: {
		// +usage=Specify the type of placement-loader definition to load
		type: string
		// +usage=The properties to compute
		properties?: {...}
	}
}
