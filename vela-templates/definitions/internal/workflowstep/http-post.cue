import (
	"vela/http"
	"encoding/json"
	"strings"
)

"http-post": {
	type: "workflow-step"
	annotations: {
		"category": "External Integration"
	}
	labels: {}
	description: "Send an HTTP POST/PUT/DELETE/GET request once and expose parsed outputs. Use with http-get-wait for polling."
}

template: {
	parts: *[parameter.endpoint, parameter.uri] | [...string]
	url:   strings.Join(parts, "")

	req: http.#HTTPDo & {
		$params: {
			method: parameter.method
			url:    url
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
				if parameter.ratelimiter != _|_ {
					ratelimiter: parameter.ratelimiter
				}
			}
		}
	}

	respBody: req.$returns.body
	resp:     json.Unmarshal(respBody)

	outputs: {
		statusCode: req.$returns.statusCode
		body:       resp
		id?:        *resp["id"] | string
	}

	parameter: {
		// +usage=Base endpoint, e.g. https://api.example.com
		endpoint: string
		// +usage=Path, e.g. /resource
		uri: string
		// +usage=HTTP method
		method: *"POST" | "PUT" | "DELETE" | "GET"
		// +usage=Request body for POST/PUT
		body?: {...}
		// +usage=HTTP headers
		header?: [string]: string
		// +usage=Request timeout (e.g. 10s)
		timeout?: string
		// +usage=Rate limiter settings {limit: 200, period: \"5s\"}
		ratelimiter?: {
			limit:  int
			period: string
		}
	}
}


