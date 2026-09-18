import "vela/kube"

"vela-env": {
	type: "source"
	annotations: {}
	labels: {}
	description: "The KubeVela environment an Application is deployed into - its name, namespace, whether the namespace is a managed env, and the labels and annotations a platform keeps there. Reads the Application's own namespace only."
}

template: {
	schema: {
		name:      string
		namespace: string
		managed:   bool
		labels: [string]:      string
		annotations: [string]: string
	}

	storage: {
		storageTTL:     "5m"
		onStaleFailure: "use-stale"
	}

	parameter: {}

	_ns: kube.#Get & {
		$params: resource: {
			apiVersion: "v1"
			kind:       "Namespace"
			metadata: name: context.namespace
		}
	}

	_labels: *_ns.$returns.metadata.labels | {}

	output: {
		name:      *_labels["namespace.oam.dev/env"] | context.namespace
		namespace: context.namespace
		// A one-element list indexed at 0 is CUE's "first case that holds"; a bare
		// `x != _|_` is not an expression CUE evaluates to a bool outside a
		// comprehension.
		managed: [
			if _labels["namespace.oam.dev/env"] != _|_ {true},
			false,
		][0]
		labels: _labels
		annotations: *_ns.$returns.metadata.annotations | {}
	}
}
