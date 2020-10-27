output: {
	apiVersion: "core.oam.dev/v1alpha2"
	kind:       "ManualScalerTrait"
	spec: {
		replicaCount: parameter.replica
	}
}
parameter: {
	//+short=r
	replica: *2 | int
}
