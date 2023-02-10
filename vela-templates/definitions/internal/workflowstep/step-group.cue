"step-group": {
	type: "workflow-step"
	annotations: {
		"category": "Process Control"
	}
	description: "A special step that you can declare 'subSteps' in it, 'subSteps' is an array containing any step type whose valid parameters do not include the `step-group` step type itself. The sub steps were executed in parallel."
}
template: {
	// no parameters, the nop only to make the template not empty or it's invalid
	nop: {}
}
