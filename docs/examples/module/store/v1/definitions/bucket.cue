// A definition file has two top-level parts: a metadata key, whose name is
// the capability name, and a template block. This installs on the hub as
// store-v1-bucket. Definitions are not placed by the runtime request.
"bucket": {
	type:        "component"
	description: "v1 bucket. Renders a ConfigMap."
	attributes: workload: type: "autodetects.core.oam.dev"
}

template: {
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: context.name
		data: bucket: parameter.bucketName
	}
	parameter: bucketName: string
}
