#Template: {
	apiVersion: "standard.oam.dev/v1alpha1"
	kind:       "Route"
	spec: {
		host: route.domain
		path: route.path
		backend: {
			port: route.port
		}
	}
}
route: {
	domain: string
	path:   *"" | string
	port:   *443 | int
}
