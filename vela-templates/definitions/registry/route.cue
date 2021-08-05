route: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Configures external access to your service."
	attributes: {
		appliesToWorkloads: ["deployments.apps"]
		podDisruptive: false
		definitionRef: name: "routes.standard.oam.dev"
		extension: install: helm: {
			repo:    "stable"
			name:    "nginx-ingress"
			url:     "https://kubernetes-charts.storage.googleapis.com/"
			version: "1.41.2"
		}
	}
}
template: {
	outputs: route: {
		apiVersion: "standard.oam.dev/v1alpha1"
		kind:       "Route"
		spec: {
			host: parameter.domain

			if parameter.issuer != "" {
				tls: issuerName: parameter.issuer
			}

			if parameter["rules"] != _|_ {
				rules: parameter.rules
			}

			provider:     *"nginx" | parameter.provider
			ingressClass: *"nginx" | parameter.ingressClass
		}
	}
	parameter: {
		// +usage= Domain name
		domain: *"" | string

		issuer: *"" | string
		rules?: [...{
			path:          string
			rewriteTarget: *"" | string
		}]
		provider?:     string
		ingressClass?: string
	}
}
