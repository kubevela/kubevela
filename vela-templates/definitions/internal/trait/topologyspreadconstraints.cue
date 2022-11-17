topologyspreadconstraints: {
	type: "trait"
	annotations: {}
	description: "Add topology spread constraints hooks for every container of K8s pod for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
	}
}
template: {
	constraintsArray: [
		for v in parameter.constraints {
			maxSkew:           v.maxSkew
			topologyKey:       v.topologyKey
			whenUnsatisfiable: v.whenUnsatisfiable
			labelSelector:     v.labelSelector
			if v.nodeAffinityPolicy != _|_ {
				nodeAffinityPolicy: v.nodeAffinityPolicy
			}
			if v.nodeTaintsPolicy != _|_ {
				nodeTaintsPolicy: v.nodeTaintsPolicy
			}
			if v.minDomains != _|_ {
				minDomains: v.minDomains
			}
			if v.matchLabelKeys != _|_ {
				matchLabelKeys: v.matchLabelKeys
			}
		},
	]
	patch: spec: template: spec: {
		topologySpreadConstraints: constraintsArray
	}
	#labSelector: {
		matchLabels?: [string]: string
		matchExpressions?: [...{
			key:      string
			operator: *"In" | "NotIn" | "Exists" | "DoesNotExist"
			values?: [...string]
		}]
	}
	parameter: {
		constraints: [...{
			// +usage=Describe the degree to which Pods may be unevenly distributed
			maxSkew: int
			// +usage=Specify the key of node labels
			topologyKey: string
			// +usage=Indicate how to deal with a Pod if it doesn't satisfy the spread constraint
			whenUnsatisfiable: *"DoNotSchedule" | "ScheduleAnyway"
			// +usage: labelSelector to find matching Pods
			labelSelector: #labSelector
			// +usage=Indicate a minimum number of eligible domains
			minDomains?: int
			// +usage=A list of pod label keys to select the pods over which spreading will be calculated
			matchLabelKeys?: [...string]
			// +usage=Indicate how we will treat Pod's nodeAffinity/nodeSelector when calculating pod topology spread skew
			nodeAffinityPolicy?: *"Honor" | "Ignore"
			// +usage=Indicate how we will treat node taints when calculating pod topology spread skew
			nodeTaintsPolicy?: *"Honor" | "Ignore"
		}]
	}
}
