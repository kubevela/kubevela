package main

// One topology policy places the whole addon, as catalog addons do. The
// renderer keeps the generated widget-kit component on the hub, and fromPolicy
// copies this policy's clusters onto it as the module's placement.
output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	spec: {
		components: []
		policies: [{
			name: "topo-runtime"
			type: "topology"
			properties: clusters: parameter.clusters
		}]
	}
}
