// The same capability name as v1/definitions/bucket.cue, and that is the
// point: the two install side by side as "demo-store-v1-bucket" and
// "demo-store-v2-bucket" because the API line is part of the object name.
//
// Because "bucket" now matches two definitions, a bare `type: bucket`
// reference is ambiguous and is rejected with an error naming both modules
// and lines. Use `type: demo-store/v2/bucket` (or `v2/bucket`) instead.
"bucket": {
	type: "component"
	attributes: workload: type: "autodetects.core.oam.dev"
	description: "v2 of the bucket capability: renames tier to class and adds a required region."
}

template: {
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: context.name
		data: {
			bucket:     parameter.bucketName
			region:     parameter.region
			class:      parameter.class
			renderedBy: "demo-store-v2-bucket"
		}
	}

	parameter: {
		bucketName: string
		// Required in v2 and absent from v1: the breaking change this line
		// exists for.
		region: string
		// v1's "tier" renamed, with a new allowed value.
		class: *"standard" | "premium" | "archive"
	}
}
