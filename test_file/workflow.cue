// A Policy that creates a ConfigMap with user-provided data
"myworkflow": {
	type: "workflow-step"
	annotations: {
		"category": "Application Delivery"
	}
	labels: {
		"scope": "Application"
	}
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
			data: parameter.data
		}

		// What the user can supply when they invoke this policy
		parameter: {
			// +usage=Name of the ConfigMap to create
			name: string

			// +usage=Namespace for the ConfigMap
			namespace?: *"default" | string

			// +usage=Key-value pairs to store in the ConfigMap
			data: [string]: string

			testParam: *"5" | string
		}
	}
