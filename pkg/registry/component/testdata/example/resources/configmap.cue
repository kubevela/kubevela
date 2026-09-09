output: {
	type: "raw"
	properties: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      "exampleinput"
			namespace: "default"
			labels: {
			  version: context.metadata.version
			}
		}
		data: input: parameter.example
	}
}
