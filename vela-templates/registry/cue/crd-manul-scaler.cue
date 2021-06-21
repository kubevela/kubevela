outputs: scaler: {
	apiVersion: "core.oam.dev/v1alpha2"
	kind:       "ManualScalerTrait"
	spec: {
		replicaCount: parameter.replicas
	}
}
parameter: {
	//+short=r
	//+usage=Replicas of the workload
	replicas: *1 | int
}
