rollout: {
	type: "trait"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Rollout the component."
	attributes: {
		manageWorkload: true
	}
}
template: {
	outputs: rollout: {
		apiVersion: "standard.oam.dev/v1alpha1"
		kind:       "Rollout"
		metadata: {
			name:      context.name
			namespace: context.namespace
		}
		spec: {
			targetRevisionName: parameter.targetRevision
			componentName:      context.name
			rolloutPlan: {
				rolloutStrategy: "IncreaseFirst"
				if parameter.rolloutBatches != _|_ {
					rolloutBatches: parameter.rolloutBatches
				}
				targetSize: parameter.targetSize
				if parameter["batchPartition"] != _|_ {
					batchPartition: parameter.batchPartition
				}
			}
		}
	}

	parameter: {
		targetRevision: *context.revision | string
		targetSize:     int
		rolloutBatches?: [...rolloutBatch]
		batchPartition?: int
	}

	rolloutBatch: replicas: int
}
