"ref-objects": {
	type: "component"
	annotations: {}
	labels: {}
	description: "Ref-objects allow users to specify ref objects to use. Notice that this component type have special handle logic."
	attributes: workload: type: "autodetects.core.oam.dev"
}
template: {
	#K8sObject: {
		apiVersion: string
		kind:       string
		metadata: {
			name: string
			...
		}
		...
	}

	output: parameter.objects[0]

	outputs: {
		for i, v in parameter.objects {
			if i > 0 {
				"objects-\(i)": v
			}
		}
	}
	parameter: {
		objects: [...#K8sObject]
	}
}
