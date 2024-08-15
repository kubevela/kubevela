import (
	"vela/http"
	"vela/builtin"
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
	fail: {
		if http.$returns.response.statusCode > 400 {
			requestFail: builtin.#Fail & {
				$params: message: "request of \(parameter.url) is fail: \(http.response.statusCode)"
			}
		}
	}
	response: json.Unmarshal(http.$returns.response.body)
	parameter: {
		url:    string
		method: *"GET" | "POST" | "PUT" | "DELETE"
		body?: {...}
		header?: [string]: string
	}
}
