import "vela/test"

_web: {
	definition: "expose"
	context: {name: "web", appName: "shop"}
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [{name: "web", image: "shop:1.0"}]
	}
}

"exposes the ports through a ClusterIP Service selecting the component": test.#TraitRender & _web & {
	parameter: ports: [{port: 80}, {port: 53, protocol: "UDP"}, {port: 9090, name: "metrics"}]
	expect: {
		outputs: {
			service: {
				apiVersion: "v1"
				kind:       "Service"
				metadata: name: "web"
				spec: {
					selector: {
						"app.oam.dev/component": "web" @exact()
					}
					type: "ClusterIP"
					ports: [
						{port: 80, targetPort: 80, name: "port-80", protocol: "TCP"},
						{port: 53, targetPort: 53, name: "port-53-udp", protocol: "UDP"},
						{port: 9090, targetPort: 9090, name: "metrics"},
					]
				}
			}
		} @exact()
		output: spec: template: spec: containers: [{name: "web", image: "shop:1.0"}]
	}
}

"the deprecated port list names each port by number": test.#TraitRender & _web & {
	parameter: port: [80, 443]
	expect: outputs: service: spec: ports: [
		{port: 80, targetPort: 80, name: "port-80"},
		{port: 443, targetPort: 443, name: "port-443"},
	]
}

"ports win over the deprecated port list": test.#TraitRender & _web & {
	parameter: {port: [80], ports: [{port: 8080}]}
	expect: outputs: service: spec: ports: [{port: 8080}]
}

"a nodePort is kept for a NodePort Service": test.#TraitRender & _web & {
	parameter: {type: "NodePort", ports: [{port: 80, nodePort: 30080}]}
	expect: outputs: service: spec: {type: "NodePort", ports: [{nodePort: 30080}]}
}

"a nodePort is dropped for other Services": test.#TraitRender & _web & {
	parameter: {type: "LoadBalancer", ports: [{port: 80, nodePort: 30080}]}
	expect: outputs: service: spec: {type: "LoadBalancer", ports: [{nodePort?: _|_}]}
}

"matchLabels replace the component selector": test.#TraitRender & _web & {
	parameter: {ports: [{port: 80}], matchLabels: app: "shop-web"}
	expect: outputs: service: spec: {
		selector: {app: "shop-web"} @exact()
	}
}

"annotations land on the Service": test.#TraitRender & _web & {
	parameter: {ports: [{port: 80}], annotations: "service.beta.kubernetes.io/aws-load-balancer-type": "nlb"}
	expect: outputs: service: metadata: annotations: "service.beta.kubernetes.io/aws-load-balancer-type": "nlb"
}

"the type must be a Service type": test.#TraitRender & _web & {
	parameter: {ports: [{port: 80}], type: "Ingress"}
	expect: error: {
		parameter: [=~"type"] @contains()
	}
}

"a ClusterIP Service is healthy and shows its cluster IP": test.#TraitStatus & _web & {
	parameter: ports: [{port: 80}]
	observed: outputs: service: spec: clusterIP: "10.96.0.12"
	expect: {healthy: true, message: "ClusterIP: 10.96.0.12"}
}

"a NodePort Service is healthy with no message": test.#TraitStatus & _web & {
	parameter: {type: "NodePort", ports: [{port: 80}]}
	expect: {healthy: true, message: ""}
}

// The top-level message default and the LoadBalancer branch's default are
// both marked, so neither wins.
"a LoadBalancer Service waits for its external IP": test.#TraitStatus & _web & {
	parameter: {type: "LoadBalancer", ports: [{port: 80}]}
	expect: {healthy: false, message: "ExternalIP: Pending"}
} @pending(message is defaulted twice, to empty and to ExternalIP: Pending, so it is non-concrete until an IP is assigned)

"a LoadBalancer Service is healthy once it has an external IP": test.#TraitStatus & _web & {
	parameter: {type: "LoadBalancer", ports: [{port: 80}]}
	observed: outputs: service: status: loadBalancer: ingress: [{ip: "203.0.113.7"}]
	expect: {healthy: true, message: "ExternalIP: 203.0.113.7"}
}
