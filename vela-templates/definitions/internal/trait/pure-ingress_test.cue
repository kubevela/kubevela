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
				apiVersion: "networking.k8s.io/v1"
				kind:       "Ingress"
				metadata: name: "web"
				spec: rules: [{
					host: "shop.example.com"
					http: {
						paths: [
							{path: "/", pathType: "ImplementationSpecific", backend: service: {name: "web", port: number: 8080}},
							{path: "/api", pathType: "ImplementationSpecific", backend: service: {name: "web", port: number: 9090}},
						] @contains()
					}
				}]
			}
		} @exact()
	}
}

"clusters before 1.19 get the beta Ingress": test.#TraitRender & _web & {
	context: clusterVersion: {major: "1", minor: 18, gitVersion: "v1.18.20", platform: "linux/amd64"}
	parameter: {domain: "shop.example.com", http: "/": 80}
	expect: outputs: ingress: {
		apiVersion: "networking.k8s.io/v1beta1"
		spec: rules: [{http: paths: [{path: "/", backend: {serviceName: "web", servicePort: 80}}]}]
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
}

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
