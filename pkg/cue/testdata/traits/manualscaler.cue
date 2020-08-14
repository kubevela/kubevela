#Template: {
	apiVersion: "core.oam.dev/v1alpha2"
	kind:       "ManualScalerTrait"
	spec: {
		replicaCount: scale.replica
	}
}
scale: {
	//+short=r
	replica: *2 | int
}
