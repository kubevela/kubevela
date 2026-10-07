import "vela/test"

_web: {
	definition: "gateway"
	context: {name: "web", appName: "shop"}
	workload: {apiVersion: "apps/v1", kind: "Deployment", metadata: name: "web"}
}

"exposes each path through a Service and an Ingress": test.#TraitRender & _web & {
	parameter: {domain: "shop.example.com", http: {"/": 8080, "/api": 9090}}
	expect: {
		outputs: {
			service: {
				apiVersion: "v1"
				kind:       "Service"
				metadata: name: "web"
				spec: {
					selector: "app.oam.dev/component": "web"
					ports: [
						{name: "port-8080", port: 8080, targetPort: 8080},
						{name: "port-9090", port: 9090, targetPort: 9090},
					] @contains()
				}
			}
			ingress: {
				apiVersion: "networking.k8s.io/v1"
				kind:       "Ingress"
				metadata: {name: "web", annotations: "kubernetes.io/ingress.class": "nginx"}
				spec: {
					ingressClassName?: _|_
					tls?:              _|_
					rules: [{
						host: "shop.example.com"
						http: {
							paths: [
								{path: "/", pathType: "ImplementationSpecific", backend: service: {name: "web", port: number: 8080}},
								{path: "/api", backend: service: port: number: 9090},
							] @contains()
						}
					}]
				}
			}
		} @exact()
	}
}

"a class in the spec rather than an annotation": test.#TraitRender & _web & {
	parameter: {http: "/": 80, class: "traefik", classInSpec: true}
	expect: outputs: ingress: {
		metadata: annotations: "kubernetes.io/ingress.class"?: _|_
		spec: ingressClassName: "traefik"
	}
}

"TLS for the domain from a Secret": test.#TraitRender & _web & {
	parameter: {domain: "shop.example.com", http: "/": 80, secretName: "shop-tls"}
	expect: outputs: ingress: spec: tls: [{hosts: ["shop.example.com"], secretName: "shop-tls"}]
}

"a name suffixes its objects, so two gateways can coexist": test.#TraitRender & _web & {
	parameter: {http: "/": 80, name: "admin"}
	expect: {
		outputs: {
			"service-admin": metadata: name: "web-admin"
			"ingress-admin": {
				metadata: name: "web-admin"
				spec: rules: [{http: paths: [{backend: service: name: "web-admin"}]}]
			}
		} @exact()
	}
}

"an existing Service is routed to, not created": test.#TraitRender & _web & {
	parameter: {http: "/": 80, existingServiceName: "shop-frontend"}
	expect: outputs: {
		service?: _|_
		ingress: spec: rules: [{http: paths: [{backend: service: name: "shop-frontend"}]}]
	}
}

"annotations, labels and the gateway host land on the Ingress": test.#TraitRender & _web & {
	parameter: {
		http: "/": 80
		gatewayHost: "gw.example.com"
		annotations: "nginx.ingress.kubernetes.io/rewrite-target": "/"
		labels: tier:                                              "edge"
	}
	expect: outputs: ingress: metadata: {
		annotations: {"ingress.controller/host": "gw.example.com", "nginx.ingress.kubernetes.io/rewrite-target": "/"}
		labels: tier: "edge"
	}
}

"clusters before 1.19 get the beta Ingress": test.#TraitRender & _web & {
	context: clusterVersion: {major: "1", minor: 18, gitVersion: "v1.18.20", platform: "linux/amd64"}
	parameter: http: "/": 80
	expect: outputs: ingress: {
		apiVersion: "networking.k8s.io/v1beta1"
		spec: rules: [{http: paths: [{backend: {serviceName: "web", servicePort: 80}}]}]
	}
}

"healthy once its Ingress exists": test.#TraitStatus & _web & {
	parameter: http: "/": 80
	expect: healthy: true
} @pending(the status template fails under CUE 0.14 at ig: *_|_ | _ and no upgrade pass rescues it)

"suggests port-forwarding before a load balancer is assigned": test.#TraitStatus & _web & {
	parameter: http: "/": 80
	expect: message: =~"No loadBalancer found, visiting by using 'vela port-forward shop'"
} @pending(the status template fails under CUE 0.14 at ig: *_|_ | _ and no upgrade pass rescues it)

// Kubernetes reports status.loadBalancer; the template reads status.loadbalancer.
"shows the URL and IP once a load balancer is assigned": test.#TraitStatus & _web & {
	parameter: {domain: "shop.example.com", http: "/": 80}
	observed: outputs: ingress: status: loadBalancer: ingress: [{ip: "203.0.113.7"}]
	expect: message: "Visiting URL: shop.example.com, IP: 203.0.113.7\n"
} @pending(the status reads status.loadbalancer, which Kubernetes spells loadBalancer)
