import (
	"encoding/base64"
	"encoding/json"
)

"config-image-registry": {
	type: "component"
	annotations: {
		"alias.config.oam.dev": "Image Registry"
	}
	labels: {
		"catalog.config.oam.dev":       "velacore-config"
		"type.config.oam.dev":          "image-registry"
		"multi-cluster.config.oam.dev": "true"
	}
	description: "Config information to authenticate image registry"
	attributes: workload: type: "autodetects.core.oam.dev"
}

template: {
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      parameter.name
			namespace: context.namespace
			labels: {
				"config.oam.dev/catalog":       "velacore-config"
				"config.oam.dev/type":          "image-registry"
				"config.oam.dev/multi-cluster": "true"
				"config.oam.dev/identifier":    parameter.registry
			}
		}
		type: "kubernetes.io/dockerconfigjson"
		stringData: {
			if parameter.type == "account" {
				".dockerconfigjson": json.Marshal({
					"auths": "\(parameter.registry)": {
						"username": parameter.username
						"password": parameter.password
						if parameter.email != _|_ {
							"email": parameter.email
						}
						"auth": base64.Encode(null, (parameter.username + ":" + parameter.password))
					}
				})
			}
		}
	}

	parameter: {
		// +usage=Config name
		name: string
		// +usage=Private Image registry FQDN
		registry: string
		// +usage=Config type
		type: "account"
		auth ?: {
			// +usage=Private Image registry username
			username: string
			// +usage=Private Image registry password
			password: string
			// +usage=Private Image registry email
			email?: string
		}
	}
}
