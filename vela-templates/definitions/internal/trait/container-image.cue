"container-image": {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Set the image of the container."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["*"]
	}
}
template: {
	parameter: {
		container: *"" | string
		image: string
		imagePullPolicy: *"IfNotPresent" | "Always" | "Never"
	}
	// +patchStrategy=retainKeys
	patch: spec: template: spec: {
		// +patchKey=name
		containers: [{
			if parameter.container != "" {
				name: parameter.container
			}
			if parameter.container == "" {
				name: context.name
			}
			// +patchStrategy=retainKeys
			image: parameter.image
			imagePullPolicy: parameter.imagePullPolicy
		}]
	}
}
