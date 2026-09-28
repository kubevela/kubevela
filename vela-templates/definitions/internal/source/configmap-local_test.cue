import "vela/test"

_settings: {apiVersion: "v1", kind: "ConfigMap", metadata: name: "settings", data: {tier: "gold", region: "eu"}}

"reads the ConfigMap's data from the Application's namespace": test.#SourceExec & {
	definition: "configmap-local"
	parameter: name: "settings"
	resources: [_settings]
	expect: {
		output: {
			data: {tier: "gold", region: "eu"} @exact()
		}
		calls: "vela/kube": "#Get": [{$params: {cluster: "local", resource: {kind: "ConfigMap", metadata: name: "settings"}}}]
	}
}

"follows the Application's namespace": test.#SourceExec & {
	definition: "configmap-local"
	context: namespace: "team-a"
	parameter: name:    "settings"
	resources: [_settings & {metadata: namespace: "team-a"}]
	expect: calls: "vela/kube": "#Get": [{$params: resource: metadata: namespace: "team-a"}]
}

"a missing ConfigMap fails the source": test.#SourceExec & {
	definition: "configmap-local"
	parameter: name: "absent"
	expect: error:   =~"not found"
}

"caches per cluster and namespace, for five minutes, serving stale on failure": test.#SourceExec & {
	definition: "configmap-local"
	parameter: name: "settings"
	resources: [_settings]
	expect: storage: {ttl: "5m", onStaleFailure: "use-stale", keyInputs: ["cluster", "namespace"]}
}
