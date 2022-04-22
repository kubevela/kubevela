"xp-expose": {
	type: "trait"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
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
			port: [
				for p in parameter.ports {
					port:       p.port
					targetPort: p.port
					name:       p.name
				},
			]
			type: parameter.type
		}
	}

	#port: {
		// +usage=Specify the port name
		name: string
		// +usage=Specify the port value
		port: int
	}

	parameter: {
		// +usage=Specify the exposion ports
		port: [...#port]
		// +usage=Specify what kind of Service you want. options: "ClusterIP","NodePort","LoadBalancer","ExternalName"
		type: *"ClusterIP" | "NodePort" | "LoadBalancer" | "ExternalName"
	}
}
