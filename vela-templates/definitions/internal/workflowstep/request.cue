import (
	"vela/op"
	"vela/http"
	"encoding/json"
)

request: {
	alias: ""
	attributes: {}
	description: "Send request to the url"
	annotations: {
		"category": "External Integration"
	}
	labels: {}
	type: "workflow-step"
}

template: {
	req: http.#HTTPDo & {
		$params: {
			method: parameter.method
			url:    parameter.url
			request: {
				if parameter.body != _|_ {
					body: json.Marshal(parameter.body)
				}
				if parameter.header != _|_ {
					header: parameter.header
				}
			}
		}
	}

	wait: op.#ConditionalWait & {
		continue: req.$returns != _|_
		message?: "Waiting for response from \(parameter.url)"
	}

	fail: op.#Steps & {
		if req.$returns.statusCode >= 400 {
			requestFail: op.#Fail & {
				message: "request of \(parameter.url) is fail: \(req.$returns.statusCode)"
			}
		}
	}

	// The body is kept as sent; response holds it parsed, only when it is JSON, so an empty
	// or text reply succeeds rather than failing the step after the request has been made.
	if req.$returns != _|_ {
		responseBody: req.$returns.body
		if json.Valid(req.$returns.body) {
			response: json.Unmarshal(req.$returns.body)
		}
	}

	parameter: {
		url:    string
		method: *"GET" | "POST" | "PUT" | "DELETE"
		body?: {...}
		header?: [string]: string
	}
}
