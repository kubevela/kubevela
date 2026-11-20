// A ComponentDefinition in vela-definition CUE format (the same format as
// `vela def apply`). The top-level key is the SHORT name; the render service
// installs it as widget-kit-inline-v1-widget (naming.DefinitionName) and labels it
//   definition.oam.dev/module=widget-kit-inline
//   definition.oam.dev/module-api-version=v1
//   definition.oam.dev/name=widget
widget: {
	type:        "component"
	description: "widget-kit-inline v1 Widget: a Widget custom resource plus a ConfigMap card."
	attributes: workload: type: "autodetects.core.oam.dev"
}
template: {
	output: {
		apiVersion: "kitinline.example.com/v1alpha1"
		kind:       "Widget"
		metadata: name: context.name
		spec: {
			apiLine:  "v1"
			classRef: "widget-kit-inline-v1-standard"
			color:    parameter.color
			replicas: parameter.replicas
		}
	}
	outputs: card: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: "\(context.name)-card"
		data: {
			module: "widget-kit-inline"
			line:   "v1"
			color:  parameter.color
		}
	}
	parameter: {
		// +usage=Widget colour
		color: *"blue" | "red" | "green"
		// +usage=Replica count recorded on the Widget
		replicas: *1 | int & >=1 & <=3
	}
}
