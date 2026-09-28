import "vela/test"

"suspends the workflow": test.#WorkflowStepExec & {
	definition: "suspend"
	expect: {
		phase:   "suspending"
		message: "Suspended by field suspend"
	}
}

// The engine resumes a step once its duration has passed, on a later run; a
// single run can only show that it suspends.
"suspends when given a duration": test.#WorkflowStepExec & {
	definition: "suspend"
	parameter: duration: "30s"
	expect: phase:       "suspending"
}

"a duration that cannot be parsed fails the step": test.#WorkflowStepExec & {
	definition: "suspend"
	parameter: duration: "half an hour"
	expect: {
		phase:   "failed"
		message: =~"failed to parse duration half an hour"
	}
}

"shows the given message": test.#WorkflowStepExec & {
	definition: "suspend"
	parameter: message: "waiting for the change board"
	expect: {
		phase:   "suspending"
		message: "waiting for the change board"
	}
} @pending(kubevela/workflow builtin.#Suspend ignores its message parameter)
