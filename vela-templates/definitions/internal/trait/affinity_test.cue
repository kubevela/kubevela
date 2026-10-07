import "vela/test"

_web: {
	definition: "affinity"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [{name: "web", image: "shop:1.0"}]
	}
}

_term: {labelSelector: matchLabels: app: "cache", topologyKey: "kubernetes.io/hostname"}

"required pod affinity lands on the pod spec": test.#TraitRender & _web & {
	parameter: podAffinity: required: [_term]
	expect: output: spec: template: spec: {
		containers: [{name: "web", image: "shop:1.0"}]
		affinity: {
			podAffinity: {
				requiredDuringSchedulingIgnoredDuringExecution: [{
					labelSelector: matchLabels: app: "cache"
					topologyKey: "kubernetes.io/hostname"
				}]
				preferredDuringSchedulingIgnoredDuringExecution?: _|_
			}
		} @exact()
		tolerations?: _|_
	}
}

"preferred pod anti-affinity carries its weight and term": test.#TraitRender & _web & {
	parameter: podAntiAffinity: preferred: [{weight: 50, podAffinityTerm: _term}]
	expect: output: spec: template: spec: {
		affinity: {
			podAntiAffinity: {
				preferredDuringSchedulingIgnoredDuringExecution: [{
					weight: 50
					podAffinityTerm: {labelSelector: matchLabels: app: "cache", topologyKey: "kubernetes.io/hostname"}
				}]
				requiredDuringSchedulingIgnoredDuringExecution?: _|_
			}
		} @exact()
	}
}

"preferred pod affinity carries its weight and term": test.#TraitRender & _web & {
	parameter: podAffinity: preferred: [{weight: 50, podAffinityTerm: _term}]
	expect: output: spec: template: spec: {
		affinity: {
			podAffinity: {
				preferredDuringSchedulingIgnoredDuringExecution: [{
					weight: 50
					podAffinityTerm: {labelSelector: matchLabels: app: "cache", topologyKey: "kubernetes.io/hostname"}
				}]
				requiredDuringSchedulingIgnoredDuringExecution?: _|_
			}
		} @exact()
	}
}

"required pod anti-affinity lands on the pod spec": test.#TraitRender & _web & {
	parameter: podAntiAffinity: required: [_term]
	expect: output: spec: template: spec: {
		affinity: {
			podAntiAffinity: {
				requiredDuringSchedulingIgnoredDuringExecution: [{
					labelSelector: matchLabels: app: "cache"
					topologyKey: "kubernetes.io/hostname"
				}]
				preferredDuringSchedulingIgnoredDuringExecution?: _|_
			}
		} @exact()
	}
}

"namespaces and a namespace selector are passed through": test.#TraitRender & _web & {
	parameter: podAffinity: required: [{
		topologyKey: "zone"
		namespaces: ["shop", "cache"]
		namespaceSelector: matchLabels: team: "shop"
	}]
	expect: output: spec: template: spec: affinity: podAffinity: requiredDuringSchedulingIgnoredDuringExecution: [{
		topologyKey: "zone"
		namespaces: ["shop", "cache"]
		namespaceSelector: matchLabels: team: "shop"
		labelSelector?: _|_
	}]
}

"a label selector expression defaults to In": test.#TraitRender & _web & {
	parameter: podAffinity: required: [{
		topologyKey: "zone"
		labelSelector: matchExpressions: [{key: "app", values: ["cache"]}]
	}]
	expect: output: spec: template: spec: affinity: podAffinity: requiredDuringSchedulingIgnoredDuringExecution: [{
		labelSelector: matchExpressions: [{key: "app", operator: "In", values: ["cache"]}]
	}]
}

"node affinity takes selector terms and preferences": test.#TraitRender & _web & {
	parameter: nodeAffinity: {
		required: nodeSelectorTerms: [{matchExpressions: [{key: "disktype", values: ["ssd"]}]}]
		preferred: [{weight: 10, preference: matchFields: [{key: "metadata.name", operator: "NotIn", values: ["node-1"]}]}]
	}
	expect: output: spec: template: spec: {
		affinity: {
			nodeAffinity: {
				requiredDuringSchedulingIgnoredDuringExecution: nodeSelectorTerms: [{
					matchExpressions: [{key: "disktype", operator: "In", values: ["ssd"]}]
					matchFields?: _|_
				}]
				preferredDuringSchedulingIgnoredDuringExecution: [{
					weight: 10
					preference: matchFields: [{key: "metadata.name", operator: "NotIn", values: ["node-1"]}]
				}]
			}
		} @exact()
	}
}

"tolerations default to Equal and keep only what is given": test.#TraitRender & _web & {
	parameter: tolerations: [
		{key: "dedicated", value: "shop", effect: "NoSchedule"},
		{operator: "Exists", effect: "NoExecute", tolerationSeconds: 300},
	]
	expect: output: spec: template: spec: {
		tolerations: [
			{key: "dedicated", operator: "Equal", value: "shop", effect: "NoSchedule", tolerationSeconds?: _|_},
			{operator: "Exists", effect: "NoExecute", tolerationSeconds: 300, key?: _|_, value?: _|_},
		]
		affinity?: _|_
	}
}

"no parameters leave the pod spec alone": test.#TraitRender & _web & {
	expect: {
		output: spec: template: spec: {
			containers: [{name: "web", image: "shop:1.0"}]
			affinity?:    _|_
			tolerations?: _|_
		}
		outputs: {} @exact()
	}
}

"a weight above 100 is rejected": test.#TraitRender & _web & {
	parameter: podAffinity: preferred: [{weight: 101, podAffinityTerm: _term}]
	expect: error: {
		parameter: [=~"weight"] @contains()
	}
}

"a toleration effect must be a taint effect": test.#TraitRender & _web & {
	parameter: tolerations: [{key: "dedicated", effect: "Sometimes"}]
	expect: error: {
		parameter: [=~"effect"] @contains()
	}
}

"a pod affinity term needs a topology key": test.#TraitRender & _web & {
	parameter: podAffinity: required: [{labelSelector: matchLabels: app: "cache"}]
	expect: error: =~"topologyKey"
}
