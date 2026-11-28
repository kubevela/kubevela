// v2 labeler, in CUE this time (v1 ships it as YAML).
labeler: {
	type:        "trait"
	description: "widget-kit v2 labeler: tier label plus a line label."
	attributes: podDisruptive: false
}
template: {
	patch: metadata: labels: {
		"kit.example.com/tier":       parameter.tier
		"kit.example.com/line":       "v2"
		"kit.example.com/labeled-by": "widget-kit-v2-labeler"
	}
	parameter: tier: *"silver" | string
}
