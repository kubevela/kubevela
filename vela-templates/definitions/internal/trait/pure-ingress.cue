"pure-ingress": {
	attributes: {
		appliesToWorkloads: ["*"]
		conflictsWith: []
		podDisruptive: false
		status: {
			customStatus: #"""
				let lb = context.outputs.ingress.status.loadBalancer
				if lb.ingress == _|_ {
					message: "No loadBalancer found, visiting by using 'vela port-forward " + context.appName + " --route'\n"
				}
				if lb.ingress != _|_ if len(lb.ingress) > 0 {
					let igs = lb.ingress
					let rules = context.outputs.ingress.spec.rules
					host: *"" | string
					if rules != _|_ if len(rules) > 0 if rules[0].host != _|_ {
						host: rules[0].host
					}
					if igs[0].ip != _|_ {
						message: "Visiting URL: " + host + ", IP: " + igs[0].ip
					}
					if igs[0].ip == _|_ {
						message: "Visiting URL: " + host
					}
				}
				"""#
		}
		workloadRefPath: ""
	}
	description: "Enable public web traffic for the component without creating a Service."
	labels: {
		"ui-hidden":  "true"
		"deprecated": "true"
	}
	type: "trait"
}

template: {
	legacyAPI: context.clusterVersion.minor < 19

	outputs: ingress: {
		if legacyAPI {
			apiVersion: "networking.k8s.io/v1beta1"
		}
		if !legacyAPI {
			apiVersion: "networking.k8s.io/v1"
		}
		kind: "Ingress"
		metadata:
			name: context.name
		spec: {
			rules: [{
				host: parameter.domain
				http: {
					paths: [
						for k, v in parameter.http {
							path: k
							if !legacyAPI {
								pathType: "ImplementationSpecific"
							}
							backend: {
								if legacyAPI {
									serviceName: context.name
									servicePort: v
								}
								if !legacyAPI {
									service: {
										name: context.name
										port: number: v
									}
								}
							}
						},
					]
				}
			}]
		}
	}
	parameter: {
		// +usage=Specify the domain you want to expose
		domain: string

		// +usage=Specify the mapping relationship between the http path and the workload port
		http: [string]: int
	}
}
