// A TraitDefinition in the same line. Definitions of different kinds live
// side by side in one definitions/ directory; the "type" field is what
// distinguishes them, not the filename or the directory.
//
// Installs as "demo-store-v1-labeler". Referenced from an Application's
// traits list as demo-store/v1/labeler, v1/labeler, or labeler.
"labeler": {
	type: "trait"
	attributes: {
		// Whether applying this trait forces the workload to be recreated.
		podDisruptive: false
		// Which workloads it may be attached to; "*" means any.
		appliesToWorkloads: ["*"]
	}
	description: "Adds labels to the component's rendered workload."
}

template: {
	// A trait template uses "patch" to modify the component's output
	// in place, rather than "output" to render a new resource.
	patch: metadata: labels: {
		for k, v in parameter.labels {
			(k): v
		}
	}

	parameter: {
		// Labels merged onto the workload's metadata.
		labels: [string]: string
	}
}
