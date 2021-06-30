patch: {
	spec: template: spec: {
		if parameter.affinity != _|_ {
			affinity: nodeAffinity: requiredDuringSchedulingIgnoredDuringExecution: nodeSelectorTerms: [{
				matchExpressions: [
					for k, v in parameter.affinity {
						key:      k
						operator: "In"
						values:   v
					},
				]}]
		}
		if parameter.tolerations != _|_ {
			tolerations: [
				for k, v in parameter.tolerations {
					effect:   "NoSchedule"
					key:      k
					operator: "Equal"
					value:    v
				}]
		}
	}
}
parameter: {
	affinity?: [string]: [...string]
	tolerations?: [string]: string
}
