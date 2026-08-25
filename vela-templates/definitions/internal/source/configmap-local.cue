import "vela/kube"

"configmap-local": {
	type: "source"
	annotations: {}
	labels: {}
	description: "Reads a ConfigMap's data from the Application's own namespace on the cluster being rendered for. Values are strings, as Kubernetes stores them."
}

template: {
	schema: {
		data: [string]: string
	}

	storage: {
		storageTTL:     "5m"
		onStaleFailure: "use-stale"
	}

	parameter: {
		// +usage=Name of the ConfigMap, read from the Application's own namespace
		name: string
	}

	_cm: kube.#Get & {
		$params: {
			cluster: context.cluster
			resource: {
				apiVersion: "v1"
				kind:       "ConfigMap"
				metadata: {
					name:      parameter.name
					namespace: context.namespace
				}
			}
		}
	}

	output: data: _cm.$returns.data
}
