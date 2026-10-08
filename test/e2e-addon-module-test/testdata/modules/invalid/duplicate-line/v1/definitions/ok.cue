ok: {
	type: "component"
	attributes: workload: type: "autodetects.core.oam.dev"
}
template: {
	output: {apiVersion: "v1", kind: "ConfigMap", metadata: name: context.name}
	parameter: {}
}
