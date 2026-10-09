import "vela/test"

_shop: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	metadata: name: "shop"
	spec: components: []
	status: services: [
		{
			name: "web", namespace: "default", healthy: true, workloadHealthy: true, message: "Ready:2/2"
			details: replicas: "2"
			traits: [
				{type: "gateway", healthy: true, message: "No loadBalancer found"},
				{type: "scaler", healthy: true},
				{type: "gateway", healthy: false, message: "a second gateway"},
			]
		},
		{name: "web", cluster: "eu", namespace: "shop", healthy: false, workloadHealthy: false, message: "0/2 ready"},
	]
}

"reports a component's health in the local cluster": test.#SourceExec & {
	definition: "vela-component"
	parameter: {app: "shop", component: "web"}
	resources: [_shop]
	expect: output: {
		name:            "web"
		healthy:         true
		workloadHealthy: true
		message:         "Ready:2/2"
		details: replicas: "2"
		cluster:   "local"
		namespace: "default"
		// A trait type reported twice is read from its first.
		traits: {
			gateway: {healthy: true, pending: false, message: "No loadBalancer found"}
			scaler: healthy: true
		} @exact()
	}
}

"reports another cluster's placement when asked": test.#SourceExec & {
	definition: "vela-component"
	parameter: {app: "shop", component: "web", cluster: "eu"}
	resources: [_shop]
	expect: output: {cluster: "eu", healthy: false, message: "0/2 ready", namespace: "shop"}
}

"a component the Application does not report fails the source": test.#SourceExec & {
	definition: "vela-component"
	parameter: {app: "shop", component: "api"}
	resources: [_shop]
	expect: error: user: [=~"reports no component api"]
}

"a cluster the component is not in fails the source, naming where it is": test.#SourceExec & {
	definition: "vela-component"
	parameter: {app: "shop", component: "web", cluster: "us"}
	resources: [_shop]
	expect: error: user: [=~"not placed in cluster us; it is in: local, eu"]
}

"caches per namespace for a minute, never serving stale": test.#SourceExec & {
	definition: "vela-component"
	parameter: {app: "shop", component: "web"}
	resources: [_shop]
	expect: storage: {ttl: "1m", onStaleFailure: "fail", keyInputs: ["namespace"]}
}

"reads an Application in another namespace": test.#SourceExec & {
	definition: "vela-component"
	parameter: {app: "shop", component: "web", namespace: "team-a"}
	resources: [_shop & {metadata: namespace: "team-a"}]
	expect: output: {name: "web", cluster: "local", healthy: true, namespace: "default"}
}

"an Application in another namespace is not found from this one": test.#SourceExec & {
	definition: "vela-component"
	parameter: {app: "shop", component: "web"}
	resources: [_shop & {metadata: namespace: "team-a"}]
	expect: error: message: =~"applications.core.oam.dev \"shop\" not found"
}
