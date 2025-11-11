import (
	"vela/op"
	"vela/http"
	"encoding/json"
	"strings"
)

"http-get-wait": {
	type: "workflow-step"
	annotations: {
		"category": "External Integration"
	}
	labels: {}
	description: "Poll a GET endpoint and wait until the continue condition is met. Split your POST into a previous step to avoid re-execution."
}

template: {
	getParts: *[parameter.endpoint, parameter.uri] | [...string]
	getUrlBase: strings.Join(getParts, "")
	getUrl: *getUrlBase | "\(getUrlBase)/\(parameter.id)"

	resp: http.#HTTPGet & {
		$params: {
			url: getUrl
			request: {
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

	respMap: json.Unmarshal(resp.response.body)

	wait: op.#ConditionalWait & {
		continue: parameter.continueExpr
		message?: parameter.message
	}

	outputs?: {
		body: respMap
	}

	parameter: {
		// +usage=Base endpoint, e.g. https://api.example.com
		endpoint: string
		// +usage=Path, e.g. /resource
		uri: string
		// +usage=Optional id appended to URL like /{id}
		id?: string
		// +usage=HTTP headers
		header?: [string]: string
		// +usage=Request timeout (e.g. 10s)
		timeout?: string
		// +usage=Rate limiter settings {limit: 200, period: \"5s\"}
		ratelimiter?: {
			limit:  int
			period: string
		}
		// +usage=Continue expression based on respMap, e.g. respMap[\"status\"] == \"success\" && respMap[\"output\"] != _|_
		continueExpr: bool
		// +usage=Optional message when waiting
		message?: string
	}
}


