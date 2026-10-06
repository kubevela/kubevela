import "vela/test"

_web: {
	definition: "pure-ingress"
	context: {name: "web", appName: "shop"}
	workload: {apiVersion: "apps/v1", kind: "Deployment", metadata: name: "web"}
}

"routes each path to the component's own Service name": test.#TraitRender & _web & {
	parameter: {domain: "shop.example.com", http: {"/": 8080, "/api": 9090}}
	expect: {
		output: {apiVersion: "apps/v1", kind: "Deployment", spec?: _|_}
		outputs: {
			ingress: {
				apiVersion: "networking.k8s.io/v1beta1"
				kind:       "Ingress"
				metadata: name: "web"
				spec: rules: [{
					host: "shop.example.com"
					http: {
						paths: [
							{path: "/", backend: {serviceName: "web", servicePort: 8080}},
							{path: "/api", backend: {serviceName: "web", servicePort: 9090}},
						] @contains()
					}
				}]
			}
		} @exact()
	}
}

"a domain is required": test.#TraitRender & _web & {
	parameter: http: "/": 80
	expect: error: template: [=~"host: incomplete value"]
}

"a port must be an integer": test.#TraitRender & _web & {
	parameter: {domain: "shop.example.com", http: "/": "80"}
	expect: error: {
		parameter: [=~"http"] @contains()
	}
}

"suggests port-forwarding before a load balancer is assigned": test.#TraitStatus & _web & {
	parameter: {domain: "shop.example.com", http: "/": 80}
	expect: message: "No loadBalancer found, visiting by using 'vela port-forward shop --route'\n"
} @pending(the status takes len of the absent loadBalancer ingress list, so it fails until one is assigned)

"shows the URL and IP once a load balancer is assigned": test.#TraitStatus & _web & {
	parameter: {domain: "shop.example.com", http: "/": 80}
	observed: outputs: ingress: status: loadBalancer: ingress: [{ip: "203.0.113.7"}]
	expect: message: "Visiting URL: shop.example.com, IP: 203.0.113.7"
}

"shows the URL alone when the load balancer has a hostname": test.#TraitStatus & _web & {
	parameter: {domain: "shop.example.com", http: "/": 80}
	observed: outputs: ingress: status: loadBalancer: ingress: [{hostname: "lb.example.net"}]
	expect: message: "Visiting URL: shop.example.com"
}
