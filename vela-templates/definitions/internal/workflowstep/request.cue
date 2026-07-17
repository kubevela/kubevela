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
				if parameter.timeout != _|_ {
					timeout: parameter.timeout
				}
			}
		}
	}

	wait: op.#ConditionalWait & {
		continue: req.$returns != _|_
		message?: "Waiting for response from \(parameter.url)"
	}

	fail: op.#Steps & {
		if req.$returns.statusCode > 400 {
			requestFail: op.#Fail & {
				message: "request of \(parameter.url) is fail: \(req.$returns.statusCode)"
			}
		}
	}

	response: json.Unmarshal(req.$returns.body)

	parameter: {
		url:    string
		method: *"GET" | "POST" | "PUT" | "DELETE"
		body?: {...}
		header?: [string]: string
		// +usage=The timeout of this request (Go duration string, e.g. "30s", "2m", "500ms"). Defaults to 3s when omitted. Invalid values fail when the step runs.
		timeout?: string & =~"^(0|(([0-9]+(\\.[0-9]*)?|\\.[0-9]+)(ns|us|µs|μs|ms|s|m|h))+)$"
	}
}
