"testdef": {
	type: "component"
	annotations: {
		"definition.oam.dev/example-url": "http://127.0.0.1:65501/examples/applications/create-namespace.yaml"
	}
	labels: {}
	description: "K8s-objects allow users to specify raw K8s objects in properties"
	attributes: workload: type: "autodetects.core.oam.dev"
}
template: {
	outputs: {
		for i, v in parameter.objects {
			if i > 0 {
				"objects-\(i)": v
			}
		}
	}
	parameter: {
		// +usage=A test key
		objects: [...{}]
	}
}
