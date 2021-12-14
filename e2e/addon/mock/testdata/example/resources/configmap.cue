output: {
	type: "raw"
	properties: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      "exampleinput"
			namespace: "default"
		}
		data: input: parameter.example
	}
}
