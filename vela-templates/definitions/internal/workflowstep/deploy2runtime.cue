import (
	"vela/op"
)

"deploy2runtime": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden":  "true"
		"deprecated": "true"
		"scope":      "Application"
	}
	description: "Deploy application to runtime clusters"
}
template: {
	app: op.#Steps & {
		load: op.#Load
		clusters: [...string]
		if parameter.clusters == _|_ {
			listClusters: op.#ListClusters
			clusters:     listClusters.outputs.clusters
		}
		if parameter.clusters != _|_ {
			clusters: parameter.clusters
		}

		apply: op.#Steps & {
			for _, cluster_ in clusters {
				for name, c in load.value {
					"\(cluster_)-\(name)": op.#ApplyComponent & {
						value:   c
						cluster: cluster_
					}
				}
			}
		}
	}

	parameter: {
		// +usage=Declare the runtime clusters to apply, if empty, all runtime clusters will be used
		clusters?: [...string]
	}
}
