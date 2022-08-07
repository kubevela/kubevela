"kustomize-json-patch-mock-adddon": {
	attributes: {
		podDisruptive: false
	}
	description: "A list of JSON6902 patch to selected target"
	labels: {
		"ui-hidden": "true"
	}
	type: "trait"
}

template: {
	patch: {
		spec: {
			patches: parameter.patchesJson
		}
	}

	parameter: {
		// +usage=A list of JSON6902 patch.
		patchesJson: [...#jsonPatchItem]
	}

	// +usage=Contains a JSON6902 patch
	#jsonPatchItem: {
		target: #selector
		patch: [...{
			// +usage=operation to perform
			op: string | "add" | "remove" | "replace" | "move" | "copy" | "test"
			// +usage=operate path e.g. /foo/bar
			path: string
			// +usage=specify source path when op is copy/move
			from?: string
			// +usage=specify opraation value when op is test/add/replace
			value?: string
		}]
	}

	// +usage=Selector specifies a set of resources
	#selector: {
		group?:              string
		version?:            string
		kind?:               string
		namespace?:          string
		name?:               string
		annotationSelector?: string
		labelSelector?:      string
	}

}
