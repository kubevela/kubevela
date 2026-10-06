import "vela/test"

_web: {
	definition: "topologyspreadconstraints"
	context: name: "web"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [{name: "web", image: "shop:1.0"}]
	}
}

"spreads pods across a topology, not scheduling on a breach by default": test.#TraitRender & _web & {
	parameter: constraints: [{maxSkew: 1, topologyKey: "topology.kubernetes.io/zone", labelSelector: matchLabels: app: "web"}]
	expect: {
		output: spec: template: spec: {
			containers: [{name: "web", image: "shop:1.0"}]
			topologySpreadConstraints: [{
				maxSkew:           1
				topologyKey:       "topology.kubernetes.io/zone"
				whenUnsatisfiable: "DoNotSchedule"
				labelSelector: matchLabels: app: "web"
				minDomains?:         _|_
				matchLabelKeys?:     _|_
				nodeAffinityPolicy?: _|_
				nodeTaintsPolicy?:   _|_
			}]
		}
		outputs: {} @exact()
	}
}

"optional fields pass through when given": test.#TraitRender & _web & {
	parameter: constraints: [{
		maxSkew:           2
		topologyKey:       "kubernetes.io/hostname"
		whenUnsatisfiable: "ScheduleAnyway"
		labelSelector: matchExpressions: [{key: "app", values: ["web"]}]
		minDomains: 3
		matchLabelKeys: ["pod-template-hash"]
		nodeAffinityPolicy: "Ignore"
		nodeTaintsPolicy:   "Honor"
	}]
	expect: output: spec: template: spec: topologySpreadConstraints: [{
		whenUnsatisfiable: "ScheduleAnyway"
		labelSelector: matchExpressions: [{key: "app", operator: "In", values: ["web"]}]
		minDomains: 3
		matchLabelKeys: ["pod-template-hash"]
		nodeAffinityPolicy: "Ignore"
		nodeTaintsPolicy:   "Honor"
	}]
}

"each constraint is kept, in order": test.#TraitRender & _web & {
	parameter: constraints: [
		{maxSkew: 1, topologyKey: "topology.kubernetes.io/zone", labelSelector: {}},
		{maxSkew: 1, topologyKey: "kubernetes.io/hostname", labelSelector: {}},
	]
	expect: output: spec: template: spec: topologySpreadConstraints: [
		{topologyKey: "topology.kubernetes.io/zone"},
		{topologyKey: "kubernetes.io/hostname"},
	]
}

"a constraint needs a topology key": test.#TraitRender & _web & {
	parameter: constraints: [{maxSkew: 1, labelSelector: {}}]
	expect: error: template: [=~"topologyKey: incomplete value"]
}

"an unknown whenUnsatisfiable is rejected": test.#TraitRender & _web & {
	parameter: constraints: [{maxSkew: 1, topologyKey: "zone", whenUnsatisfiable: "Never", labelSelector: {}}]
	expect: error: {
		parameter: [=~"whenUnsatisfiable"] @contains()
	}
}
