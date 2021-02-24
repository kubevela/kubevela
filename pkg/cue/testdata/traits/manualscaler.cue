outputs: scaler: {
	apiVersion: "core.oam.dev/v1alpha2"
	kind:       "ManualScalerTrait"
	spec: {
		replicaCount: parameter.replicas
	}
}
parameter: {
	//+short=r
	replicas: *2 | int
}
