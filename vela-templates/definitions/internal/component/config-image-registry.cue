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
			name:      context.name
			namespace: context.namespace
			labels: {
				"config.oam.dev/catalog":       "velacore-config"
				"config.oam.dev/type":          "image-registry"
				"config.oam.dev/multi-cluster": "true"
				"config.oam.dev/identifier":    parameter.registry
				"config.oam.dev/sub-type":      "auth"
			}
		}
		if parameter.auth != _|_ {
			type: "kubernetes.io/dockerconfigjson"
		}
		if parameter.auth == _|_ {
			type: "Opaque"
		}
		if parameter.auth != _|_ {
			stringData: {
				".dockerconfigjson": json.Marshal({
					"auths": "\(parameter.registry)": {
						"username": parameter.auth.username
						"password": parameter.auth.password
						if parameter.auth.email != _|_ {
							"email": parameter.auth.email
						}
						"auth": base64.Encode(null, (parameter.auth.username + ":" + parameter.auth.password))
					}
				})
			}
		}
	}

	parameter: {
		// +usage=Image registry FQDN
		registry: string
		// +usage=Authenticate the image registry
		auth?: {
			// +usage=Private Image registry username
			username: string
			// +usage=Private Image registry password
			password: string
			// +usage=Private Image registry email
			email?: string
		}
	}
}
