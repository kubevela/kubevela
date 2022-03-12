"config-helm-repository": {
	type: "component"
	annotations: {
		"alias.config.oam.dev": "Helm Repository"
	}
	labels: {
		"catalog.config.oam.dev":       "velacore-config"
		"multi-cluster.config.oam.dev": "true"
		"type.config.oam.dev":          "helm-repository"
	}
	description: "Config information to authenticate helm chart repository"
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
				"config.oam.dev/type":          "helm-repository"
				"config.oam.dev/multi-cluster": "true"
				"config.oam.dev/identifier":    parameter.registry
			}
		}
		type: "Opaque"

		if parameter.https != _|_ {
			stringData: parameter.https
		}
		if parameter.ssh != _|_ {
			stringData: parameter.ssh
		}
	}

	parameter: {
		oss?: {
			bucket:   string
			endpoint: string
		}
		https?: {
			url:      string
			username: string
			password: string
		}
		// +usage=https://fluxcd.io/legacy/helm-operator/helmrelease-guide/chart-sources/#ssh
		ssh?: {
			url:      string
			identity: string
		}
	}
}
