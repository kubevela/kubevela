import "vela/test"

"enables with defaults": test.#AddonRender & {
	addon: "."
	expect: {
		application: kind: "Application"
		components: {
			web: {type: "shop-web", properties: {image: "shop:1.0", replicas: 1}}
			svc: properties: {
				kind: "Service"
				metadata: labels: version: "1.2.0"
				spec: type: "ClusterIP"
			}
		}
		definitions: ComponentDefinition: "shop-web": kind: "ComponentDefinition"
	}
}

"parameters reach the components": test.#AddonRender & {
	addon: "."
	parameter: {image: "shop:2.0", serviceType: "NodePort"}
	expect: components: {
		web: properties: image: "shop:2.0"
		svc: properties: spec: type: "NodePort"
	}
}

"rejects a parameter the addon does not allow": test.#AddonRender & {
	addon: "."
	parameter: serviceType: "Nope"
	expect: error: =~"parameter.serviceType"
}

