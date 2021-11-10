network: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Enable public web traffic for the component."
	attributes: {
		podDisruptive: true
		status: {
			customStatus: #"""
				import "strings"
				import "strconv"

				expose: {
					if parameter.expose != _|_ {
						ports: [
							for v in parameter.expose.ports {
								strconv.FormatInt(v, 10)
							}
						]
						msg: "\nExpose ports: " + strings.Join(ports, ", ")
					}
					if parameter.expose == _|_ {
						msg: ""
					}
				}

				hostAlias: {
					if parameter.hostAliases != _|_ {
						msg: [
							for v in parameter.hostAliases {
								"\nHostAliases IP: " + v.ip + "\nHostAliases HostNames: " + strings.Join(v.hostnames, ", ")
							}
						]
					}
					if parameter.hostAliases == _|_ {
						msg: []
					}
				}
				
				ingress: {
					if parameter.ingress != _|_ {
						let igs = context.outputs.ingress.status.loadBalancer.ingress
						if igs == _|_ {
							msg: "\nIngress: No loadBalancer found, visiting by using 'vela port-forward " + context.appName
						}
						if len(igs) > 0 {
							if igs[0].ip != _|_ {
								msg: "\nIngress: Visiting URL: " + context.outputs.ingress.spec.rules[0].host + ", IP: " + igs[0].ip
							}
							if igs[0].ip == _|_ {
								msg: "\nIngress: Visiting URL: " + context.outputs.ingress.spec.rules[0].host
							}
						}
					}
					if parameter.ingress == _|_ {
						msg: ""
					}
				}
				message: strings.Join(hostAlias.msg, "") + ingress.msg + expose.msg
				"""#
			healthPolicy: #"""
				if parameter.ingress != _|_ {
					isHealth: len(context.outputs.service.spec.clusterIP) > 0
				}
				"""#
		}
	}
}
template: {
	patch: {
		if parameter.hostAliases != _|_ {
			// +patchKey=ip
			spec: template: spec: hostAliases: parameter.hostAliases
		}
	}

	outputs: {
		if parameter.expose != _|_ {
			expose: {
				apiVersion: "v1"
				kind:       "Service"
				metadata: name: context.name + "-expose"
				spec: {
					selector: "app.oam.dev/component": context.name
					ports: [
						for p in parameter.expose.ports {
							port:       p
							targetPort: p
						},
					]
					type: parameter.expose.type
				}
			}
		}

		if parameter.ingress != _|_ {
			service: {
				apiVersion: "v1"
				kind:       "Service"
				metadata: name: context.name
				spec: {
					selector: "app.oam.dev/component": context.name
					ports: [
						for k, v in parameter.ingress.http {
							port:       v
							targetPort: v
						},
					]
				}
			}
			
			// the ingress API matches K8s v1.20+
			ingress: {
				apiVersion: "networking.k8s.io/v1"
				kind:       "Ingress"
				metadata: {
					name: context.name
					annotations: {
						"kubernetes.io/ingress.class": parameter.ingress.class
					}
				}
				spec: rules: [{
					host: parameter.ingress.domain
					http: paths: [
						for k, v in parameter.ingress.http {
							path:     k
							pathType: "ImplementationSpecific"
							backend: service: {
								name: context.name
								port: number: v
							}
						},
					]
				}]
			}
		}
	}

	parameter: {
		// +usage=Specify the ports you want to expose
		expose?: {
			// +usage=Specify the exposion ports
			ports: [...int]
			// +usage=Specify what kind of Service you want. options: "ClusterIP", "NodePort", "LoadBalancer", "ExternalName"
			type: *"ClusterIP" | "NodePort" | "LoadBalancer" | "ExternalName"
		}

		// +usage=Specify the hostAliases to add
		hostAliases?: [...{
			// +usage=Specify the ip for host aliases
			ip: string
			// +usage=Specify the hostnames
			hostnames: [...string]
		}]

		// +usage=Enable public web traffic for the component, the ingress API matches K8s v1.20+.
		ingress?: {
			// +usage=Specify the domain you want to expose
			domain: string
			// +usage=Specify the mapping relationship between the http path and the workload port
			http: [string]: int
			// +usage=Specify the class of ingress to use
			class: *"nginx" | string
		}
	}
}
