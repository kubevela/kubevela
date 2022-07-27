import (
	"strconv"
)

expose: {
	type: "trait"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Expose port to enable web traffic for your component."
	attributes: {
		podDisruptive: false
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps"]
		status: {
			customStatus: #"""
				message: *"" | string
				service: context.outputs.service
				if service.spec.type == "ClusterIP" {
					message: "ClusterIP: \(service.spec.clusterIP)"
				}
				if service.spec.type == "LoadBalancer" {
					status: service.status
					isHealth: status != _|_ && status.loadBalancer != _|_ && status.loadBalancer.ingress != _|_ && len(status.loadBalancer.ingress) > 0
					if !isHealth {
						message: "ExternalIP: Pending"
					}
					if isHealth {
						message: "ExternalIP: \(status.loadBalancer.ingress[0].ip)"
					}
				}
				"""#
			healthPolicy: #"""
				isHealth: *true | bool
				service: context.outputs.service
				if service.spec.type == "LoadBalancer" {
					status: service.status
					isHealth: status != _|_ && status.loadBalancer != _|_ && status.loadBalancer.ingress != _|_ && len(status.loadBalancer.ingress) > 0
				}
				"""#
		}
	}
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
				for p in parameter.port {
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
		port: [...int]
		// +usage=Specify the annotaions of the exposed service
		annotations: [string]: string
		// +usage=Specify what kind of Service you want. options: "ClusterIP","NodePort","LoadBalancer","ExternalName"
		type: *"ClusterIP" | "NodePort" | "LoadBalancer" | "ExternalName"
	}
}
