probe: {
	type:        "component"
	description: "probe-kit v1 probe (build a)."
	attributes: workload: type: "autodetects.core.oam.dev"
}
template: {
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: context.name
		data: build: "a"
	}
	parameter: {}
}
