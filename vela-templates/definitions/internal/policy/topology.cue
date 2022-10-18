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
		// +usage=Deprecated: Use clusterLabelSelector instead.
		clusterSelector?: [string]: string
		// +usage=Specify the target namespace to deploy in the selected clusters, default inherit the original namespace.
		namespace?: string
	}
}
