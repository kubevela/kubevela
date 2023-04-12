rules: [
	{
		group:    "apps"
		resource: "deployment"
		subResources: [
			{
				group:    "apps"
				resource: "replicaSet"
				selectors: {
					ownerReference: true
				}
			},
		]
		peerResources: commonPeerResources
	}, {
		group:    "apps"
		resource: "replicaSet"
		subResources: [
			{
				group:    ""
				resource: "pod"
				selectors: {
					ownerReference: true
				}
			},
		]
	}, {
		group:    "apps"
		resource: "statefulSet"
		subResources: [
			{
				group:    ""
				resource: "pod"
				selectors: {
					ownerReference: true
				}
			},
		]
		peerResources: commonPeerResources
	}, {
		group:    "apps"
		resource: "daemonSet"
		subResources: [
			{
				group:    ""
				resource: "pod"
				selectors: {
					ownerReference: true
				}
			},
		]
		peerResources: commonPeerResources
	},
]

commonPeerResources: [{
	group:    ""
	resource: "configMap"
	selectors: {
		name: [
			if context.data.spec.template.spec.volumes != _|_ {
				for v in context.data.spec.template.spec.volumes if v.configMap != _|_ if v.configMap.name != _|_ {
					v.configMap.name
				},
			},
		]
	}
}, {
	group:    ""
	resource: "secret"
	selectors: {
		name: [
			if context.data.spec.template.spec.volumes != _|_ {
				for v in context.data.spec.template.spec.volumes if v.secret != _|_ if v.secret.name != _|_ {
					v.secret.name
				},
			},
		]
	}
}, {
	group:    ""
	resource: "service"
	selectors: {
		builtin: "service"
	}
}, {
	group:    "networking.k8s.io"
	resource: "ingress"
	selectors: {
		builtin: "ingress"
	}
}]
