import "vela/test"

_web: {
	definition: "service-account"
	context: {name: "web", namespace: "shop"}
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: {
			serviceAccountName: "default"
			containers: [{name: "web", image: "shop:1.0"}]
		}
	}
}

"runs the pods as an existing ServiceAccount": test.#TraitRender & _web & {
	parameter: name: "shop-reader"
	expect: {
		output: spec: template: spec: {
			serviceAccountName: "shop-reader"
			containers: [{name: "web", image: "shop:1.0"}]
		}
		outputs: {} @exact()
	}
}

"a name is required": test.#TraitRender & _web & {
	parameter: create: true
	expect: error: template: [=~"serviceAccountName: incomplete value"]
}

"create makes the ServiceAccount too": test.#TraitRender & _web & {
	parameter: {name: "shop-reader", create: true}
	expect: {
		outputs: {
			"service-account": {apiVersion: "v1", kind: "ServiceAccount", metadata: name: "shop-reader"}
		} @exact()
	}
}

"namespace privileges become a Role bound to the account": test.#TraitRender & _web & {
	parameter: {
		name: "shop-reader"
		privileges: [{verbs: ["get", "list"], apiGroups: [""], resources: ["configmaps"], resourceNames: ["shop-config"]}]
	}
	expect: {
		outputs: {
			role: {
				apiVersion: "rbac.authorization.k8s.io/v1"
				kind:       "Role"
				metadata: name: "shop-reader"
				rules: [{verbs: ["get", "list"], apiGroups: [""], resources: ["configmaps"], resourceNames: ["shop-config"], nonResourceURLs?: _|_}]
			}
			"role-binding": {
				apiVersion: "rbac.authorization.k8s.io/v1"
				kind:       "RoleBinding"
				metadata: name: "shop-reader"
				roleRef: {apiGroup: "rbac.authorization.k8s.io", kind: "Role", name: "shop-reader"}
				subjects: [{kind: "ServiceAccount", name: "shop-reader"}]
			}
		} @exact()
	}
}

"cluster privileges become a ClusterRole named for the namespace": test.#TraitRender & _web & {
	parameter: {
		name: "shop-reader"
		privileges: [{scope: "cluster", verbs: ["get"], nonResourceURLs: ["/healthz"]}]
	}
	expect: {
		outputs: {
			"cluster-role": {
				kind: "ClusterRole"
				metadata: name: "shop:shop-reader"
				rules: [{verbs: ["get"], nonResourceURLs: ["/healthz"], apiGroups?: _|_, resources?: _|_}]
			}
			"cluster-role-binding": {
				kind: "ClusterRoleBinding"
				metadata: name: "shop:shop-reader"
				roleRef: {kind: "ClusterRole", name: "shop:shop-reader"}
				subjects: [{kind: "ServiceAccount", name: "shop-reader", namespace: "shop"}]
			}
		} @exact()
	}
}

"privileges of both scopes are split between the two kinds of role": test.#TraitRender & _web & {
	parameter: {
		name: "shop-reader"
		privileges: [
			{verbs: ["get"], resources: ["pods"]},
			{scope: "cluster", verbs: ["list"], resources: ["nodes"]},
		]
	}
	expect: {
		outputs: {
			role: rules: [{verbs: ["get"], resources: ["pods"]}]
			"role-binding": _
			"cluster-role": rules: [{verbs: ["list"], resources: ["nodes"]}]
			"cluster-role-binding": _
		} @exact()
	}
}

"an unknown scope is rejected": test.#TraitRender & _web & {
	parameter: {name: "shop-reader", privileges: [{scope: "global", verbs: ["get"]}]}
	expect: error: {
		parameter: [=~"scope"] @contains()
	}
}
