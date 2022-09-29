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
