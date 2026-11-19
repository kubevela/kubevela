import "vela/test"

"without clusters, it deploys to all of them": test.#AddonRender & {
	addon: "."
	expect: application: spec: policies: [{type: "topology", properties: clusterLabelSelector: {}}]
}

"the clusters it names are registered": test.#AddonRender & {
	addon: "."
	parameter: clusters: ["prod-eu", "prod-us"]
	expect: application: spec: policies: [{type: "topology", properties: clusters: ["prod-eu", "prod-us", "local"]}]
}

"a component and a trait may share a name": test.#AddonRender & {
	addon: "."
	expect: definitions: {
		ComponentDefinition: "fleet-agent": spec: workload: definition: kind: "ConfigMap"
		TraitDefinition: "fleet-agent": spec: appliesToWorkloads: ["*"]
	}
}
