import "vela/test"

_sync: {
	definition: "sync"
	parameter: {config: "settings", webhook: "https://hooks.example.com/deploy"}
	mocks: {
		"vela/kube": "#Get": $returns: {apiVersion: "v1", kind: "ConfigMap", data: message: "hello"}
		"vela/http": "#Do": $returns: statusCode: 202
	}
}

"exactly these calls": test.#ComponentRender & _sync & {
	expect: calls: {
		"vela/kube": "#Get": [{$params: resource: {kind: "ConfigMap", metadata: name: "settings"}}]
		"vela/http": "#Do": [{$params: {method: "POST", url: =~"^https://hooks", request: body: "hello"}}]
	}
}

"what a mock returned is recorded": test.#ComponentRender & _sync & {
	expect: calls: "vela/kube": "#Get": [{$returns: data: message: "hello"}]
}

"in any order": test.#ComponentRender & _sync & {
	expect: calls: "vela/http": "#Do": [{$params: method: "POST"}] @contains()
}

"a call that was not made": test.#ComponentRender & _sync & {
	expect: calls: {
		"vela/kube": "#Apply"?: _|_
		"vela/http": "#Do": [{$params: method: "DELETE"}] @contains() @not()
	}
}

"fails on purpose": test.#ComponentRender & _sync & {
	expect: calls: "vela/http": "#Do": [{$params: method: "GET"}]
}
