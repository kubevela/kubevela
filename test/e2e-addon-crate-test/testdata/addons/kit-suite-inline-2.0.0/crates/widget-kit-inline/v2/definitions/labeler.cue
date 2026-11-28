// v2 labeler, in CUE this time (v1 ships it as YAML).
labeler: {
	type:        "trait"
	description: "widget-kit-inline v2 labeler: tier label plus a line label."
	attributes: podDisruptive: false
}
template: {
	patch: metadata: labels: {
		"kitinline.example.com/tier":       parameter.tier
		"kitinline.example.com/line":       "v2"
		"kitinline.example.com/labeled-by": "widget-kit-inline-v2-labeler"
	}
	parameter: tier: *"silver" | string
}
