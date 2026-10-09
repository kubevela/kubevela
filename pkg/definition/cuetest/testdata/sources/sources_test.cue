import "vela/test"

_shipped: "../../../../../vela-templates/definitions/internal/source/"

"configmap-local reads the Application's namespace": test.#SourceExec & {
	definition: _shipped + "configmap-local"
	parameter: name: "settings"
	resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: name: "settings", data: tier: "gold"}]
	expect: {
		output: data: tier: "gold"
		calls: "vela/kube": "#Get": [{$params: resource: metadata: name: "settings"}]
	}
}

"configmap-local follows context.namespace": test.#SourceExec & {
	definition: _shipped + "configmap-local"
	context: namespace: "team-a"
	parameter: name: "settings"
	resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: name: "settings", data: tier: "silver"}]
	expect: output: data: tier: "silver"
}

"http-get decodes JSON": test.#SourceExec & {
	definition: _shipped + "http-get"
	parameter: url: "https://config.example.com/shop.json"
	mocks: "vela/http": "#Get": $returns: {
		statusCode: 200
		body:       "{\"tier\": \"gold\"}"
		header: "Content-Type": ["application/json"]
	}
	expect: {
		output: {statusCode: 200, content: tier: "gold"}
		calls: "vela/http": "#Get": [{$params: url: "https://config.example.com/shop.json"}]
	}
}

"http-get refuses a non-2xx": test.#SourceExec & {
	definition: _shipped + "http-get"
	parameter: url: "https://config.example.com/shop.json"
	mocks: "vela/http": "#Get": $returns: {statusCode: 503, body: "", header: {}}
	expect: error: user: ["GET https://config.example.com/shop.json returned 503"]
}

"http-get must be mocked": test.#SourceExec & {
	definition: _shipped + "http-get"
	parameter: url: "https://config.example.com/shop.json"
	expect: error: message: =~"reaches outside the cluster"
}

"vela-addon sees a running addon": test.#SourceExec & {
	definition: _shipped + "vela-addon"
	parameter: {name: "fluxcd", namespace: "vela-system"}
	resources: [{
		apiVersion: "core.oam.dev/v1beta1"
		kind:       "Application"
		metadata: {name: "addon-fluxcd", namespace: "vela-system", labels: "addons.oam.dev/name": "fluxcd"}
		spec: components: []
		status: status: "running"
	}]
	expect: output: {installed: true, running: true, app: "addon-fluxcd", namespace: "vela-system"}
}

"vela-addon without the addon": test.#SourceExec & {
	definition: _shipped + "vela-addon"
	parameter: {name: "not-installed", namespace: "vela-system"}
	expect: output: {installed: false, running: false, app: ""}
}

// kube.#List's filter.namespace defaults to "" and the template's namespace
// parameter to "vela-system"; unified, neither default holds, so the call's
// $params are not concrete.
"vela-addon with its default namespace": test.#SourceExec & {
	definition: _shipped + "vela-addon"
	parameter: name: "fluxcd"
	expect: output: namespace: "vela-system"
} @pending(shipped vela-addon: filter.namespace has two defaults when namespace is left unset)

"errs are the user's": test.#SourceExec & {
	definition: "strict"
	parameter: count: -1
	expect: error: user: ["count must not be negative, got -1"]
}

"output must fit the schema": test.#SourceExec & {
	definition: "strict"
	parameter: {count: 1, extra: true}
	expect: error: schema: [=~"surplus"]
}

"context.name is the binding": test.#SourceExec & {
	definition: "strict"
	context: name: "cfg"
	parameter: count: 2
	expect: output: count: 2
}

"a hyphenated context.name is a binding": test.#SourceExec & {
	definition: "strict"
	context: name: "cluster-info"
	parameter: count: 3
	expect: output: count: 3
}

"configmap-local caches per cluster and namespace": test.#SourceExec & {
	definition: _shipped + "configmap-local"
	parameter: name: "settings"
	resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: name: "settings", data: tier: "gold"}]
	expect: storage: {
		ttl:            "5m"
		onStaleFailure: "use-stale"
		keyInputs: ["cluster", "namespace"]
		key: =~"^configmap-local-"
	}
}

"with no storage block, the defaults": test.#SourceExec & {
	definition: "strict"
	parameter: count: 1
	expect: storage: {ttl: "15m", onStaleFailure: "use-stale", keyInputs: []}
}

"a TTL compares as a duration": test.#SourceExec & {
	definition: "tuned"
	expect: storage: {ttl: "90s", onStaleFailure: "fail", keyInputs: ["appName"]}
}
