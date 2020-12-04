output: {
	apiVersion: "flagger.app/v1beta1"
	kind:       "Canary"
	spec: {
		provider:                "smi"
		progressDeadlineSeconds: 60
		service: {
			// Currently Traffic route is not supported, but this is required field for flagger CRD
			port: 80
			// Currently Traffic route is not supported, but this is required field for flagger CRD
			targetPort: 8080
		}
		analysis: {
			interval: parameter.interval
			// max number of failed metric checks before rollback
			threshold: 10
			// max traffic percentage routed to canary
			// percentage (0-100)
			maxWeight: 50
			// canary increment step
			// percentage (0-100)
			stepWeight: parameter.stepWeight
			// max replicas scale up to canary
			maxReplicas: parameter.replicas
		}
	}
}
parameter: {
	// +usage=Total replicas of the workload
	replicas: *2 | int
	// +alias=step-weight
	// +usage=Weight percent of every step in rolling update
	stepWeight: *50 | int
	// +usage=Schedule interval time
	interval: *"30s" | string
}
