widget: {
	type:        "component"
	description: "widget-kit-inline v1beta1 preview Widget (line disabled)."
	attributes: workload: type: "autodetects.core.oam.dev"
}
template: {
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: context.name
		data: line: "v1beta1"
	}
	parameter: {}
}
