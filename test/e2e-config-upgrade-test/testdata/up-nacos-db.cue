metadata: {
	name:        "up-nacos-db"
	alias:       "up-nacos-db"
	description: "config upgrade e2e"
	scope:       "namespace"
	sensitive:   false
}
template: {
	nacos: {
		endpoint: {
			name:      "up-nacos"
			namespace: "vela-system"
		}
		format: "properties"
		metadata: {
			dataId: parameter.dataId
			group:  "UPGRADE_E2E"
		}
		content: parameter.content
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {name: context.name, namespace: context.namespace}
		stringData: dataId: parameter.dataId
	}
	parameter: {
		dataId: string
		content: {
			mysqlHost: string
			mysqlPort: int
		}
	}
}
