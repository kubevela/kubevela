#Template: {
	apiVersion: "networking.k8s.io/v1beta1"
	kind:       "Ingress"
	spec: {
		rules: [{
			host: route.domain
			http: paths: [{
				backend: {
					serviceName: route.service
					servicePort: route.port
				}}]
		}]
	}
}
route: {
	domain:  string
	port:    *80 | int
	service: string
}
