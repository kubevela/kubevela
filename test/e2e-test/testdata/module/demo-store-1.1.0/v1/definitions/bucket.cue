// A definition .cue file has exactly two top-level parts.
//
//  1. A metadata key -- the quoted name below. The key IS the capability
//     name: it becomes metadata.name of the definition object, which the
//     render service then rewrites to <module>-<apiVersion>-<name>. This file
//     therefore installs as "demo-store-v1-bucket".
//  2. A "template" block holding the CUE that renders the capability.
//
// Both are required; a file missing either is rejected by the parser.
//
// Reference it from an Application in any of three forms:
//   type: demo-store/v1/bucket   fully qualified, no cluster lookup needed
//   type: v1/bucket              line-scoped, resolved by label
//   type: bucket                 bare, resolved by label if no legacy
//                                definition of that name exists
"bucket": {
	// type selects the kind of definition produced. One of: component,
	// trait, policy, workflow-step, workload.
	type: "component"

	// attributes carries the definition's spec fields other than the
	// template. For a component, workload.type "autodetects.core.oam.dev"
	// means KubeVela discovers the rendered object's GVK instead of being
	// told it up front.
	attributes: workload: type: "autodetects.core.oam.dev"

	// Optional documentation, surfaced by `vela show`.
	annotations: {}
	labels: {}
	description: "v1 of the bucket capability: renders a ConfigMap standing in for a storage bucket."
}

template: {
	// output is the single main resource the component renders.
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: context.name
		data: {
			bucket: parameter.bucketName
			// Pinning the line into the rendered object is what makes a v1
			// vs v2 install visible in the cluster without reading labels.
			renderedBy: "demo-store-v1-bucket"
		}
	}

	// outputs (plural) renders additional resources alongside output. Each
	// key is an arbitrary local name.
	outputs: marker: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: context.name + "-marker"
		data: line: "v1"
	}

	// parameter is the user-facing schema of this capability: what an
	// Application puts under the component's "properties".
	parameter: {
		// Name recorded for the bucket. Required, no default.
		bucketName: string
		// Optional tier label. The "*" marks the default.
		tier: *"standard" | "premium"
	}
}
