#Template: {
	apiVersion: "extend.oam.dev/v1alpha2"
	kind:       "SimpleRolloutTrait"
	spec: {
		replica:        rollout.replica
		maxUnavailable: rollout.maxUnavailable
		batch:          rollout.batch
	}
}

rollout: {
	replica:        *3 | int
	maxUnavailable: *1 | int
	batch:          *2 | int
}
