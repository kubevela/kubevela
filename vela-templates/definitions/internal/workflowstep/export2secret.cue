import (
	"vela/op"
	"encoding/base64"
	"encoding/json"
)

"export2secret": {
	type: "workflow-step"
	annotations: {
		"category": "Resource Management"
	}
	description: "Export data to Kubernetes Secret in your workflow."
}
template: {
	secret: op.#Steps & {
		data: *parameter.data | {}
		if parameter.kind == "docker-registry" && parameter.dockerRegistry != _|_ {
			registryData: {
				auths: {
					"\(parameter.dockerRegistry.server)": {
						username: parameter.dockerRegistry.username
						password: parameter.dockerRegistry.password
						auth:     base64.Encode(null, "\(parameter.dockerRegistry.username):\(parameter.dockerRegistry.password)")
					}
				}
			}
			data: {
				".dockerconfigjson": json.Marshal(registryData)
			}
		}
		apply: op.#Apply & {
			value: {
				apiVersion: "v1"
				kind:       "Secret"
				if parameter.type == _|_ && parameter.kind == "docker-registry" {
					type: "kubernetes.io/dockerconfigjson"
				}
				if parameter.type != _|_ {
					type: parameter.type
				}
				metadata: {
					name: parameter.secretName
					if parameter.namespace != _|_ {
						namespace: parameter.namespace
					}
					if parameter.namespace == _|_ {
						namespace: context.namespace
					}
				}
				stringData: data
			}
			cluster: parameter.cluster
		}
	}
	parameter: {
		// +usage=Specify the name of the secret
		secretName: string
		// +usage=Specify the namespace of the secret
		namespace?: string
		// +usage=Specify the type of the secret
		type?: string
		// +usage=Specify the data of secret
		data: {}
		// +usage=Specify the cluster of the secret
		cluster: *"" | string
		// +usage=Specify the kind of the secret
		kind: *"generic" | "docker-registry"
		// +usage=Specify the docker data
		dockerRegistry?: {
			// +usage=Specify the username of the docker registry
			username: string
			// +usage=Specify the password of the docker registry
			password: string
			// +usage=Specify the server of the docker registry
			server: *"https://index.docker.io/v1/" | string
		}
	}
}
