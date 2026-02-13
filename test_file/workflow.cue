// A Policy that creates a ConfigMap with user-provided data
"myworkflow": {
	type: "workflow-step"
	description: "Apply a configmap in application"
}

template: {
	// What this policy will render
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      parameter.name
			namespace: parameter.namespace
		}
		data: {
			data: parameter.testParam
		}
	}

	// What the user can supply when they invoke this policy
	parameter: {
		// +usage=Name of the ConfigMap to create
		name: string

		// +usage=Namespace for the ConfigMap
		namespace?: *"default" | string

		testParam: *"6" | string
	}
}
