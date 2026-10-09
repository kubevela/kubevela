import "vela/test"

_shop: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	metadata: name: "shop"
	spec: components: []
	status: {
		status: "running"
		latestRevision: {name: "shop-v3", revision: 3, revisionHash: "abc123"}
		workflow: {mode: "DAG-DAG", status: "succeeded", suspend: false, terminated: false, finished: true, message: "", appRevision: "shop-v3", startTime: "2026-09-27T10:00:00Z"}
		services: _services
	}
	_services: *[
		{name: "web", namespace: "default", healthy: true},
		{name: "web", cluster: "eu", namespace: "shop", healthy: false, message: "0/2 ready"},
		{name: "worker", namespace: "default", healthy: true},
	] | [...]
}

"summarises the Application in the source's namespace": test.#SourceExec & {
	definition: "vela-app"
	parameter: name: "shop"
	resources: [_shop]
	expect: output: {
		name:     "shop"
		phase:    "running"
		revision: "shop-v3"
		// One unhealthy placement makes the Application unhealthy.
		healthy: false
		workflow: {phase: "succeeded", finished: true}
		components: ["web", "worker"]
		clusters: ["local", "eu"]
		services: {
			web: {
				healthy: false
				clusters: {
					local: {healthy: true, namespace: "default"}
					eu: {healthy: false, message: "0/2 ready", namespace: "shop"}
				}
			}
			worker: healthy: true
		}
	}
}

"reads another namespace when asked": test.#SourceExec & {
	definition: "vela-app"
	parameter: {name: "shop", namespace: "team-a"}
	resources: [_shop & {metadata: namespace: "team-a"}]
	expect: {
		output: namespace: "team-a"
		calls: "vela/kube": "#Get": [{$params: resource: metadata: namespace: "team-a"}]
	}
}

"an Application with no services is not healthy": test.#SourceExec & {
	definition: "vela-app"
	parameter: name: "shop"
	resources: [_shop & {_services: []}]
	expect: output: {healthy: false, components: [], clusters: []}
}

"a missing Application fails the source": test.#SourceExec & {
	definition: "vela-app"
	parameter: name: "absent"
	expect: error:   =~"not found"
}

"caches per namespace for a minute, never serving stale": test.#SourceExec & {
	definition: "vela-app"
	parameter: name: "shop"
	resources: [_shop]
	expect: storage: {ttl: "1m", onStaleFailure: "fail", keyInputs: ["namespace"]}
}
