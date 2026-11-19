metadata: {
	name:        "nacos-config"
	alias:       "Nacos Config"
	description: "a config whose rendered content the controller publishes to Nacos"
}

template: {
	nacos: {
		// can not reference the parameter
		endpoint: {
			name:      "nacos"
			namespace: "vela-system"
		}
		format: "properties"
		// may reference the parameter
		metadata: {
			dataId: parameter.dataId
			group:  parameter.group
		}
		content: parameter.content
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      context.name
			namespace: context.namespace
		}
		stringData: {
			dataId: parameter.dataId
			group:  parameter.group
		}
	}
	parameter: {
		dataId: string
		group:  *"DEFAULT_GROUP" | string
		content: {
			mysqlHost: string
			mysqlPort: int
		}
	}
}
