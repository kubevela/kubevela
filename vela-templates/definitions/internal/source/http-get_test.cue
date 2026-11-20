import "vela/test"

_url: "https://config.example.com/shop"

_get: {
	definition: "http-get"
	parameter: url: _url
}

"decodes a JSON body": test.#SourceExec & _get & {
	mocks: "vela/http": "#Get": $returns: {statusCode: 200, body: "{\"tier\": \"gold\"}", header: "Content-Type": ["application/json; charset=utf-8"]}
	expect: {
		output: {statusCode: 200, content: tier: "gold"}
		calls: "vela/http": "#Get": [{$params: {method: "GET", url: _url}}]
	}
}

"decodes a YAML body": test.#SourceExec & _get & {
	mocks: "vela/http": "#Get": $returns: {statusCode: 200, body: "tier: gold\n", header: "Content-Type": ["application/yaml"]}
	expect: output: content: tier: "gold"
}

"keeps any other body as a string": test.#SourceExec & _get & {
	mocks: "vela/http": "#Get": $returns: {statusCode: 200, body: "v1.2.3", header: "Content-Type": ["text/plain"]}
	expect: output: content: "v1.2.3"
}

"a response without a content type is a string": test.#SourceExec & _get & {
	mocks: "vela/http": "#Get": $returns: {statusCode: 200, body: "v1.2.3", header: {}}
	expect: output: content: "v1.2.3"
}

"a non-2xx status fails with the definition's own error": test.#SourceExec & _get & {
	mocks: "vela/http": "#Get": $returns: {statusCode: 503, body: "", header: {}}
	expect: error: user: ["GET \(_url) returned 503"]
}

"the request reaches outside the cluster, so a test mocks it": test.#SourceExec & _get & {
	expect: error: message: =~"reaches outside the cluster"
}

// It reads no context, so every Application reading a URL shares one entry.
"caches per URL alone": test.#SourceExec & _get & {
	mocks: "vela/http": "#Get": $returns: {statusCode: 200, body: "", header: {}}
	expect: storage: {ttl: "5m", onStaleFailure: "use-stale", keyInputs: []}
}
