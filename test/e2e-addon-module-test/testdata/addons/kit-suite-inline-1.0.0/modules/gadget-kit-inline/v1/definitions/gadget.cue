// Installed as gadget-kit-inline-v1-gadget. Output is a Gadget custom resource whose
// CRD comes from this module's own module-level auxiliary tier.
gadget: {
	type:        "component"
	description: "gadget-kit-inline v1 Gadget custom resource."
	attributes: workload: type: "autodetects.core.oam.dev"
}
template: {
	output: {
		apiVersion: "kitinline.example.com/v1alpha1"
		kind:       "Gadget"
		metadata: name: context.name
		spec: {
			apiLine: "v1"
			mode:    parameter.mode
		}
	}
	parameter: {
		// +usage=Gadget mode
		mode: *"eco" | "turbo"
	}
}
