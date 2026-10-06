// A trait that emits an extra object through `outputs`. Used by scenario 04 to
// show how the trait type string ends up in the trait.oam.dev/type label of
// that object (see architecture.md, "Known defects").
note: {
	type:        "trait"
	description: "Writes a ConfigMap note next to the component."
	attributes: podDisruptive: false
}
template: {
	outputs: note: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: "\(context.name)-note"
		data: text: parameter.text
	}
	parameter: {
		// +usage=Text stored in the note
		text: *"noted by widget-kit-inline v1" | string
	}
}
