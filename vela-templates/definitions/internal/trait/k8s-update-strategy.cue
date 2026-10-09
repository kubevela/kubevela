"k8s-update-strategy": {
	alias: ""
	annotations: {}
	attributes: {
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps"]
		conflictsWith: []
		podDisruptive:   false
		workloadRefPath: ""
	}
	description: "Set k8s update strategy for Deployment/DaemonSet/StatefulSet"
	labels: {}
	type: "trait"
}

template: {
	// rollingStrategy is optional; its fields take their defaults when it is left out
	let rolling = {
		maxSurge:       *"25%" | string
		maxUnavailable: *"25%" | string
		partition:      *0 | int
		if parameter.strategy.rollingStrategy != _|_ {
			parameter.strategy.rollingStrategy
		}
	}
	patch: {
		spec: {
			if parameter.targetKind == "Deployment" && parameter.strategy.type != "OnDelete" {
				// +patchStrategy=retainKeys
				strategy: {
					type: parameter.strategy.type
					if parameter.strategy.type == "RollingUpdate" {
						rollingUpdate: {
							maxSurge:       rolling.maxSurge
							maxUnavailable: rolling.maxUnavailable
						}
					}
				}
			}

			if parameter.targetKind == "StatefulSet" && parameter.strategy.type != "Recreate" {
				// +patchStrategy=retainKeys
				updateStrategy: {
					type: parameter.strategy.type
					if parameter.strategy.type == "RollingUpdate" {
						rollingUpdate: {
							partition: rolling.partition
						}
					}
				}
			}

			if parameter.targetKind == "DaemonSet" && parameter.strategy.type != "Recreate" {
				// +patchStrategy=retainKeys
				updateStrategy: {
					type: parameter.strategy.type
					if parameter.strategy.type == "RollingUpdate" {
						rollingUpdate: {
							maxSurge:       rolling.maxSurge
							maxUnavailable: rolling.maxUnavailable
						}
					}
				}
			}

		}}
	parameter: {
		// +usage=Specify the apiVersion of target
		targetAPIVersion: *"apps/v1" | string
		// +usage=Specify the kind of target
		targetKind: *"Deployment" | "StatefulSet" | "DaemonSet"
		// +usage=Specify the strategy of update
		strategy: {
			// +usage=Specify the strategy type
			type: *"RollingUpdate" | "Recreate" | "OnDelete"
			// +usage=Specify the parameters of rollong update strategy
			rollingStrategy?: {
				maxSurge:       *"25%" | string
				maxUnavailable: *"25%" | string
				partition:      *0 | int
			}
		}
	}
}
