import (
	"vela/test"
	"vela/kube"
)

// A namespace KubeVela manages as an environment carries its name.
envNamespace: kube.#Apply & {$params: value: {
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: {
		name: "shop-prod"
		labels: {"namespace.oam.dev/env": "prod", team: "payments"}
		annotations: owner: "payments"
	}
}} @before()

"a managed namespace is its environment": test.#SourceExec & {
	definition: "vela-env"
	context: namespace: "shop-prod"
	expect: output: {
		name:      "prod"
		namespace: "shop-prod"
		managed:   true
		labels: {"namespace.oam.dev/env": "prod", team: "payments"}
		annotations: owner: "payments"
	}
}

"an unmanaged namespace is named after itself": test.#SourceExec & {
	definition: "vela-env"
	context: namespace: "plain"
	expect: output: {name: "plain", namespace: "plain", managed: false}
}

"caches per namespace": test.#SourceExec & {
	definition: "vela-env"
	context: namespace: "plain"
	expect: storage: {ttl: "5m", onStaleFailure: "use-stale", keyInputs: ["namespace"]}
}
