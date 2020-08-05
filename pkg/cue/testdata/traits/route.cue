#Template: {
	apiVersion: "apps/v1"
	kind:       "Route"
	spec: {
		domain: route.domain
	}
}

route: {
	domain: string
}
