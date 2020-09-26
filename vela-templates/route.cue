data: {
	apiVersion: "standard.oam.dev/v1alpha1"
	kind:       "Route"
	spec: {
		host: parameter.domain
		path: parameter.path
		tls: {
			issuerName: parameter.issuer
		}
		backend: {
			port: parameter.port
		}
	}
}
#route: {
	domain: *"" | string
	path:   *"" | string
	port:   *443 | int
	issuer: *"" | string
}
parameter: #route
