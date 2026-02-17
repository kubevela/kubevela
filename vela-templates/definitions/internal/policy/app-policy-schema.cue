// app-policy-schema.cue defines the standard schema for Application-scoped policies
// Application-scoped policies (scope: "Application") transform the Application CR before parsing
// They use the transforms pattern and don't generate Kubernetes resources

package policy

// ApplicationPolicyTemplate defines the standard structure for Application-scoped policies
// This provides type safety, validation, and defaults for policy templates
#ApplicationPolicyTemplate: {
	// Parameter schema defined by each policy
	parameter: {...}

	// Configuration for policy execution and caching behavior
	config: {
		// Whether policy is enabled (default: true)
		// Can be a boolean literal or an expression (e.g., parameter.enabled)
		enabled: *true | bool

		// Per-output-type refresh control for caching
		// Controls when each output type should be re-rendered vs cached
		refresh?: {
			// Spec refresh control (components, workflow, policies)
			// These are structural changes that affect Application behavior
			// Default mode: "never" - only refresh when Application revision changes
			spec?: {
				// Refresh mode: when to re-render this output type
				// - "never": Cache indefinitely (until Application revision changes)
				// - "always": Re-render on every reconciliation
				// - "periodic": Re-render after interval seconds
				mode: *"never" | "always" | "periodic"

				// Interval in seconds (required if mode == "periodic")
				if mode == "periodic" {
					interval: int & >0
				}

				// Force refresh expression (optional)
				// Dynamic boolean expression to force cache invalidation
				// Example: context.appLabels["force-refresh"] != _|_
				forceRefresh?: bool
			}

			// Labels refresh control (metadata labels)
			// Default mode: "never" - labels are usually static
			labels?: {
				mode: *"never" | "always" | "periodic"
				if mode == "periodic" {
					interval: int & >0
				}
				forceRefresh?: bool
			}

			// Annotations refresh control (metadata annotations)
			// Default mode: "never" - annotations are usually static
			annotations?: {
				mode: *"never" | "always" | "periodic"
				if mode == "periodic" {
					interval: int & >0
				}
				forceRefresh?: bool
			}

			// Context refresh control (additional workflow context)
			// Default mode: "never" - context is usually static
			ctx?: {
				mode: *"never" | "always" | "periodic"
				if mode == "periodic" {
					interval: int & >0
				}
				forceRefresh?: bool
			}
		}
	}

	// Policy output structure
	// Defines the transformations to apply to the Application
	output: {
		// Structural changes (require Application revision for changes)
		// These modify the Application's components, workflow, or policies
		components?: [...#Component]
		workflow?:   #Workflow
		policies?:   [...#Policy]

		// Metadata changes (can refresh between reconciliations if configured)
		// Labels and annotations applied to the Application
		labels?:      {[string]: string}
		annotations?: {[string]: string}

		// Additional context passed to workflow execution
		// This is available to workflow steps via context
		ctx?: {[string]: _}
	}
}

// Placeholder definitions - these should reference the actual KubeVela types
// In practice, these would import from the actual Application schema
#Component: {...}
#Workflow: {...}
#Policy: {...}
