data: {
	apiVersion: "extend.oam.dev/v1alpha2"
	kind:       "SimpleRolloutTrait"
	spec: {
		replica:        parameter.replica
		maxUnavailable: parameter.maxUnavailable
		batch:          parameter.batch
	}
}

#rollout: {
	replica:        *3 | int
	maxUnavailable: *1 | int
	batch:          *2 | int
}
parameter: #rollout
