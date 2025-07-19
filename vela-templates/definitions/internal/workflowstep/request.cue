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
	} @step(1)

	wait: op.#ConditionalWait & {
		continue: req.$returns != _|_
		message?: "Waiting for response from \(parameter.url)"
	} @step(2)

	fail: op.#Steps & {
		if req.$returns.statusCode > 400 {
			requestFail: op.#Fail & {
				message: "request of \(parameter.url) is fail: \(req.$returns.statusCode)"
			}
		}
	} @step(3)

	response: json.Unmarshal(req.$returns.body)

	parameter: {
		url:    string
		method: *"GET" | "POST" | "PUT" | "DELETE"
		body?: {...}
		header?: [string]: string
	}
}
