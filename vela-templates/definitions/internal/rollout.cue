rollout: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "rollout the component"
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
			labels: {
				"trait.oam.dev/rely-on-comp-rev": parameter.targetRevision
			}
		}
		spec: {
			targetRevisionName: parameter.targetRevision
			componentName:      context.name
			rolloutPlan: {
				rolloutStrategy: "IncreaseFirst"
				rolloutBatches:  parameter.rolloutBatches
				targetSize:      parameter.targetSize
				if parameter["batchPartition"] != _|_ {
					batchPartition: parameter.batchPartition
				}
			}
		}
	}

	parameter: {
		targetRevision: *context.revision | string
		targetSize:     int
		rolloutBatches: [...rolloutBatch]
		batchPartition?: int
	}

	rolloutBatch: replicas: int
}
