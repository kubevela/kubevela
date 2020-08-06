#Template: {
	apiVersion: "core.oam.dev/v1alpha2"
	kind:       "ManualScalerTrait"
	spec: {
		replicaCount: manualscaler.replica
	}
}
manualscaler: {
	//+short=r
	replica: *2 | int
}
