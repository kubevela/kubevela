// Test template with pattern parameter selectors
parameter: {
	// Regular string parameter
	name: string

	// Pattern parameter selector - this would cause Unquoted() to panic
	[string]: _

	// Another regular parameter
	port: *8080 | int
}

output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
}