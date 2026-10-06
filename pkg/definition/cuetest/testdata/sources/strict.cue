strict: {
	type:        "source"
	description: "Echoes its parameter, refusing negatives, for a component to read"
}
template: {
	schema: {count: int}
	consumableFrom: ["component"]
	parameter: {
		count: int
		extra: *false | bool
	}
	errs: [
		if parameter.count < 0 {"count must not be negative, got \(parameter.count)"},
	]
	output: {
		count: parameter.count
		if parameter.extra {surplus: true}
	}
}
