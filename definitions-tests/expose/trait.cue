import (
	"strconv"
)
expose: {
	type: "trait"
	description: "Expose port to enable web traffic for your component."
}
template: {
	outputs: service: {
		apiVersion: "v1"
		kind:       "Service"
		metadata: name:        context.name
		metadata: annotations: parameter.annotations
		spec: {
			selector: "app.oam.dev/component": context.name
			ports: [
				for p in parameter.ports {
					name:       "port-" + strconv.FormatInt(p, 10)
					port:       p
					targetPort: p
				},
			]
			type: parameter.type
		}
	}
	parameter: {
		// +usage=Specify the exposion ports
		ports: [...int]
		// +usage=Specify the annotaions of the exposed service
		annotations: [string]: string
		// +usage=Specify what kind of Service you want. options: "ClusterIP","NodePort","LoadBalancer","ExternalName"
		type: *"ClusterIP" | "NodePort" | "LoadBalancer" | "ExternalName"
	}
}
