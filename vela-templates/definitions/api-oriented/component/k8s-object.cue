"k8s-object": {
	type: "component"
	annotations: {}
	labels: {}
	description: "K8s-object allow users to specify raw K8s object in properties"
	attributes: workload: type: "autodetects.core.oam.dev"
}
template: {
	output: parameter.objects[0]

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
