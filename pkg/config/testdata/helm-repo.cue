metadata: {
	name: "helm-repository"
	// alias:     "Helm Repository"
	scope:     "system"
	sensitive: false
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
				"config.oam.dev/type":          "helm-repository"
				"config.oam.dev/multi-cluster": "true"
				"config.oam.dev/sub-type":      "helm"
			}
		}
		// If the type is empty, it will assign value using this format.
		type: "catalog.config.oam.dev/helm-repository"
		stringData: {
			url: parameter.url
			if parameter.username != _|_ {
				username: parameter.username
			}
			if parameter.password != _|_ {
				password: parameter.password
			}
		}
		data: {
			if parameter.caFile != _|_ {
				caFile: parameter.caFile
			}
		}
	}
	parameter: {
		// +usage=The public url of the helm chart repository.
		url: string
		// +usage=The username of basic auth repo.
		username?: string
		// +usage=The password of basic auth repo.
		password?: string
		// +usage=The ca certificate of helm repository. Please encode this data with base64.
		caFile?: string
	}
}
