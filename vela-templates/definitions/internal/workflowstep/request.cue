import (
	"vela/op"
	"encoding/json"
)

request: {
	alias: ""
	attributes: {}
	description: "Send request to the url"
	annotations: {
		"category": "External Intergration"
	}
	labels: {}
	type: "workflow-step"
}

template: {
	http: op.#HTTPDo & {
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
	fail: op.#Steps & {
		if http.response.statusCode > 400 {
			requestFail: op.#Fail & {
				message: "request of \(parameter.url) is fail: \(http.response.statusCode)"
			}
		}
	}
	response: json.Unmarshal(http.response.body)
	parameter: {
		url:    string
		method: *"GET" | "POST" | "PUT" | "DELETE"
		body?: {...}
		header?: [string]: string
	}
}
