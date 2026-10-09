// Added in 1.1.0. On upgrade this appears as a new object in the
// demo-store-v1-defs tier and installs as demo-store-v1-archive.
"archive": {
	type: "component"
	attributes: workload: type: "autodetects.core.oam.dev"
	description: "Cold storage capability, added in module version 1.1.0."
}

template: {
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: context.name
		data: {
			archive:    parameter.name
			renderedBy: "demo-store-v1-archive"
		}
	}
	parameter: {
		name: string
	}
}
