import "vela/test"

_ok: mocks: "vela/http": "#HTTPDo": $returns: {statusCode: 200, body: "{\"version\":\"1.2.3\"}"}

"sends a GET to the url by default": test.#WorkflowStepExec & _ok & {
	definition: "request"
	parameter: url: "https://api.example.com/version"
	expect: {
		phase: "succeeded"
		calls: "vela/http": "#HTTPDo": [{$params: {
			method: "GET"
			url:    "https://api.example.com/version"
			request: {} @exact()
		}}]
	}
}

"sends the body as JSON with the given method and header": test.#WorkflowStepExec & _ok & {
	definition: "request"
	parameter: {
		url:    "https://api.example.com/releases"
		method: "POST"
		body: {name: "shop", replicas: 2}
		header: "X-Team": "payments"
	}
	expect: {
		phase: "succeeded"
		calls: "vela/http": "#HTTPDo": [{$params: {
			method: "POST"
			request: {
				body: "{\"name\":\"shop\",\"replicas\":2}"
				header: "X-Team": "payments"
			} @exact()
		}}]
	}
}

"passes the timeout, secret headers and rate limiter through": test.#WorkflowStepExec & _ok & {
	definition: "request"
	parameter: {
		url:     "https://api.example.com/version"
		timeout: "10s"
		headersFromSecret: [{header: "Authorization", secret: "api-token", key: "token"}]
		ratelimiter: {limit: 5, period: "1m"}
	}
	expect: calls: "vela/http": "#HTTPDo": [{$params: {
		request: {
			timeout: "10s"
			headersFromSecret: [{header: "Authorization", secret: "api-token", key: "token"}]
			ratelimiter: {limit: 5, period: "1m"}
		} @exact()
	}}]
}

"a status above 400 fails the step": test.#WorkflowStepExec & {
	definition: "request"
	parameter: url: "https://api.example.com/version"
	mocks: "vela/http": "#HTTPDo": $returns: {statusCode: 503, body: "{}"}
	expect: {
		phase:   "failed"
		message: "request of https://api.example.com/version is fail: 503"
	}
}

_noResponse: {
	definition: "request"
	parameter: url: "https://api.example.com/version"
	mocks: "vela/http": "#HTTPDo": {}
}

"waits until there is a response": test.#WorkflowStepExec & _noResponse & {
	expect: phase: "running"
}

"says what it waits for": test.#WorkflowStepExec & _noResponse & {
	expect: message: "Waiting for response from https://api.example.com/version"
}

"a response body that is not JSON fails the step": test.#WorkflowStepExec & {
	definition: "request"
	parameter: url: "https://api.example.com/version"
	mocks: "vela/http": "#HTTPDo": $returns: {statusCode: 200, body: "OK"}
	expect: {
		phase:   "failed"
		message: =~"json: invalid JSON"
	}
}

"a timeout that is not a duration is rejected": test.#WorkflowStepExec & _ok & {
	definition: "request"
	parameter: {url: "https://api.example.com/version", timeout: "soon"}
	expect: {
		phase:  "failed"
		reason: "Execute"
		calls: "vela/http"?: _|_
	}
}

"a url is required": test.#WorkflowStepExec & _ok & {
	definition: "request"
	expect: {
		phase:  "failed"
		reason: "Execute"
		calls: "vela/http"?: _|_
	}
}
