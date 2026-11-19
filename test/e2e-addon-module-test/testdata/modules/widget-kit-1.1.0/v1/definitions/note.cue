// A trait that emits an extra object through `outputs`. Used by scenario 04 to
// verify this trait's outputs use the installed name in trait.oam.dev/type,
// not the Form 3 reference string.
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
		text: *"noted by widget-kit v1" | string
	}
}
