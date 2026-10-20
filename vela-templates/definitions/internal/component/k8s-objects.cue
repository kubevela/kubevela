"k8s-objects": {
	type: "component"
	annotations: {}
	labels: {}
	description: "K8s-objects allow users to specify raw K8s objects in properties"
	attributes: workload: type: "autodetects.core.oam.dev"
}
template: {
	output: {
		if len(parameter.objects) > 0 {
			parameter.objects[0]
		}
		...
	}

	outputs: {
		for i, v in parameter.objects {
			if i > 0 {
				"objects-\(i)": v
			}
		}
	}
	parameter: {
		objects: [...{}]
	}
}
