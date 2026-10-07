import "vela/test"

// The workflow engine runs a step-group itself, sub-steps and all, and never
// evaluates this template; run on its own, the template does nothing.
"the template on its own does nothing": test.#WorkflowStepExec & {
	definition: "step-group"
	expect: {
		phase: "succeeded"
		calls: {} @exact()
	}
}
