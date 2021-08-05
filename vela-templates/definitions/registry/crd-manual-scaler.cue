"crd-manual-scaler": {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Configures replicas for your service implemented by CRD controller."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps"]
		workloadRefPath: "spec.workloadRef"
		definitionRef: name: "manualscalertraits.core.oam.dev"
	}
}
template: {
	outputs: scaler: {
		apiVersion: "core.oam.dev/v1alpha2"
		kind:       "ManualScalerTrait"
		spec: replicaCount: parameter.replicas
	}
	parameter: {
		//+short=r
		//+usage=Replicas of the workload
		replicas: *1 | int
	}
}
