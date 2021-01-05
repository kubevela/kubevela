output: {
	apiVersion: "standard.oam.dev/v1alpha1"
	kind:       "Route"
	spec: {
		host: parameter.domain

		if parameter.issuer != "" {
			tls: {
				issuerName: parameter.issuer
			}
		}

		if parameter["rules"] != _|_ {
			rules: parameter.rules
		}

		provider: *"nginx" | parameter.provider
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
	provider: *"" | string
}
