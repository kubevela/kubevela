expose: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Expose port to enable web traffic for your component."
	attributes: {
		podDisruptive: false
	}
}
template: {
	outputs: service: {
		apiVersion: "v1"
		kind:       "Service"
		metadata: name: context.name
		spec: {
			selector: "app.oam.dev/component": context.name
			ports: [
				for p in parameter.port {
					port:       p
					targetPort: p
				},
			]
			type: parameter.type
		}
	}
	parameter: {
		// +usage=Specify the exposion ports
		port: [...int]
		// +usage=Specify what kind of Service you want. options: "ClusterIP","NodePort","LoadBalancer","ExternalName"
		type: *"ClusterIP" | "NodePort" | "LoadBalancer" | "ExternalName"
	}
}
