import "vela/test"

"one mock answers every call": test.#ComponentRender & {
	definition: "reader"
	parameter: secret: "creds"
	mocks: "vela/kube": "#Get": $returns: {apiVersion: "v1", kind: "Secret", data: password: "hunter2"}
	expect: output: data: password: "hunter2"
}

"$params choose the mock": test.#ComponentRender & {
	definition: "reader"
	parameter: secret: "creds"
	mocks: "vela/kube": "#Get": [
		{$params: resource: metadata: name: "other", $returns: data: password: "wrong"},
		{$params: resource: metadata: {name: "creds", namespace: "default"}, $returns: data: password: "right"},
		{$returns: data: password: "fallback"},
	]
	expect: output: data: password: "right"
}

"an unmocked call fails": test.#ComponentRender & {
	definition: "reader"
	parameter: secret: "creds"
	expect: error: =~"unmocked call vela/kube.#Get with \\$params"
}

"http": test.#ComponentRender & {
	definition: "fetcher"
	parameter: url: "https://example.com/version"
	mocks: "vela/http": "#Do": {
		$params: url: =~"^https://example.com/"
		$returns: {statusCode: 200, body: "v1.2.3"}
	}
	expect: output: data: {status: "200", body: "v1.2.3"}
}

"a mock must fit the real signature": test.#ComponentRender & {
	definition: "fetcher"
	parameter: url: "https://example.com/version"
	mocks: "vela/http": "#Do": $returns: {statusCode: "200", body: "v1.2.3"}
	expect: error: =~"the mock.s \\$returns does not fit its signature: .*statusCode"
}

"a derived definition is mocked by its own name": test.#ComponentRender & {
	definition: "methods"
	mocks: "vela/http": {
		// Answers only calls that are GETs, as #Get is.
		"#Get": $returns: {statusCode: 200, body: "v1.2.3"}
		// Everything else #Do covers, here the POST.
		"#Do": $returns: {statusCode: 202, body: "noted"}
	}
	expect: {
		output: data: {version: "v1.2.3", report: "noted"}
		calls: "vela/http": {
			// A call is listed under every definition it satisfies.
			"#Get": [{$params: url: "https://example.com/version"}]
			"#Post": [{$params: url: "https://example.com/report"}]
			"#Do": [_, _]
		}
	}
}
