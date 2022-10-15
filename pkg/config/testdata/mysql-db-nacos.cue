metadata: {
	name:  "nacos"
	alias: "Nacos Config"
}

template: {
	nacos: {
		// can not references the parameter
		endpoint: {
			name:      "nacos"
			namespace: "default"
		}
		format: "properties"

		// could references the parameter
		metadata: {
			dataId: parameter.dataId
			group:  parameter.group
			if parameter.appName != _|_ {
				appName: parameter.appName
			}
		}
		content: parameter.content
	}
	outputs: {
		"test": {
			kind:       "ConfigMap"
			apiVersion: "v1"
			metadata: {
				name:      context.name
				namespace: context.namespace
			}
			data: {
				"string": "string"
			}
		}
	}
	parameter: {
		dataId:   string
		group:    *"DEFAULT_GROUP" | string
		appName?: string
		content: {
			mysqlHost: string
			mysqlPort: int
			username?: string
			password?: string
		}
	}
}
