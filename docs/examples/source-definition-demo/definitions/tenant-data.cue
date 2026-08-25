import "vela/kube"

"tenant-data": {
	type: "source"
	annotations: {}
	labels: {}
	description: "Reads per-namespace tenant metadata (name, department, environment, optional cost center) from namespace labels."
}
template: {
	schema: {
		name:        string
		department:  string
		environment: string
		costCenter?: string
	}

	// Cache per namespace on the current cluster.
	storage: {
		storageTTL:     "10m"
		onStaleFailure: "use-stale"
	}

	parameter: {}

	// Read the Application's own namespace object.
	_ns: kube.#Get & {
		$params: {
			cluster: context.cluster
			resource: {
				apiVersion: "v1"
				kind:       "Namespace"
				metadata: name: context.namespace
			}
		}
	}

	_labels: _ns.$returns.metadata.labels

	// Required labels are enforced by consumption (incomplete -> Failed).
	// costCenter is optional: only surfaced when the label is present.
	output: {
		name:        _labels["tenant.example.com/name"]
		department:  _labels["tenant.example.com/department"]
		environment: _labels["tenant.example.com/environment"]
		if _labels["tenant.example.com/cost-center"] != _|_ {
			costCenter: _labels["tenant.example.com/cost-center"]
		}
	}
}
