"my-trait": {
	type: "trait"
	annotations: {}
	description: "Rollout the component."
	attributes: {
		manageWorkload: true
		status: {
			customStatus: #"""
				message: context.outputs.rollout.status.rollingState
				"""#
			healthPolicy: #"""
				isHealth: context.outputs.rollout.status.batchRollingState == "batchReady"
				"""#
		}
	}
}
template: {
	outputs: rollout: {}
}
