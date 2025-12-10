package component

// Schema for validating component definitions
// This file uses CUE's constraint system to validate structure

#ComponentDefinition: {
	// Every component must have a type
	type: "component"

	// Description is required and must be a non-empty string
	description: string & !=""

	// Annotations are optional but if present must be a struct
	annotations?: [string]: string

	// Labels are optional but if present must be a struct
	labels?: [string]: string

	// Attributes define component behavior
	attributes?: {
		workload?: {
			definition: {
				apiVersion: string
				kind:       string
			}
			type?: string
		}
		status?: {
			customStatus?:  string
			healthPolicy?:  string
		}
	}
}

#Template: {
	// Template must have parameter schema
	parameter: [string]: _

	// Template must have output
	output: {
		apiVersion: string
		kind:       string
		spec?:      _
	}

	// Outputs are optional for additional resources
	outputs?: [string]: {
		apiVersion: string
		kind:       string
		...
	}
}

// Import and validate statefulset definition structure
statefulset: #ComponentDefinition & {
	type: "component"
	description: string & =~"^.{10,}$"  // Description must be at least 10 chars
}

template: #Template & {
	parameter: {
		image: string  // Image is required
		cpu?:   string
		memory?: string
		...
	}

	output: {
		apiVersion: "apps/v1"
		kind:       "StatefulSet"
	}
}
