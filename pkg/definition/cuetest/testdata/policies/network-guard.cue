"network-guard": {
	type:        "policy"
	description: "A NetworkPolicy per component, admitting traffic from one namespace"
}
template: {
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: "\(context.name)-summary"
		data: {for comp, _ in context.artifacts {(comp): "guarded from \(parameter.allowFrom)"}}
	}
	outputs: {
		for comp, a in context.artifacts {
			(comp): {
				apiVersion: "networking.k8s.io/v1"
				kind:       "NetworkPolicy"
				metadata: name: "\(comp)-guard"
				spec: {
					podSelector: matchLabels: a.workload.spec.selector.matchLabels
					ingress: [{from: [{namespaceSelector: matchLabels: "kubernetes.io/metadata.name": parameter.allowFrom}]}]
				}
			}
		}
	}
	parameter: allowFrom: string
}
