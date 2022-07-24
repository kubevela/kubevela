import (
	"vela/op"
)

"deploy2runtime": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden":  "true"
		"deprecated": "true"
	}
	description: "Deploy application to runtime clusters"
}
template: {
	app: op.#Steps & {
		load: op.#Load @step(1)
		clusters: [...string]
		if parameter.clusters == _|_ {
			listClusters: op.#ListClusters @step(2)
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
		} @step(3)
	}

	parameter: {
		// +usage=Declare the runtime clusters to apply, if empty, all runtime clusters will be used
		clusters?: [...string]
	}
}
