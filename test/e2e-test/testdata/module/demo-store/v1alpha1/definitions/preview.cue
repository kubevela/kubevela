// Never installed while the line is disabled. Present to prove that a
// disabled line's definitions are still parsed and validated at publish time,
// so a broken experimental line fails the publish rather than lying dormant
// and failing later when somebody enables it.
"preview": {
	type: "component"
	attributes: workload: type: "autodetects.core.oam.dev"
	description: "Experimental capability, shipped but not installed."
}

template: {
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: context.name
		data: renderedBy: "demo-store-v1alpha1-preview"
	}
	parameter: {}
}
