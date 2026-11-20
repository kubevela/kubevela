output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	spec: components: [{
		name: "web"
		type: "shop-web"
		properties: {image: parameter.image, replicas: parameter.replicas}
	}]
}
