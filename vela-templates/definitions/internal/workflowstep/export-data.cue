import (
	"vela/op"
)

"export-data": {
	type: "workflow-step"
	annotations: {
		"category": "Application Delivery"
	}
	labels: {
		"scope": "Application"
	}
	description: "Export data to clusters specified by topology."
}
template: {
	object: {
		apiVersion: "v1"
		kind:       parameter.kind
		metadata: {
			name:      *context.name | string
			namespace: *context.namespace | string
			if parameter.name != _|_ {
				name: parameter.name
			}
			if parameter.namespace != _|_ {
				namespace: parameter.namespace
			}
		}
		if parameter.kind == "ConfigMap" {
			data: parameter.data
		}
		if parameter.kind == "Secret" {
			stringData: parameter.data
		}
	} @step(1)

	getPlacements: op.#GetPlacementsFromTopologyPolicies & {
		policies: *[] | [...string]
		if parameter.topology != _|_ {
			policies: [parameter.topology]
		}
	} @step(2)

	apply: op.#Steps & {
		for p in getPlacements.placements {
			(p.cluster): op.#Apply & {
				value:   object
				cluster: p.cluster
			}
		}
	} @step(3)

	parameter: {
		// +usage=Specify the name of the export destination
		name?: string
		// +usage=Specify the namespace of the export destination
		namespace?: string
		// +usage=Specify the kind of the export destination
		kind: *"ConfigMap" | "Secret"
		// +usage=Specify the data to export
		data: {}
		// +usage=Specify the topology to export
		topology?: string
	}
}
