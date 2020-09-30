data: {
	apiVersion: "networking.k8s.io/v1beta1"
	kind:       "Ingress"
	spec: {
		rules: [ for _, r in parameter.rules {
			host: r.domain
			http: [ for _, p in r.paths {
				backend: {
					serviceName: p.service
					servicePort: p.port
				}
			}]
		}]
	}
}
#route: {
	rules: [ ...{
		domain: string
		paths: [ ... {
			service: string
			port:    int16
		}]
	}]
}
parameter: #route
