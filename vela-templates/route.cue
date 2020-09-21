#Template: {
	apiVersion: "standard.oam.dev/v1alpha1"
	kind:       "Route"
	spec: {
		host: route.domain
		path: route.path
		tls: {
			issuerName: route.issuer
		}
		backend: {
			port: route.port
		}
	}
}
route: {
	domain: *"" | string
	path:   *"" | string
	port:   *443 | int
	issuer: *"" | string
}
