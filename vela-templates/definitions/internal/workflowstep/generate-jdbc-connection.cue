import (
	"vela/op"
	"encoding/base64"
)

"generate-jdbc-connection": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Generate a JDBC connection based on Component of alibaba-rds"
}
template: {
	output: op.#Read & {
		value: {
			apiVersion: "v1"
			kind:       "Secret"
			metadata: {
				name: parameter.name
				if parameter.namespace != _|_ {
					namespace: parameter.namespace
				}
			}
		}
	}

	cluster: parameter.cluster

	dbHost: op.#ConvertString & {bt: base64.Decode(null, output.value.data["DB_HOST"])}
  dbPort: op.#ConvertString & {bt: base64.Decode(null, output.value.data["DB_PORT"])}
	dbName: op.#ConvertString & {bt: base64.Decode(null, output.value.data["DB_NAME"])}
//	key: parameter.jdbcSecretKey
	jdbc: [{name: "jdbc", value: "jdbc://" + dbHost.str + ":" + dbPort.str + "/" + dbName.str}]

	parameter: {
		// +usage=Specify the name of the secret generated by alibaba-rds
		name: string
		// +usage=Specify the namespace of the secret generated by alibaba-rds
		namespace?: string
		// +usage=Specify the cluster of the object
		cluster: *"" | string
//		jdbcSecretKey: string
	}
}
