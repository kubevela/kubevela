import "vela/kube"

"cluster-lookup": {
	type: "source"
	annotations: {}
	labels: {}
	description: "Reads cluster-wide facts (region, zone, provider) from the cluster-info ConfigMap."
}
template: {
	// Output contract: the fields any Application may read with $(source...).
	schema: {
		region:   string
		zone:     string
		provider: string
	}

	// One cluster-wide fact: a single cache entry per cluster, shared by
	// every Application on that cluster that reads it.
	storage: {
		storageTTL:     "15m"
		onStaleFailure: "use-stale"
	}

	// No author-supplied inputs; everything comes from the cluster read.
	parameter: {}

	// Explicitly read the cluster-info ConfigMap in the platform namespace.
	_clusterInfo: kube.#Get & {
		$params: {
			cluster: context.cluster
			resource: {
				apiVersion: "v1"
				kind:       "ConfigMap"
				metadata: {
					name:      "cluster-info"
					namespace: "kube-system"
				}
			}
		}
	}

	// Consuming each key in output makes it required: if a key is absent the
	// value is incomplete, resolution fails, and the source reports Failed.
	output: {
		region:   _clusterInfo.$returns.data["region"]
		zone:     _clusterInfo.$returns.data["zone"]
		provider: _clusterInfo.$returns.data["provider"]
	}
}
