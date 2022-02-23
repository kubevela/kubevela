rollout: {
	type: "trait"
	annotations: {}
	description: "Rollout the component."
	attributes: {
		manageWorkload: true
		status: {
			customStatus: #"""
				message: context.outputs.rollout.status.rollingState
				"""#
			healthPolicy: #"""
				isHealth: context.outputs.rollout.status.batchRollingState == "batchReady"
				"""#
		}
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
			if parameter.targetRevision != _|_ {
				targetRevisionName: parameter.targetRevision
			}
			if parameter.targetRevision == _|_ {
				targetRevisionName: context.revision
			}
			componentName: context.name
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
		// +usage=Specify the target revision, it should be set if you want to rollback. such as: componentname-v1
		targetRevision?: string
		// +usage=Specify the count of replicas.
		targetSize: int
		// +usage=Specify the rollout batches, The total number of replicas of all batches needs to be equal to number of targetSize.
		rolloutBatches?: [...rolloutBatch]
		// +usage=Specify the batch partition in current deploying. It is used to control batch processes.
		batchPartition?: int
	}

	rolloutBatch: replicas: int
}
