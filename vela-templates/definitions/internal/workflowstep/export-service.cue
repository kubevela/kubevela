import (
	"vela/op"
)

"export-service": {
	type: "workflow-step"
	annotations: {
		"category": "Application Delivery"
	}
	labels: {
		"scope": "Application"
	}
	description: "Export service to clusters specified by topology."
}
template: {
	meta: {
		name:      *context.name | string
		namespace: *context.namespace | string
		if parameter.name != _|_ {
			name: parameter.name
		}
		if parameter.namespace != _|_ {
			namespace: parameter.namespace
		}
	}
	objects: [{
		apiVersion: "v1"
		kind:       "Service"
		metadata:   meta
		spec: {
			type: "ClusterIP"
			ports: [{
				protocol:   "TCP"
				port:       parameter.port
				targetPort: parameter.targetPort
			}]
		}
	}, {
		apiVersion: "v1"
		kind:       "Endpoints"
		metadata:   meta
		subsets: [{
			addresses: [{ip: parameter.ip}]
			ports: [{port: parameter.targetPort}]
		}]
	}] @step(1)

	getPlacements: op.#GetPlacementsFromTopologyPolicies & {
		policies: *[] | [...string]
		if parameter.topology != _|_ {
			policies: [parameter.topology]
		}
	} @step(2)

	apply: op.#Steps & {
		for p in getPlacements.placements {
			for o in objects {
				"\(p.cluster)-\(o.kind)": op.#Apply & {
					value:   o
					cluster: p.cluster
				}
			}
		}
	} @step(3)

	parameter: {
		// +usage=Specify the name of the export destination
		name?: string
		// +usage=Specify the namespace of the export destination
		namespace?: string
		// +usage=Specify the ip to be export
		ip: string
		// +usage=Specify the port to be used in service
		port: int
		// +usage=Specify the port to be export
		targetPort: int
		// +usage=Specify the topology to export
		topology?: string
	}
}
