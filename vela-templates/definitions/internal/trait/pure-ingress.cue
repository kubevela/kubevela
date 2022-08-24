"pure-ingress": {
	attributes: {
		appliesToWorkloads: ["*"]
		conflictsWith: []
		podDisruptive: false
		status: {
			customStatus: #"""
				let igs = context.outputs.ingress.status.loadBalancer.ingress
				if igs == _|_ {
					message: "No loadBalancer found, visiting by using 'vela port-forward " + context.appName + " --route'\n"
				}
				if len(igs) > 0 {
					if igs[0].ip != _|_ {
						message: "Visiting URL: " + context.outputs.ingress.spec.rules[0].host + ", IP: " + igs[0].ip
					}
					if igs[0].ip == _|_ {
						message: "Visiting URL: " + context.outputs.ingress.spec.rules[0].host
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
	outputs: ingress: {
		apiVersion: "networking.k8s.io/v1beta1"
		kind:       "Ingress"
		metadata:
			name: context.name
		spec: {
			rules: [{
				host: parameter.domain
				http: {
					paths: [
						for k, v in parameter.http {
							path: k
							backend: {
								serviceName: context.name
								servicePort: v
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
